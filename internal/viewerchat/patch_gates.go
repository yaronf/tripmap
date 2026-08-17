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
		routeListVenue  bool // upsert on route for a new/endpoint-touching place → use stops
		overnightTouch  bool // true overnight/endpoint surgery
		insertDay       bool
		stopsFullReplace bool
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
		if _, hasStops := m["stops"]; hasStops {
			stopsFullReplace = true
			otherStructural = append(otherStructural, fmt.Sprintf("days.%s.stops (full stops replace)", key))
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

	askLower := strings.ToLower(userAsk)
	switch {
	case stopsFullReplace && !overnightTouch && !insertDay && !routeListVenue && len(otherStructural) == countStopsOnly(otherStructural):
		b.WriteString(
			"Do not replace the whole days.N.stops array (that can drop existing pubs/stops). "+
				"Add or update one venue with places + upsert_stop (list:\"stops\"). "+
				"To remove one stop use remove_stop with exact place ids from getTripYAML",
		)
	case routeListVenue && !overnightTouch && !insertDay && onlyRouteVenue(otherStructural, stopsFullReplace):
		if looksLikeOnDriveLogisticsAsk(askLower) {
			b.WriteString(
				"On-the-drive logistics (pick-up / before ferry) must appear on route in the day timeline. "+
					"Use replaceDayRoutes for that day only: keep existing start/end, insert the place in order "+
					"(e.g. after morning depart, or before wellington-ferry-terminal) with correct-city lat/lon + Google maps_url. "+
					"Do not use the stops list — viewer shows all stops after mid-route sights",
			)
		} else {
			b.WriteString(
				"RETRY the same patchTrip with upsert_stop.list set to \"stops\" (keep places + upsert_stop). "+
					"Evening returns (not before ferry), restaurants, bars, and side venues belong on the stops list — "+
					"not on route, and not by rewriting the day's drive or inserting a day",
			)
		}
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
				"For the venue/stop: retry patchTrip with upsert_stop.list=\"stops\" (not route), "+
					"unless this is a morning pick-up on the drive — then replaceDayRoutes mid-route insert. ",
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
