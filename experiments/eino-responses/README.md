# Eino Responses API smoke (vs ADK #1197)

Isolated check: does **stock** Eino multi-turn work on OpenAI Responses without an `input_text` rewrite?

This is **not** a tripmap MCP agent yet — only the connector/chaining question.

## Run

```bash
cd experiments/eino-responses
set -a && source ../../.env && set +a
export OPENAI_MODEL=gpt-5-mini

# Preferred path: server-side previous_response_id
go run . -cache=true -log runs/eino-cache.jsonl

# Force full client-side history replay (no auto cache)
go run . -cache=false -log runs/eino-nocache.jsonl
```

## Pass criteria

| Check | Pass |
|-------|------|
| Turn 2 completes (no HTTP 400) | required |
| No assistant `type: input_text` in request bodies | required for nocache path |
| Turn 2 with `-cache=true` sends `previous_response_id` | preferred (stronger than ADK) |

ADK-Go v2.2.0 fails turn 2 without a local rewrite ([adk-go#1197](https://github.com/google/adk-go/issues/1197)).
