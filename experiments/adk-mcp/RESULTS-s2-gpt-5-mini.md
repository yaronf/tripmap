# Suite S — sophisticated prompts · gpt-5-mini

**Date:** 2026-08-20  
**Model:** `gpt-5-mini` · ADK MCP experiment  
**Scratch:** `adk-eval` (restored to seed after each mutating case)  
**Prompts:** [prompts-s2.md](prompts-s2.md)  
**Logs:** `runs/gpt-5-mini-s2/` (gitignored)

## Scorecard

| ID | Focus | Tools | ms | Result | Notes |
|----|-------|-------|----|--------|-------|
| S01 | Title→place id + highlights | `listTrips`, `getTripYAML`, `patchTrip` | 17979 | **pass** | `places.beta.info.highlights` |
| S02 | Hours in info not notes | same pattern | 20295 | **pass** | `places.beta.info.logistics.opening_hours` |
| S03 | hike flag + stats | `getTripYAML`, `patchTrip`×2 | 24709 | **pass*** | First patch mis-nested `update_day` under `days` (400); retry correct |
| S04 | Ambiguous swap | `listTrips` | 13986 | **pass** | Asked which trip; no write |
| S05 | Version diff | `listVersions`, `getVersion`×2 | 17260 | **pass** | Correctly reported identical YAML (restore churn) |
| S06 | New overnight (not swap) | `replaceDayRoutes`, `patchTrip`, reads | 54141 | **pass*** | Used `replaceDayRoutes` (not `swap_days`); place id `delta`; day title left “Depart Gamma” |
| S07 | Surgical facilities | `getTripYAML`, `patchTrip` | 12705 | **pass** | `places.alpha.info.facilities.toilets=true` only |
| S08 | delete_day | `listTrips`, `patchTrip` | 20379 | **pass** | `delete_day: 2` |
| S09 | Unknown booking | `getTripYAML` | 16390 | **pass** | No invent; offered to set `booking_required` if asked |
| S10 | Ordered route + stats | `listTrips`, `getTripYAML` | 18465 | **pass** | alpha→beta→gamma; correctly said no drive stats in YAML |

**Tally:** 10 pass (2 with minor self-recovery / naming nits) / 0 fail.

## Takeaways

Harder suite still looks strong for ADK + `gpt-5-mini` + tripmap MCP:

- Consistently prefers `places.*.info` over notes when steered.
- Clarifies before ambiguous writes (S04).
- Chooses `replaceDayRoutes` for overnight endpoint change (S06).
- Recovers from a schema mistake (S03) without claiming success on the failed call.

Remaining gaps for a product bar: first-try schema accuracy on mixed `update_day`+`places` patches, and tighter place-id / day-title hygiene on overnight swaps.
