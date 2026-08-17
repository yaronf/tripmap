package viewerchat

import (
	"strings"
	"testing"
)

func TestRejectRemoveIntentMisuse(t *testing.T) {
	ask := "Remove the Hayes place."
	err := rejectRemoveIntentMisuse([]byte(`{"upsert_stop":{"day":3,"list":"stops","place":"hayes-common"}}`), ask)
	if err == nil || !strings.Contains(err.Error(), "do not upsert_stop") {
		t.Fatalf("err=%v", err)
	}
	err = rejectRemoveIntentMisuse([]byte(`{"remove_stop":{"day":3,"list":"stops","places":["hayes-common-hamilton"]}}`), ask)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRejectRemoveWithoutYAML(t *testing.T) {
	tc := &turnToolContext{}
	err := rejectRemoveWithoutYAML([]byte(`{"remove_stop":{"day":3,"places":["x"]}}`), tc)
	if err == nil || !strings.Contains(err.Error(), "getTripYAML") {
		t.Fatalf("err=%v", err)
	}
	tc.SawGetTripYAML = true
	if err := rejectRemoveWithoutYAML([]byte(`{"remove_stop":{"day":3,"places":["x"]}}`), tc); err != nil {
		t.Fatal(err)
	}
}

func TestRejectReplaceUnlessRouteSurgery(t *testing.T) {
	err := rejectReplaceUnlessRouteSurgery("Day 6: can't we have it as a stop? Assume this is Avis.")
	if err == nil || !strings.Contains(err.Error(), "enrichment") {
		t.Fatalf("err=%v", err)
	}
	err = rejectReplaceUnlessRouteSurgery("you're confused, we're talking Wellington here.")
	if err == nil {
		t.Fatal("vague follow-up should still block bare replaceDayRoutes")
	}
	if err := rejectReplaceUnlessRouteSurgery("Change day 10 overnight to Greymouth"); err != nil {
		t.Fatal(err)
	}
	if err := rejectReplaceUnlessRouteSurgery("day 3: pick up rental car from Avis CBD"); err != nil {
		t.Fatalf("pick-up should allow replaceDayRoutes: %v", err)
	}
	if err := rejectReplaceUnlessRouteSurgery("why is the day 7 stop misplaced? Should be before the ferry."); err != nil {
		t.Fatalf("before-ferry placement should allow replaceDayRoutes: %v", err)
	}
	if err := rejectReplaceUnlessRouteSurgery("move the drop off to day 7, near the ferry"); err != nil {
		t.Fatalf("near the ferry should allow replaceDayRoutes: %v", err)
	}
}

func TestRejectStopsWhenNeedsMidRoute(t *testing.T) {
	err := rejectStopsWhenNeedsMidRoute(
		[]byte(`{"upsert_stop":{"day":3,"list":"stops","place":"avis-auckland","notes":"Pick up Avis","type":"rental"}}`),
		"Add Avis car pick up on day 3, drop off day 6 evening.",
	)
	if err == nil || !strings.Contains(err.Error(), "replaceDayRoutes") {
		t.Fatalf("pick-up on stops should be rejected: %v", err)
	}
	err = rejectStopsWhenNeedsMidRoute(
		[]byte(`{"upsert_stop":{"day":7,"list":"stops","place":"avis-wellington","type":"rental"}}`),
		"why is the day 7 stop misplaced? Should be before the ferry.",
	)
	if err == nil || !strings.Contains(err.Error(), "before ferry") {
		t.Fatalf("placement on stops should be rejected: %v", err)
	}
	err = rejectStopsWhenNeedsMidRoute(
		[]byte(`{"upsert_stop":{"day":6,"list":"stops","place":"avis-wellington","notes":"Drop off evening","type":"rental"}}`),
		"drop off day 6 evening",
	)
	if err != nil {
		t.Fatalf("evening drop-off on stops should be allowed: %v", err)
	}
}

func TestRejectReplaceOutsideAskScope(t *testing.T) {
	err := rejectReplaceOutsideAskScope([]int{6, 7, 8}, 6, "you're confused, we're talking Wellington here.")
	if err == nil || !strings.Contains(err.Error(), "8") {
		t.Fatalf("want day 8 blocked, err=%v", err)
	}
	if err := rejectReplaceOutsideAskScope([]int{6, 7}, 6, "fix day 6 route via Palmerston"); err != nil {
		t.Fatal(err)
	}
}
