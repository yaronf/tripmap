# Results — Eino Responses multi-turn smoke

**Date:** 2026-08-20  
**Module:** `experiments/eino-responses`  
**Deps:** `eino` v0.9.15 · `agenticopenai` v0.2.2  
**Model:** `gpt-5-mini`  
**Compare:** ADK-Go `openaimodel` fails turn 2 with assistant `input_text` ([#1197](https://github.com/google/adk-go/issues/1197))

## Verdict

**Eino’s Responses support is better than released ADK for multi-turn.** Stock Eino completed turn 2 with no HTTP rewrite.

| Mode | Turn 2 | Strategy observed | Assistant `input_text` |
|------|--------|-------------------|-------------------------|
| `-cache=true` (`EnableAutoCache`) | **pass** | `previous_response_id` = turn1 response id; incremental input | none |
| `-cache=false` (full history) | **pass** | Client replay with `output_text` / string (not ADK bug class) | **0** blocks |

## What this does / does not prove

- **Does:** Eino avoids the ADK turn-2 400; prefers native Responses chaining when auto-cache is on.
- **Does not:** tripmap MCP tool quality, MT01–MT10 context confusion, or production agent UX vs ChatGPT Agent.

## Next (optional)

Port the ADK MCP experiment to Eino (`AgenticModel` + MCP tools) and re-run suite MT **without** `openai_fix.go`.
