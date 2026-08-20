# Results — suite MT on Eino + MCP · gpt-5-mini

**Date:** 2026-08-20  
**Candidate:** `experiments/eino-mcp` · Eino `AgenticModel` Responses + `EnableAutoCache` · **no** ADK `input_text` rewrite  
**Deps:** eino v0.9.15 · agenticopenai v0.2.2 · tool/mcp v0.0.9  
**Scratch:** `adk-eval` (restored per scenario)  
**Scenarios:** [`../adk-mcp/suite-mt/`](../adk-mcp/suite-mt/)  
**Logs:** `runs/mt/MT*.jsonl`  
**Compare:** [`../adk-mcp/RESULTS-mt.md`](../adk-mcp/RESULTS-mt.md) (ADK + rewrite)

## Scorecard

| ID | Heuristic | Human | Notes |
|----|-----------|-------|-------|
| MT01 | **pass** | **pass** | Second-option from active list; clarifies bare “second option” |
| MT02 | **pass** | **pass** | Final date `2026-09-15` (ADK had heuristic fail on early read only) |
| MT03 | **pass** | **pass** | Stale YAML facts correctly identified after writes |
| MT04 | **pass** | **pass** | Committed title on `adk-eval` |
| MT05 | **pass** | **pass** | Only Idea A written after approve |
| MT06 | **pass** | **pass** | Later-wins + structured enrichment preference restored |
| MT07 | **pass** | **pass** | Corrected id path; verify-before-write on `alpha-base` |
| MT08 | **pass** | **pass** | Constraints recalled after digression; titles-only write |
| MT09 | **pass** | **pass** | Decided vs suggested; toilets after approve (ADK heuristic fail on early read) |
| MT10 | **pass** | **pass*** | Undo via `restoreVersion`; *beta rain warning absent after undo* (over-restore or missed write) |

**Tally:** heuristic **10/10** · human **9 pass / 1 soft** (MT10 warning hygiene).

## vs ADK (+ rewrite)

| | ADK + rewrite | Eino stock |
|--|---------------|------------|
| Multi-turn connector | Needs HTTP rewrite (#1197) | Native `previous_response_id` / cache |
| Heuristic MT | 8/10 | **10/10** |
| Human context traps | 10/10 | ~10/10 (MT10 soft) |
| Wall time / scenario | ~110–200 s | ~85–130 s (this run) |

## Takeaway

For this experiment’s goal (accumulating context over MCP itinerary edits), **Eino is the stronger Go framework today**: stock Responses multi-turn works, and suite MT matches or beats ADK-without-shim on the same scripts. Remaining product gap is agent/tool hygiene (e.g. undo-latest scope), not the conversation pipe.
