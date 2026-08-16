package viewerchat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
)

// rejectChatStructuralPatch returns an actionable error when a chat patchTrip
// attempts overnight/endpoint or other structural edits that belong on
// changeOvernight / replaceDayRoutes — or when list=route was used for a
// mid-day venue that belongs on list=stops.
func rejectChatStructuralPatch(patchJSON []byte, beforeYAML []byte) error {
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil {
		return nil // let ApplyPatch report decode errors
	}

	var (
		routeListVenue bool // upsert on route for a new/endpoint-touching place → use stops
		overnightTouch bool // true overnight/endpoint surgery
		insertDay      bool
		otherStructural []string
	)

	if p.SwapDays != nil {
		otherStructural = append(otherStructural, "swap_days")
	}
	if p.DeleteDay != nil {
		otherStructural = append(otherStructural, "delete_day")
	}
	if p.InsertDay != nil {
		insertDay = true
	}
	for key, raw := range p.Days {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, hasRoute := m["route"]; hasRoute {
			otherStructural = append(otherStructural, fmt.Sprintf("days.%s.route (full route replace)", key))
		}
	}
	if p.UpsertStop != nil && strings.EqualFold(strings.TrimSpace(p.UpsertStop.List), "route") {
		if wouldTouchOvernightEnd(beforeYAML, p.UpsertStop.Day, p.UpsertStop.Place) {
			// New place on list=route is almost always a venue/logistics mistake.
			// Only steer to changeOvernight when the place id is already the day's start/end.
			if routeUpsertIsExistingEndpoint(beforeYAML, p.UpsertStop.Day, p.UpsertStop.Place) {
				overnightTouch = true
				otherStructural = append(otherStructural,
					fmt.Sprintf("upsert_stop on list=route for day %d endpoint %q", p.UpsertStop.Day, p.UpsertStop.Place))
			} else {
				routeListVenue = true
			}
		}
	}
	if p.RemoveStop != nil && (p.RemoveStop.List == "" || strings.EqualFold(p.RemoveStop.List, "route") || strings.EqualFold(p.RemoveStop.List, "both")) {
		if removeTouchesOvernight(beforeYAML, p.RemoveStop) {
			overnightTouch = true
			otherStructural = append(otherStructural,
				fmt.Sprintf("remove_stop on route for day %d (would change overnight/endpoint)", p.RemoveStop.Day))
		}
	}

	if !routeListVenue && !overnightTouch && !insertDay && len(otherStructural) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("chat patchTrip blocked: ")
	parts := make([]string, 0, 4)
	if routeListVenue {
		parts = append(parts, fmt.Sprintf(
			"upsert_stop list=route for day %d place %q",
			p.UpsertStop.Day, strings.TrimSpace(p.UpsertStop.Place)))
	}
	if insertDay {
		parts = append(parts, "insert_day")
	}
	parts = append(parts, otherStructural...)
	b.WriteString(strings.Join(parts, "; "))
	b.WriteString(". ")

	switch {
	case routeListVenue && !overnightTouch && !insertDay && len(otherStructural) == 0:
		// Avis / Hayes failure mode: do not mention replaceDayRoutes here.
		b.WriteString(
			"RETRY the same patchTrip with upsert_stop.list set to \"stops\" (keep places + upsert_stop). "+
				"Restaurants, bars, rental desks, and other mid-day venues belong on the stops list — "+
				"not on route, and not by rewriting the day's drive or inserting a day",
		)
	case insertDay && !overnightTouch && len(otherStructural) == 0 && !routeListVenue:
		b.WriteString(
			"insert_day is blocked in viewer chat. To add a venue/stop use patchTrip with places + " +
				"upsert_stop (list:\"stops\"). To change an overnight use changeOvernight. " +
				"Do not invent new days unless the user asked to insert a day",
		)
	default:
		if routeListVenue {
			b.WriteString(
				"For the venue/stop: retry patchTrip with upsert_stop.list=\"stops\" (not route). ",
			)
		}
		if insertDay {
			b.WriteString("insert_day is blocked — use upsert_stop on stops for venues. ")
		}
		if overnightTouch || len(otherStructural) > 0 {
			b.WriteString(
				"For overnight/endpoint or full route rewrite use changeOvernight (preferred) or replaceDayRoutes. ",
			)
		}
		b.WriteString(
			"patchTrip in chat is for places.*.info, update_day notes/title, and stops-list upsert/remove",
		)
	}
	return fmt.Errorf("%s", b.String())
}

// routeUpsertIsExistingEndpoint is true when place is already the day's route start or end.
func routeUpsertIsExistingEndpoint(yaml []byte, day int, place string) bool {
	trip, err := itinerary.ParseYAML(yaml)
	if err != nil {
		return false
	}
	d := findDay(trip, day)
	if d == nil || len(d.Route) == 0 {
		return false
	}
	place = strings.TrimSpace(place)
	first := strings.TrimSpace(d.Route[0].Place)
	last := strings.TrimSpace(d.Route[len(d.Route)-1].Place)
	return place == first || place == last
}

func wouldTouchOvernightEnd(yaml []byte, day int, place string) bool {
	trip, err := itinerary.ParseYAML(yaml)
	if err != nil {
		return true // fail closed
	}
	d := findDay(trip, day)
	if d == nil || len(d.Route) == 0 {
		return false
	}
	place = strings.TrimSpace(place)
	first := strings.TrimSpace(d.Route[0].Place)
	last := strings.TrimSpace(d.Route[len(d.Route)-1].Place)
	// New place appended to route, or updating an existing end/start place id.
	if place == first || place == last {
		return true
	}
	for _, s := range d.Route {
		if strings.TrimSpace(s.Place) == place {
			return false // mid-route update OK
		}
	}
	// Unknown place on list=route → treated as append; if only one overnight slot semantics, still risky.
	return true
}

func removeTouchesOvernight(yaml []byte, rm *itinerary.RemoveStop) bool {
	if rm == nil {
		return false
	}
	trip, err := itinerary.ParseYAML(yaml)
	if err != nil {
		return true
	}
	d := findDay(trip, rm.Day)
	if d == nil || len(d.Route) == 0 {
		return false
	}
	first := strings.TrimSpace(d.Route[0].Place)
	last := strings.TrimSpace(d.Route[len(d.Route)-1].Place)
	ids := append([]string{}, rm.Places...)
	if rm.Place != "" {
		ids = append(ids, rm.Place)
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == first || id == last {
			return true
		}
	}
	return false
}

func findDay(trip itinerary.Trip, day int) *itinerary.Day {
	for i := range trip.Days {
		if trip.Days[i].Day == day {
			return &trip.Days[i]
		}
	}
	return nil
}
