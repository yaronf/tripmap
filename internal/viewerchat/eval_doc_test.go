package viewerchat_test

// End-state harness cases (scripted; no live model).
// Real failures encoded as unit tests elsewhere:
//   - Day vs stop / logistics: itinerary patch strict decode
//   - Coffee stop ≠ rewrite endpoints: TestRejectChatStructuralPatch
//   - NM = no mutate: TestLooksLikeCancel
//   - Overnight preserves next mid: TestApplyChangeOvernightPreservesNextMid
//   - Absurd overnight distance: TestApplyChangeOvernightRejectsAbsurdDistance
//   - Day-scoped YAML: TestBuildDayScopedYAML / TestHandleGetTripYAMLScope
