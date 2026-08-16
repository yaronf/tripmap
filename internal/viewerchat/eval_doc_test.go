package viewerchat_test

// End-state harness cases (scripted; no live model).
// Real failures encoded as unit tests elsewhere:
//   - Day vs stop / logistics: itinerary patch strict decode
//   - Coffee stop ≠ rewrite endpoints: TestRejectChatStructuralPatch
//   - NM = no mutate: TestLooksLikeCancel
//   - Overnight preserves next mid: TestApplyChangeOvernightPreservesNextMid
//   - Absurd overnight distance: TestApplyChangeOvernightRejectsAbsurdDistance
//   - Day-scoped YAML: TestBuildDayScopedYAML / TestHandleGetTripYAMLScope
//   - Enrichment must not trip on distant continuity: TestEnrichScopedContinuity /
//     TestInvariantsNeedRepairOnlyStructural
//   - Remove after add must not reenact add: TestNeutralizePriorAddBeforeRemove
//
// Case: "Hayes Common lunch" (2026-08-17)
//   User: add lunch stop on Day 3 (enrichment upsert_stop). Patch succeeds.
//   Bug: enrichAfterMutate scanned whole-trip continuity; pre-existing mismatches
//   elsewhere → continuity_ok=false → harness forced repair → model narrated
//   "informational issues with other days…" and truncated mid-sentence.
//   Assert (when scripted): final YAML has the stop; answer mentions Day 3 only;
//   no repair round; no offer to fix unrelated continuity.
//
// Case: "Remove the restaurant" after add (2026-08-17)
//   Same thread: user asks to remove; model replied that it had just added it
//   (sticky prior assistant claim). Assert: remove_stop (or restore); answer is
//   about removal; curated prior assistant mutation claims are neutralized.
