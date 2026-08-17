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
// mid-day venue that belongs on list=stops — or when days.*.stops full replace
// would bypass upsert_stop (Avis wipe-pubs failure mode).
func rejectChatStructuralPatch(patchJSON []byte, beforeYAML []byte, userAsk string) error {
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil {
		return nil // let ApplyPatch report decode errors
	}

	var (
		routeListVenue   bool // upsert on route for a new/endpoint-touching place → use stops
		overnightTouch   bool // true overnight/endpoint surgery
		insertDay        bool
		stopsFullReplace bool
		otherStructural  []string
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
		if _, hasStops := m["stops"]; hasStops {
			stopsFullReplace = true
			otherStructural = append(otherStructural, fmt.Sprintf("days.%s.stops (full stops replace)", key))
		}
	}
	if p.UpsertStop != nil && strings.EqualFold(strings.TrimSpace(p.UpsertStop.List), "route") {
		after := strings.TrimSpace(p.UpsertStop.After)
		before := strings.TrimSpace(p.UpsertStop.Before)
		if after != "" || before != "" {
			if routePositionalWouldChangeEndpoints(beforeYAML, p.UpsertStop.Day, after, before) {
				overnightTouch = true
				otherStructural = append(otherStructural,
					fmt.Sprintf("upsert_stop list=route before/after would change day %d start or end", p.UpsertStop.Day))
			}
			// Mid-route insert next to an existing place is allowed (no keyword heuristics).
		} else if wouldTouchOvernightEnd(beforeYAML, p.UpsertStop.Day, p.UpsertStop.Place) {
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

	if !routeListVenue && !overnightTouch && !insertDay && !stopsFullReplace && len(otherStructural) == 0 {
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

	_ = userAsk
	switch {
	case stopsFullReplace && !overnightTouch && !insertDay && !routeListVenue && len(otherStructural) == countStopsOnly(otherStructural):
		b.WriteString(
			"Do not replace the whole days.N.stops array (that can drop existing pubs/stops). " +
				"Add or update one venue with places + upsert_stop (list:\"stops\"). " +
				"To remove one stop use remove_stop with exact place ids from getTripYAML",
		)
	case routeListVenue && !overnightTouch && !insertDay && onlyRouteVenue(otherStructural, stopsFullReplace):
		b.WriteString(
			"Appending a new place to list=route changes the overnight. " +
				"If the place belongs in drive order, retry the same patchTrip with list:\"route\" and " +
				"before or after set to an existing route place id from getTripYAML " +
				"(first and last of the day stay unchanged). " +
				"If it is a side/evening venue with no sequence, use list:\"stops\" instead. " +
				"Do not rewrite the day's route or call replaceDayRoutes for a single insert",
		)
	case insertDay && !overnightTouch && len(otherStructural) == 0 && !routeListVenue:
		b.WriteString(
			"insert_day is blocked in viewer chat. To add a venue/stop use patchTrip with places + " +
				"upsert_stop (list:\"stops\"). To change an overnight use changeOvernight. " +
				"Do not invent new days unless the user asked to insert a day",
		)
	default:
		if stopsFullReplace {
			b.WriteString("For stops: use upsert_stop/remove_stop, not days.N.stops full replace. ")
		}
		if routeListVenue {
			b.WriteString(
				"For a sequenced drive insert: list:\"route\" plus before/after an existing route place id. " +
					"For a side venue: list:\"stops\". ",
			)
		}
		if insertDay {
			b.WriteString("insert_day is blocked — use upsert_stop on stops for venues. ")
		}
		if overnightTouch || hasRouteReplace(otherStructural) {
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

func countStopsOnly(parts []string) int {
	n := 0
	for _, p := range parts {
		if strings.Contains(p, ".stops (full stops replace)") {
			n++
		}
	}
	return n
}

func onlyRouteVenue(other []string, stopsFull bool) bool {
	return !stopsFull && len(other) == 0
}

func hasRouteReplace(parts []string) bool {
	for _, p := range parts {
		if strings.Contains(p, ".route (full route replace)") {
			return true
		}
	}
	return false
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

func routePositionalWouldChangeEndpoints(yaml []byte, day int, after, before string) bool {
	trip, err := itinerary.ParseYAML(yaml)
	if err != nil {
		return true
	}
	d := findDay(trip, day)
	if d == nil || len(d.Route) == 0 {
		return true
	}
	first := strings.TrimSpace(d.Route[0].Place)
	last := strings.TrimSpace(d.Route[len(d.Route)-1].Place)
	if a := strings.TrimSpace(after); a != "" && a == last {
		return true
	}
	if b := strings.TrimSpace(before); b != "" && b == first {
		return true
	}
	return false
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

func dayByNumber(trip itinerary.Trip, day int) (itinerary.Day, bool) {
	for i := range trip.Days {
		if trip.Days[i].Day == day {
			return trip.Days[i], true
		}
	}
	return itinerary.Day{}, false
}
