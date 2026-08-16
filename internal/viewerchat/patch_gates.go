package viewerchat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
)

// rejectChatStructuralPatch returns an actionable error when a chat patchTrip
// attempts overnight/endpoint or other structural edits that belong on
// changeOvernight / replaceDayRoutes.
func rejectChatStructuralPatch(patchJSON []byte, beforeYAML []byte) error {
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil {
		return nil // let ApplyPatch report decode errors
	}

	var blocked []string
	if p.SwapDays != nil {
		blocked = append(blocked, "swap_days")
	}
	if p.DeleteDay != nil {
		blocked = append(blocked, "delete_day")
	}
	if p.InsertDay != nil {
		blocked = append(blocked, "insert_day")
	}
	for key, raw := range p.Days {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, hasRoute := m["route"]; hasRoute {
			blocked = append(blocked, fmt.Sprintf("days.%s.route (full route replace)", key))
		}
	}
	if p.UpsertStop != nil && strings.EqualFold(strings.TrimSpace(p.UpsertStop.List), "route") {
		if wouldTouchOvernightEnd(beforeYAML, p.UpsertStop.Day, p.UpsertStop.Place) {
			blocked = append(blocked,
				fmt.Sprintf("upsert_stop on list=route for day %d (would change overnight/endpoint)", p.UpsertStop.Day))
		}
	}
	if p.RemoveStop != nil && (p.RemoveStop.List == "" || strings.EqualFold(p.RemoveStop.List, "route") || strings.EqualFold(p.RemoveStop.List, "both")) {
		if removeTouchesOvernight(beforeYAML, p.RemoveStop) {
			blocked = append(blocked,
				fmt.Sprintf("remove_stop on route for day %d (would change overnight/endpoint)", p.RemoveStop.Day))
		}
	}

	if len(blocked) == 0 {
		return nil
	}
	return fmt.Errorf(
		"chat patchTrip blocked structural edit: %s. "+
			"For overnight/endpoint changes use changeOvernight (preferred) or replaceDayRoutes. "+
			"patchTrip in chat is for places.*.info, update_day notes/title, mid-day stops list upsert/remove, and similar enrichment only",
		strings.Join(blocked, "; "),
	)
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
