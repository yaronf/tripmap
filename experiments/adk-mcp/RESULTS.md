# Candidate results — ADK Go + gpt-4o (Responses API)

**Date:** 2026-08-20  
**Candidate:** `experiments/adk-mcp` · model `gpt-4o` · MCP `https://tripmap.sheffer.org/mcp`  
**Scratch trip:** `adk-eval` (created for this run; restored to clean YAML after T08/T09/T10)  
**Baseline (ChatGPT Agent):** not run in this session — fill when you have paired Agent logs.

Concrete prompts: [prompts.md](prompts.md). Raw JSONL: `runs/T0N.jsonl` (local; gitignored).

## Scorecard (candidate vs expected)

| ID | Expected | Tools used | ms | Result | Notes |
|----|----------|------------|----|--------|-------|
| T01 | `listTrips` | `listTrips` | 3948 | **pass*** | Correct IDs; *invented display titles* from ids (`Holland` etc.) — mild hallucination |
| T02 | get day 3 title/notes | `listTrips`, `getSchema`, `getTripYAML` | 5605 | **pass** | Correct “Alpha to Beta day” + notes; 2 extra reads |
| T03 | `getSchema` for PlaceInfo | `getSchema` | 5442 | **pass** | Answered from schema (facilities, highlights, links, …) |
| T04 | Clarify; no write | `listTrips`, `getTripYAML` | 6162 | **partial** | Asked for destination prefs; **assumed** trip `adk-eval` without asking which trip; no `patchTrip` ✓ |
| T05 | No tool | _(none)_ | 1586 | **pass** | One-sentence answer; zero MCP calls |
| T06 | Fail safely | `listTrips` | 2946 | **pass** | Reported trip missing; no invented delete |
| T07 | Surface getTrip error | `getTrip` | 3237 | **pass** | Relayed HTTP 400 invalid-id error clearly |
| T08 | `places.beta.info.warnings` | `listTrips`, `getTripYAML`, `patchTrip` | 7422 | **pass** | Exact patch: `places.beta.info.warnings=["The road may be closed after rain."]` |
| T09 | `update_day` title only | `listTrips`, `getSchema`, `patchTrip` | 11488 | **pass** | Exact patch: `update_day={day:4,title:"Gamma rest (eval)"}` |
| T10 | `listVersions` then `getVersion` | `listVersions`, `getVersion` | 12711 | **pass** | No `restoreVersion`; summarized latest YAML |

**Candidate tally:** 9 pass / 1 partial / 0 fail (critical writes T08–T09 correct).

## Conclusion (candidate-only)

ADK Go + `gpt-4o` + live tripmap MCP is **promising** for this suite: correct tool choice on schema/read/write paths, clean clarify-without-write on T04 (aside from assuming the scratch trip), and no data-integrity misses on the two real workflows.

Still needed for a full proceed/reject vs ChatGPT Agent:

1. Run the same [prompts.md](prompts.md) in ChatGPT Agent (record app version + model).
2. Compare tool sequences and any wrong writes.
3. Optional: 3× repeats on T02/T04/T08 if nondeterminism matters.

## Integration notes

- Instruction brace rewrite `/me/trips/<id>/` (ADK session-state) did not block any case.
- `listTrips` returns ids only — agents may dress them up as titles (T01).
- Extra `listTrips`/`getSchema` calls are common before patches; harmless but noisy for latency/cost.
