package viewerchat

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
)

// turnToolContext tracks per-turn tool usage for harness gates.
type turnToolContext struct {
	SawGetTripYAML bool
}

func latestUserAsk(msgs []ClientMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(msgs[i].Role), "user") {
			return strings.TrimSpace(msgs[i].Content)
		}
	}
	return ""
}

// rejectRemoveIntentMisuse blocks re-adding when the user asked to remove.
func rejectRemoveIntentMisuse(patchJSON []byte, userAsk string) error {
	if !looksLikeRemoveAsk(strings.ToLower(userAsk)) {
		return nil
	}
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil {
		return nil
	}
	if p.UpsertStop != nil {
		return fmt.Errorf(
			"remove intent: do not upsert_stop. Call getTripYAML, then patchTrip remove_stop "+
				"with the exact place id(s) from that day's stops/route (list usually \"stops\")",
		)
	}
	if len(p.Places) > 0 && p.RemoveStop == nil && p.UpdateDay == nil {
		return fmt.Errorf(
			"remove intent: do not create places. Call getTripYAML, then remove_stop with existing place ids",
		)
	}
	return nil
}

func rejectRemoveWithoutYAML(patchJSON []byte, tc *turnToolContext) error {
	if tc == nil || tc.SawGetTripYAML {
		return nil
	}
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil {
		return nil
	}
	if p.RemoveStop == nil {
		return nil
	}
	return fmt.Errorf(
		"before remove_stop, call getTripYAML (day-scoped) and copy exact place id(s) from stops/route — " +
			"do not guess ids or upsert a replacement",
	)
}

// rejectReplaceUnlessRouteSurgery keeps replaceDayRoutes for explicit route work only.
// Venue/stop asks (and vague follow-ups like "we're talking Wellington") must use
// patchTrip upsert_stop on list "stops" — not rewrite whole day routes.
// Exception: morning pick-up / before-ferry / timeline placement need mid-route insert.
func rejectReplaceUnlessRouteSurgery(userAsk string) error {
	lower := strings.ToLower(strings.TrimSpace(userAsk))
	if lower == "" {
		return nil
	}
	if looksLikeRouteSurgeryAsk(lower) || looksLikeOnDriveLogisticsAsk(lower) {
		return nil
	}
	if looksLikeEnrichmentStopAsk(lower) {
		return fmt.Errorf(
			"this ask is an enrichment/side stop, not route surgery. "+
				"Evening returns (not before ferry), restaurants, bars: patchTrip with places + upsert_stop (list:\"stops\"). "+
				"Morning pick-up or drop-off before ferry/boarding: replaceDayRoutes mid-route insert "+
				"(keep start/end; insert logistics in timeline order). "+
				"Do not call replaceDayRoutes to dump a venue onto an unrelated day's ferry/route",
		)
	}
	return fmt.Errorf(
		"replaceDayRoutes requires an explicit route/overnight ask or on-the-drive logistics "+
			"(e.g. change overnight, pick up after morning depart, drop off before the ferry, fix misplaced stop on the drive). "+
			"For evening/side venues use patchTrip places + upsert_stop with list:\"stops\". "+
			"Do not rewrite neighboring days while answering a stop request",
	)
}

// rejectStopsWhenNeedsMidRoute blocks putting pick-up / before-ferry logistics on
// the stops list (viewer shows all stops: after mid-route sights — so "before ferry"
// is impossible via stops).
func rejectStopsWhenNeedsMidRoute(patchJSON []byte, userAsk string) error {
	lower := strings.ToLower(userAsk)
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil || p.UpsertStop == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(p.UpsertStop.List), "stops") {
		return nil
	}
	notes := strings.ToLower(p.UpsertStop.Notes)
	if looksLikeTimelinePlacementAsk(lower) {
		return fmt.Errorf(
			"timeline placement (before ferry / misplaced on the drive) cannot use list:\"stops\" — "+
				"the viewer always shows stops after mid-route sights. "+
				"Use replaceDayRoutes for that day only: keep existing endpoints, insert the place on route "+
				"in the correct order (e.g. Avis before wellington-ferry-terminal), remove it from stops",
		)
	}
	if looksLikePickUpAsk(lower) && (strings.Contains(notes, "pick") || strings.Contains(notes, "collect") ||
		strings.Contains(notes, "hire")) {
		return fmt.Errorf(
			"morning pick-up must not use list:\"stops\" (it would appear after mid-route sights). "+
				"Use replaceDayRoutes for that day: keep start/end overnights, insert the rental mid-route "+
				"right after morning depart (correct-city lat/lon + Google maps_url)",
		)
	}
	return nil
}

func looksLikeEnrichmentStopAsk(lower string) bool {
	if looksLikeRouteSurgeryAsk(lower) || looksLikeOnDriveLogisticsAsk(lower) {
		return false
	}
	for _, needle := range []string{
		"as a stop", "as stop", "lunch", "dinner", "breakfast", "brunch",
		"restaurant", "wine bar", "wine ", "cafe", "coffee", "pub", "bar",
		"avis", "rental car", "rental desk", "return the rental", "return rental",
		"return car", "recommend", "recommendation", "add a ", "add the ",
		"drop off", "drop-off", "dropoff",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// looksLikeOnDriveLogisticsAsk: logistics that must sit on route for correct timeline order.
func looksLikeOnDriveLogisticsAsk(lower string) bool {
	return looksLikePickUpAsk(lower) || looksLikeTimelinePlacementAsk(lower)
}

func looksLikePickUpAsk(lower string) bool {
	for _, needle := range []string{
		"pick up", "pickup", "pick-up", "collect the rental", "collect rental",
		"get the rental", "hire the car", "hire car",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// looksLikeTimelinePlacementAsk: user wants a stop earlier on the day/drive (before ferry, etc.).
func looksLikeTimelinePlacementAsk(lower string) bool {
	for _, needle := range []string{
		"before the ferry", "before ferry", "before the terminal", "before boarding",
		"near the ferry", "near ferry", "misplaced", "should be before", "put it before",
		"move it before", "on the route before", "before picton", "before wellington-ferry",
		"drop off before", "drop-off before", "return before",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func looksLikeRouteSurgeryAsk(lower string) bool {
	for _, needle := range []string{
		"overnight", "change the route", "rewrite the route", "via ",
		"reorder", "swap day", "endpoint", "replace day", "driving route",
		"change overnight", "new overnight", "mid-route", "on the drive",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

var dayMentionRe = regexp.MustCompile(`(?i)\bday\s*(\d+)\b`)

func dayNumsMentioned(userAsk string) []int {
	matches := dayMentionRe.FindAllStringSubmatch(userAsk, -1)
	seen := map[int]bool{}
	var out []int
	for _, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// rejectReplaceOutsideAskScope blocks rewriting days far from the viewer/ask
// (Avis Wellington turn rewrote day 7–8 ferry after a day-6 stop ask).
func rejectReplaceOutsideAskScope(dayNums []int, viewerDay int, userAsk string) error {
	if len(dayNums) == 0 {
		return nil
	}
	allowed := map[int]bool{}
	mentioned := dayNumsMentioned(userAsk)
	for _, n := range mentioned {
		allowed[n] = true
		allowed[n+1] = true
		if n > 1 {
			allowed[n-1] = true
		}
	}
	if viewerDay > 0 {
		allowed[viewerDay] = true
		allowed[viewerDay+1] = true
		if viewerDay > 1 {
			allowed[viewerDay-1] = true
		}
	}
	if len(allowed) == 0 {
		if len(dayNums) <= 2 {
			return nil
		}
		return fmt.Errorf(
			"replaceDayRoutes touches days %v but no viewer day / day mention in the user ask — "+
				"limit to the day(s) the user named (at most day N and N+1 for continuity)",
			dayNums,
		)
	}
	var blocked []int
	for _, d := range dayNums {
		if !allowed[d] {
			blocked = append(blocked, d)
		}
	}
	if len(blocked) == 0 {
		return nil
	}
	return fmt.Errorf(
		"replaceDayRoutes blocked for day(s) %v — outside this turn's scope "+
			"(viewer day %d; user-mentioned days %v; allowed neighborhood %v). "+
			"Only edit the day the user asked about (and at most ±1 for route continuity). "+
			"Do not rewrite ferry/unrelated days while fixing a stop on another day",
		blocked, viewerDay, mentioned, sortedIntKeys(allowed),
	)
}

func sortedIntKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
