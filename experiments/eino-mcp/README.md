# Eino + tripmap MCP experiment

Candidate agent using CloudWeGo **Eino** `AgenticModel` (OpenAI Responses + `EnableAutoCache`) and live tripmap MCP tools via `eino-ext` MCP client.

Compare with [`../adk-mcp/`](../adk-mcp/). No `input_text` rewrite — stock Responses multi-turn.

## Run

```bash
cd experiments/eino-mcp
set -a && source ../../.env && set +a
export OPENAI_MODEL=gpt-5-mini

go run . --prompt 'List my tripmap trips.' --log runs/list.jsonl

# Reuses ADK suite-mt scenario JSON:
go run . --scenario ../adk-mcp/suite-mt/scenarios/MT01_rejected_italy.json --log runs/mt/MT01.jsonl
```

`--no-cache` disables `EnableAutoCache` (forces full client history replay).

## Notes

- Instruction text matches the ADK experiment / MCP server instructions.
- Heuristic checks are the same kinds as ADK suite MT.
