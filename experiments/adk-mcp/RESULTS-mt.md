# Results — suite MT (multi-turn context) · gpt-5-mini

**Date:** 2026-08-20  
**Model:** `gpt-5-mini` · ADK MCP experiment + local Responses `input_text`→`output_text` rewrite ([adk-go#1197](https://github.com/google/adk-go/issues/1197))  
**Scratch:** `adk-eval` (restored at scenario start)  
**Scenarios:** [suite-mt/](suite-mt/)  
**Logs:** `runs/mt/MT*.jsonl` (gitignored)

Caveat: scores are **ADK + workaround**, not stock ADK multi-turn and not native Responses reasoning-item chaining.

## Scorecard

| ID | Focus | Wall ms | Heuristic | Human | Notes |
|----|-------|---------|-----------|-------|-------|
| MT01 | Rejected Italy → “second option” | 131709 | **pass** | **pass** | Wrote from current option set; final turn clarifies ambiguous “second option” |
| MT02 | Dates changed twice | 147933 | **fail*** | **pass** | *Only* `no_tools_until_turn` (early read of start). Final date `2026-09-15` in write; no stale `09-01`/`09-10` |
| MT03 | Stale YAML after write | 132083 | **pass** | **pass** | Re-read after writes; listed first-YAML facts no longer true |
| MT04 | Two trips / “that trip” | 122257 | **pass** | **pass** | Clarified then patched `adk-eval` only; final answer `adk-eval` |
| MT05 | Explore then approve one | 136642 | **pass** | **pass** | No tools until approve; only Idea A (`toilets` on alpha) |
| MT06 | Later instruction wins | 128637 | **pass** | **pass** | Day-3 notes then structured beta warning; articulated later-wins |
| MT07 | Failed id then fix | 111661 | **pass** | **pass** | No retained `alpha-base` / `alpha-coast` in mutate args; verify-before-write on retry |
| MT08 | Early constraint after digression | 142370 | **pass** | **pass** | Recalled no-delete / no-restore / titles-only; day 1 title only |
| MT09 | Decided vs suggested | 198445 | **fail*** | **pass** | *Only* early reads before approve. Separated decisions/suggestions; toilets written after approve |
| MT10 | Ambiguous final + undo latest | 105604 | **pass** | **pass*** | Pending title → day 5; `restoreVersion` for undo-latest; beta rain warning still present after undo |

**Tally:** heuristic 8 pass / 2 fail (both check-strictness on early reads) · human **10 pass** (MT10 minor: day 5 title back to seed after undo, as intended).

## Takeaways

- Under the wire rewrite, ADK+`gpt-5-mini` handled the designed context traps well: option-set freshness, date precedence, stale YAML, two-trip pronouns, explore-then-approve, later-wins, id correction, digression+constraints, decided-vs-suggested, undo-latest.
- The two heuristic fails are **not** context failures — the agent read trip state before the script’s “no tools yet” gate. Scenario checks for MT02 were loosened afterward; MT09 still allows optional early reads in human scoring.
- Still **not** evidence that unpatched ADK-Go is multi-turn capable, nor that reasoning-item replay matches ChatGPT Agent.
