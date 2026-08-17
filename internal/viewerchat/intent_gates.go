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
			"remove intent: do not upsert_stop. Call getTripYAML, then patchTrip remove_stop " +
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

// rejectReplaceUnlessRouteSurgery keeps replaceDayRoutes for explicit overnight/drive
// rewrite only. Venue asks and vague follow-ups use patchTrip upsert_stop (list stops,
// or list route with before/after) — not a full route rewrite.
func rejectReplaceUnlessRouteSurgery(userAsk string) error {
	lower := strings.ToLower(strings.TrimSpace(userAsk))
	if lower == "" {
		return nil
	}
	if looksLikeRouteSurgeryAsk(lower) {
		return nil
	}
	if looksLikeEnrichmentStopAsk(lower) {
		return fmt.Errorf(
			"this ask is a single stop, not route surgery. " +
				"patchTrip with places + upsert_stop: list:\"stops\" for a side/evening venue, " +
				"or list:\"route\" with before/after an existing route place id from getTripYAML " +
				"when the stop belongs in drive order (first/last of the day stay unchanged). " +
				"Do not call replaceDayRoutes to insert one place",
		)
	}
	return fmt.Errorf(
		"replaceDayRoutes requires an explicit overnight or full-route rewrite. " +
			"For a stop use patchTrip places + upsert_stop (list:\"stops\", or list:\"route\" with before/after). " +
			"Do not rewrite neighboring days while answering a stop request",
	)
}

func looksLikeEnrichmentStopAsk(lower string) bool {
	if looksLikeRouteSurgeryAsk(lower) {
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

// looksLikeRouteSurgeryAsk: explicit overnight or full drive rewrite, not phrase-matching
// "before the ferry" / "pick up".
func looksLikeRouteSurgeryAsk(lower string) bool {
	for _, needle := range []string{
		"overnight", "change the route", "rewrite the route",
		"reorder", "swap day", "endpoint", "replace day", "driving route",
		"change overnight", "new overnight",
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
