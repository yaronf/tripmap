# Experiment test cases

Paired runs: **baseline** = ChatGPT Agent (record app version + model) · **candidate** = this ADK binary (`OPENAI_MODEL`).

Before mutating cases: pick a **scratch trip id**, note `listVersions` head (or restore to a known version). Use the same trip id and restore point for both sides.

Repeat nondeterministic prompts **3×**. Score with the evaluation fields in [docs/adk-go-mcp-experiment.md](../../docs/adk-go-mcp-experiment.md).

Replace `SCRATCH_TRIP` and place/day placeholders before running.

| ID | Category | Prompt | Expected (rough) |
|----|----------|--------|------------------|
| T01 | Simple read | List my tripmap trips. | `listTrips` only; returns ids/titles |
| T02 | Multi-step | For trip `SCRATCH_TRIP`, show day 3 title and notes. | `getTrip` and/or `getTripYAML` (day scope); no writes |
| T03 | Tool choice | For `SCRATCH_TRIP`, what schema fields exist for place `info`? | Prefer `getSchema` (not inventing fields) |
| T04 | Clarify | Move the overnight on day 5 somewhere nicer. | Ask which trip / destination; **no** `patchTrip` until clarified |
| T05 | No tool | In one sentence, what is an itinerary day in tripmap? | Answer from instructions/knowledge; no MCP call |
| T06 | Impossible | Delete trip `does-not-exist-zzz` permanently. | Fail clearly; no silent invent; no unsafe guess |
| T07 | Backend error | Call getTrip for id `___invalid___`. | Surface tool/API error; no fake trip data |
| T08 | Real workflow A | For `SCRATCH_TRIP`, add a short warning on place `PLACE_ID` that the road may be closed after rain. Use structured place info, not day notes. | `patchTrip` / places.`PLACE_ID`.info.warnings (or equivalent); not stuffing into notes |
| T09 | Real workflow B | For `SCRATCH_TRIP`, change day `N` title only to `TITLE` and leave stops alone. | `patchTrip` with `update_day` (or equivalent); no route rewrite |
| T10 | Versions | For `SCRATCH_TRIP`, list recent versions then show the latest version summary. | `listVersions` then `getVersion`; no `restoreVersion` |

## Candidate one-shot examples

```bash
set -a && source .env && set +a
go run . --prompt 'List my tripmap trips.' --log /tmp/T01.jsonl
go run . --prompt 'For trip SCRATCH_TRIP, show day 3 title and notes.' --log /tmp/T02.jsonl
```

## Results

Copy into a spreadsheet or markdown table: case id, side, run #, pass/partial/fail, tools used, arg notes, latency, cost, comments.
