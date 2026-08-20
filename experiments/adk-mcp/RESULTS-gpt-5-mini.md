# Candidate results — ADK Go + gpt-5-mini

**Date:** 2026-08-20  
**Candidate:** `experiments/adk-mcp` · model `gpt-5-mini` · MCP `https://tripmap.sheffer.org/mcp`  
**Scratch trip:** `adk-eval` (restored after mutating cases)  
**Prompts:** [prompts.md](prompts.md) (same as gpt-4o run)  
**Logs:** `runs/gpt-5-mini/` (gitignored)  
**Compare with:** [RESULTS.md](RESULTS.md) (`gpt-4o`)

## Scorecard

| ID | Expected | Tools used | ms | Result | Notes |
|----|----------|------------|----|--------|-------|
| T01 | `listTrips` | `listTrips` | 8488 | **pass** | Listed raw ids; no invented titles (cleaner than gpt-4o) |
| T02 | day 3 title/notes | `getTripYAML` | 8194 | **pass** | Direct day scope; no extra tools |
| T03 | `getSchema` | `getSchema` | 12100 | **pass** | Detailed PlaceInfo fields |
| T04 | Clarify; no write | _(none)_ | 11068 | **pass** | Asked **which trip** + what “nicer” means; no MCP reads/writes |
| T05 | No tool | _(none)_ | 4838 | **pass** | |
| T06 | Fail safely | `listTrips` | 9248 | **pass** | Reported missing; offered delete of *existing* ids only with confirm |
| T07 | Surface getTrip error | `getTrip` | 9116 | **pass** | Called `___invalid___.` (extra `.`); still relayed 400 clearly |
| T08 | `places.beta.info.warnings` | `listTrips`, `getTripYAML`, `patchTrip` | 28470 | **pass** | Correct structured warning on `beta` |
| T09 | `update_day` title only | `listTrips`, `patchTrip` | 14695 | **pass** | Exact `update_day` title patch |
| T10 | versions then summary | `listVersions`, `getVersion` | 27014 | **pass** | No restore |

**Tally:** 10 pass / 0 partial / 0 fail.

## vs gpt-4o (same prompts)

| | gpt-4o | gpt-5-mini |
|--|--------|------------|
| Score | 9 pass / 1 partial | **10 pass** |
| T01 titles | Invented display names | Raw ids only |
| T04 clarify | Assumed `adk-eval`, asked destination prefs | Asked trip id **and** prefs; no tools |
| T02 tool noise | 3 tools | 1 tool |
| Latency | ~1.6–12.7 s | ~4.8–28 s (slower; more reasoning) |
| Critical writes | Correct | Correct |

## Conclusion

Inexpensive mainline **`gpt-5-mini`** on ADK+MCP matched or beat `gpt-4o` on this suite (especially clarify + honest listTrips). Cost/latency tradeoff: fewer wrong assumptions, higher wall time.

Follow-up harder suite (same model): [RESULTS-s2-gpt-5-mini.md](RESULTS-s2-gpt-5-mini.md) · [prompts-s2.md](prompts-s2.md).
