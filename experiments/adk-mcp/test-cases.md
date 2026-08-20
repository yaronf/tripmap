# Experiment test cases

Paired runs: **baseline** = ChatGPT Agent (record app version + model) · **candidate** = this ADK binary (`OPENAI_MODEL`, default `gpt-4o`).

Concrete prompts for the 2026-08-20 run: [prompts.md](prompts.md).  
Candidate scorecard: [RESULTS.md](RESULTS.md).

Before mutating cases: pick a **scratch trip id**, note `listVersions` head (or restore to a known version). Use the same trip id and restore point for both sides.

Repeat nondeterministic prompts at least three times when comparing baselines.

| ID | Category | Prompt shape | Expected (rough) |
|----|----------|--------------|------------------|
| T01 | Simple read | List my tripmap trips. | `listTrips` only |
| T02 | Multi-step | For trip `SCRATCH`, show day 3 title and notes. | `getTrip` / `getTripYAML`; no writes |
| T03 | Tool choice | For `SCRATCH`, what schema fields exist for place info? | Prefer `getSchema` |
| T04 | Clarify | Move the overnight on day 5 somewhere nicer. | Ask which trip / destination; no `patchTrip` yet |
| T05 | No tool | In one sentence, what is an itinerary day in tripmap? | No MCP call |
| T06 | Impossible | Delete trip `does-not-exist-zzz` permanently. | Fail clearly / safely |
| T07 | Backend error | Call getTrip for id `___invalid___`. | Surface API error |
| T08 | Real workflow A | Warning on place via structured `info`, not notes. | `patchTrip` → `places.<id>.info.warnings` |
| T09 | Real workflow B | Change one day title only. | `patchTrip` → `update_day` |
| T10 | Versions | List versions then summarize latest. | `listVersions` then `getVersion`; no restore |

```bash
set -a && source ../../.env && set +a
export OPENAI_MODEL=gpt-4o
go run . --prompt 'List my tripmap trips.' --log runs/T01.jsonl
```
