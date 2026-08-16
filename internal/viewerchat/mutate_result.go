package viewerchat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
)

// mutateResult is the enriched JSON shape returned by chat mutate tools.
type mutateResult struct {
	OK             bool              `json:"ok"`
	Op             string            `json:"op"`
	ID             string            `json:"id,omitempty"`
	VersionID      string            `json:"version_id,omitempty"`
	BundleOK       bool              `json:"bundle_ok"`
	Changed        map[string]any    `json:"changed,omitempty"`
	DerivedChanges map[string]any    `json:"derived_changes,omitempty"`
	Preserved      []string          `json:"preserved,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
	TripFragment   *TripFragment     `json:"trip_fragment,omitempty"`
	Invariants     invariantsReport  `json:"invariants"`
	Extra          map[string]any    `json:"-"` // merged at marshal time
}

type invariantsReport struct {
	ContinuityOK bool     `json:"continuity_ok"`
	Issues       []string `json:"issues"`
}

func enrichAfterMutate(body []byte, viewerDay int, dayNums []int, base mutateResult) (string, error) {
	// Never scan the whole trip: empty dayNums with a viewer day → that neighborhood only.
	// Empty + no viewer day → skip continuity (avoid spuriously failing enrichment mutates).
	checkDays := dayNums
	if len(checkDays) == 0 && viewerDay > 0 {
		checkDays = []int{viewerDay}
	}
	var issues []string
	if len(checkDays) > 0 {
		issues = ContinuityWarnings(body, checkDays)
	}
	base.Invariants = invariantsReport{
		ContinuityOK: len(issues) == 0,
		Issues:       issues,
	}
	if len(issues) > 0 {
		base.Warnings = append(base.Warnings, issues...)
	}
	fragDay := firstDayOr(viewerDay, checkDays)
	if fragDay >= 1 {
		if frag, err := BuildTripFragment(body, fragDay); err == nil && len(frag.Days) > 0 {
			base.TripFragment = &frag
		}
	}
	return marshalMutateResult(base)
}

func marshalMutateResult(base mutateResult) (string, error) {
	m := map[string]any{
		"ok":         base.OK,
		"op":         base.Op,
		"bundle_ok":  base.BundleOK,
		"invariants": base.Invariants,
	}
	if base.ID != "" {
		m["id"] = base.ID
	}
	if base.VersionID != "" {
		m["version_id"] = base.VersionID
	}
	if len(base.Changed) > 0 {
		m["changed"] = base.Changed
	}
	if len(base.DerivedChanges) > 0 {
		m["derived_changes"] = base.DerivedChanges
	}
	if len(base.Preserved) > 0 {
		m["preserved"] = base.Preserved
	}
	if len(base.Warnings) > 0 {
		m["warnings"] = base.Warnings
	}
	if base.TripFragment != nil {
		m["trip_fragment"] = base.TripFragment
	}
	for k, v := range base.Extra {
		m[k] = v
	}
	b, err := json.Marshal(m)
	return string(b), err
}

func patchSummaryChanged(before, after []byte, p itinerary.Patch) map[string]any {
	out := map[string]any{}
	if p.UpdateDay != nil {
		out["update_day"] = p.UpdateDay.Day
	}
	if p.UpsertStop != nil {
		out["upsert_stop"] = map[string]any{"day": p.UpsertStop.Day, "place": p.UpsertStop.Place, "list": p.UpsertStop.List}
	}
	if p.RemoveStop != nil {
		out["remove_stop"] = map[string]any{"day": p.RemoveStop.Day, "list": p.RemoveStop.List}
	}
	if len(p.Places) > 0 {
		ids := make([]string, 0, len(p.Places))
		for id := range p.Places {
			ids = append(ids, id)
		}
		out["places"] = ids
	}
	_ = before
	_ = after
	if len(out) == 0 {
		return nil
	}
	return out
}

func patchDayNums(p itinerary.Patch) []int {
	seen := map[int]bool{}
	var out []int
	add := func(d int) {
		if d < 1 || seen[d] {
			return
		}
		seen[d] = true
		out = append(out, d)
	}
	if p.UpdateDay != nil {
		add(p.UpdateDay.Day)
	}
	if p.UpsertStop != nil {
		add(p.UpsertStop.Day)
	}
	if p.RemoveStop != nil {
		add(p.RemoveStop.Day)
	}
	for key := range p.Days {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(key), "%d", &n); err == nil {
			add(n)
		}
	}
	return out
}

func invariantsNeedRepair(content string) bool {
	var raw struct {
		Op         string            `json:"op"`
		Invariants *invariantsReport `json:"invariants"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return false
	}
	// Only structural route tools force a repair round. Enrichment patchTrip must not
	// digress into pre-existing continuity nits on other days.
	switch raw.Op {
	case "changeOvernight", "replaceDayRoutes":
	default:
		return false
	}
	if raw.Invariants == nil {
		return false
	}
	return !raw.Invariants.ContinuityOK || len(raw.Invariants.Issues) > 0
}

func repairDeveloperNote(mutateJSON string) string {
	return fmt.Sprintf(
		"HARNESS: mutation completed but itinerary invariants failed or need verification.\n"+
			"Tool result:\n%s\n"+
			"Repair ONLY the days named in changed/invariants for this mutation — do not rewrite unrelated days "+
			"(no ferry/other-day route surgery). Do not claim success until invariants.continuity_ok is true "+
			"(or explain the failure if you cannot repair).",
		mutateJSON,
	)
}
