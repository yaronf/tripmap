package viewerchat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaronf/tripmap/internal/itinerary"
)

func TestRejectChatStructuralPatch(t *testing.T) {
	yaml := []byte(`
trip: T
schema_version: 2
places:
  a: {title: A, type: overnight}
  b: {title: B, type: overnight}
  c: {title: C, type: attraction}
days:
  - day: 1
    title: One
    route: [{place: a, type: overnight}, {place: b, type: overnight}]
`)
	err := rejectChatStructuralPatch([]byte(`{"days":{"1":{"route":[{"place":"a"},{"place":"c"}]}}}`), yaml, "")
	if err == nil || !strings.Contains(err.Error(), "changeOvernight") {
		t.Fatalf("err=%v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"upsert_stop":{"day":1,"list":"route","place":"c"}}`), yaml, "add a cafe")
	if err == nil || !strings.Contains(err.Error(), `list set to "stops"`) {
		t.Fatalf("new place on route should steer to stops, err=%v", err)
	}
	if strings.Contains(err.Error(), "replaceDayRoutes") {
		t.Fatalf("must not steer venue upsert to replaceDayRoutes: %v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"upsert_stop":{"day":1,"list":"route","place":"c"}}`), yaml, "pick up rental car from Avis CBD")
	if err == nil || !strings.Contains(err.Error(), "replaceDayRoutes") || !strings.Contains(err.Error(), "mid-route") {
		t.Fatalf("morning pick-up should steer to replaceDayRoutes mid-route, err=%v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"upsert_stop":{"day":1,"list":"route","place":"b"}}`), yaml, "")
	if err == nil || !strings.Contains(err.Error(), "changeOvernight") {
		t.Fatalf("endpoint upsert should steer to changeOvernight, err=%v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"insert_day":{"after":1,"day":{"title":"X"}}}`), yaml, "")
	if err == nil || !strings.Contains(err.Error(), "stops") {
		t.Fatalf("insert_day should steer to stops upsert, err=%v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"days":{"1":{"stops":[{"place":"c"}]}}}`), yaml, "add avis")
	if err == nil || !strings.Contains(err.Error(), "upsert_stop") {
		t.Fatalf("full stops replace should be blocked, err=%v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"upsert_stop":{"day":1,"list":"stops","place":"c"}}`), yaml, "")
	if err != nil {
		t.Fatalf("stops upsert should be allowed: %v", err)
	}
	err = rejectChatStructuralPatch([]byte(`{"update_day":{"day":1,"notes":"hi"}}`), yaml, "")
	if err != nil {
		t.Fatalf("update_day notes should be allowed: %v", err)
	}
}

func TestLooksLikeCancel(t *testing.T) {
	for _, s := range []string{"nm", "never mind", "You added Huka Falls, not exactly what I asked for, but NM."} {
		if !looksLikeCancel(strings.ToLower(s)) {
			t.Fatalf("expected cancel: %q", s)
		}
	}
	if looksLikeCancel("add a stop for coffee") {
		t.Fatal("should not cancel")
	}
}

func TestInvariantsNeedRepairOnlyStructural(t *testing.T) {
	enrich := `{"ok":true,"op":"patchTrip","invariants":{"continuity_ok":false,"issues":["day 10 != day 11"]}}`
	if invariantsNeedRepair(enrich) {
		t.Fatal("patchTrip must not force repair on continuity nits")
	}
	structural := `{"ok":true,"op":"changeOvernight","invariants":{"continuity_ok":false,"issues":["day 4 != day 5"]}}`
	if !invariantsNeedRepair(structural) {
		t.Fatal("changeOvernight should force repair when continuity fails")
	}
	ok := `{"ok":true,"op":"replaceDayRoutes","invariants":{"continuity_ok":true,"issues":[]}}`
	if invariantsNeedRepair(ok) {
		t.Fatal("clean structural result should not repair")
	}
}

// Hayes Common lunch regression: trip has a distant continuity break; enrichment
// on an unrelated day must not inherit that failure or force a repair digression.
func TestEnrichScopedContinuity(t *testing.T) {
	body := []byte(`
trip: T
schema_version: 2
places:
  a: {title: A, type: overnight}
  b: {title: B, type: overnight}
  c: {title: C, type: overnight}
  d: {title: D, type: overnight}
  cafe: {title: Hayes Common, type: food}
days:
  - day: 1
    title: One
    route: [{place: a, type: overnight}, {place: b, type: overnight}]
  - day: 2
    title: Two
    route: [{place: c, type: overnight}, {place: d, type: overnight}]
  - day: 3
    title: Three
    route: [{place: d, type: overnight}, {place: a, type: overnight}]
    stops: [{place: cafe, type: food}]
`)
	// Pre-existing: day1 end b != day2 start c. Day 3 neighborhood is fine.
	distant := ContinuityWarnings(body, []int{1})
	if len(distant) == 0 {
		t.Fatal("fixture needs distant mismatch on day 1→2")
	}
	if len(ContinuityWarnings(body, nil)) != 0 {
		t.Fatal("empty dayNums must not scan whole trip")
	}
	scoped := ContinuityWarnings(body, []int{3})
	if len(scoped) != 0 {
		t.Fatalf("day-3 scope should be clean, got %v", scoped)
	}

	out, err := enrichAfterMutate(body, 3, []int{3}, mutateResult{
		OK: true, Op: "patchTrip", BundleOK: true,
		Changed: map[string]any{"upsert_stop": map[string]any{"day": 3, "place": "cafe"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if invariantsNeedRepair(out) {
		t.Fatalf("enrichment must not force repair: %s", out)
	}
	var raw struct {
		Invariants invariantsReport `json:"invariants"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	if !raw.Invariants.ContinuityOK || len(raw.Invariants.Issues) != 0 {
		t.Fatalf("want continuity_ok on day-3 enrich, got %+v", raw.Invariants)
	}

	p := itinerary.Patch{UpsertStop: &itinerary.UpsertStop{Day: 3, Place: "cafe", List: "stops"}}
	if got := patchDayNums(p); len(got) != 1 || got[0] != 3 {
		t.Fatalf("patchDayNums=%v", got)
	}
}
