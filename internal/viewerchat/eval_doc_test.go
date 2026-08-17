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
//   - Wrong-city place coords / missing maps_url: TestRejectWeakNewPlacesWrongCity
//   - Full days.N.stops replace blocked: TestRejectChatStructuralPatch
//   - Morning pick-up allows replaceDayRoutes: TestRejectReplaceUnlessRouteSurgery
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
//
// Case: Hayes remove → wrong id → upsert (2026-08-17 logs)
//   remove_stop with guessed id failed; model upserted a duplicate. Gates:
//   remove intent rejects upsert; remove requires getTripYAML first; missing-id
//   errors list place ids on the day.
//
// Case: Avis Wellington replaceDayRoutes cascade (2026-08-17 logs)
//   "as a stop" / vague Wellington follow-up must not call replaceDayRoutes;
//   replaceDayRoutes cannot rewrite day 8 while viewer is on day 6.
//   Follow-up: list=route reject must say retry list=stops — never steer to
//   replaceDayRoutes (TestRejectChatStructuralPatch).
//
// Case: Avis pick-up/return quality (2026-08-17 logs)
//   User: day 3 pick up Avis CBD; day 6 return Avis Wellington.
//   Failures: Wellington lat/lon on Auckland place; missing maps_url; morning
//   pick-up on stops: (timeline after mid-route sights); days.6.stops full
//   replace risk to pubs. Assert:
//     - new places: correct-city lat/lon + Google maps_url (TestRejectWeakNewPlacesWrongCity)
//     - morning pick-up → mid-route via replaceDayRoutes (keep endpoints)
//     - evening return → upsert_stop list stops (not full stops array)
//     - final ViewerDayStops order: Depart → Avis → … → overnight (day 3)
//
// Case: Avis before ferry misplaced (2026-08-17 logs)
//   Drop-off moved to day 7 on stops:; user "misplaced / before the ferry".
//   replaceDayRoutes was blocked; stops upsert cannot fix timeline order.
//   Assert: placement/before-ferry asks allow replaceDayRoutes; stops upsert
//   for those asks is rejected (TestRejectStopsWhenNeedsMidRoute).
