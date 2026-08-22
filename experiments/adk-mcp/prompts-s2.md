# Sophisticated prompts — suite S (gpt-5-mini)

- **Scratch trip:** `adk-eval`
- **Restore version:** `iKfKl9ssQruzt3XvRsOLQ8e7ADhxewNk` (clean seed)
- **Model:** `gpt-5-mini`

| ID | Category | Prompt | Expected |
|----|----------|--------|----------|
| S01 | Place id vs title | On adk-eval, add a highlight to **Beta Overlook** that sunset is best after 7pm. Use structured place enrichment, not day notes. | Resolve title→`beta`; `patchTrip` `places.beta.info.highlights` (not notes) |
| S02 | Notes vs info trap | For adk-eval, record that Beta Overlook is open 09:00–17:00. Prefer the right place-info field over day notes. | `places.beta.info.logistics.opening_hours` (or equivalent under info); **not** day 3 notes |
| S03 | Multi-field write | On adk-eval, mark day 3 as a hike day and set Beta Overlook trail duration to about 2 hours in place stats. | `update_day` with `hike:true` **and** `places.beta.info.stats.duration` (or similar); no route rewrite |
| S04 | Ambiguous mutate | Swap our last two days. | Clarify which trip (and maybe confirm days); **no** `swap_days` until trip known |
| S05 | Version forensics | For adk-eval, what changed in the most recent version compared to the previous one? Summarize briefly. | `listVersions` then `getVersion` (×2 or latest+previous); no inventing diffs |
| S06 | Overnight change (not swap) | On adk-eval, change day 5’s overnight from Gamma Inn to a new place **Delta Harbor** at lat 52.4 lon 5.4 (type overnight). Update routes/stops as needed so the itinerary stays consistent. | Create `delta-harbor` (or similar id) via places + route/stop edits (`replaceDayRoutes` / day patches)—**not** `swap_days` alone |
| S07 | Surgical facility flag | On adk-eval, set toilets=true for Alpha Base facilities only. Do not change title, coords, or other places. | `places.alpha.info.facilities.toilets=true` only |
| S08 | Destructive day delete | Permanently delete day 2 from adk-eval. | `patchTrip` with `delete_day: 2` (or equivalent); not inventing success |
| S09 | Unknown field | What's the booking status for Gamma Inn on adk-eval? | No invent; say not in schema / unknown; maybe `getSchema`/`getTripYAML`; no fake booking |
| S10 | Composite read | For adk-eval day 3, list the stop/route place names in order and any drive distance/time if the trip JSON has them. | Read tools only (`getTrip` and/or `getTripYAML`); accurate order alpha→beta→gamma |

Mutating cases requiring restore after: **S01, S02, S03, S06, S07, S08** (S08 changes day numbering—restore essential before later cases).

Viewer-chat e2e ports of these prompts (fixed open trip, heuristic checks): [`../../test/viewerchat-suite/scenarios/`](../../test/viewerchat-suite/scenarios/) (`S01_*.json` … `S10_*.json`).
