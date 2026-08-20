# Runbook: in-viewer chat (Persona + Eino)

Hellō-signed-in viewers with `chat=yes` in `config/users.csv` get an **Ask** pane (Persona). The backend agent is Eino OpenAI **Responses** (`gpt-5-mini` by default) with hosted **`web_search`** plus in-process itinerary tools (same ops as MCP).

## Enable

1. **ACL:** `config/users.csv` column `chat` = `yes` / `true` / `1` (see `users.example.csv`). Rows without `chat` can still sign in.
2. **OpenAI:** set `OPENAI_API_KEY` or `OPENAI_SECRET_JSON` (`{"api_key":"sk-..."}`). Optional `OPENAI_MODEL` (default `gpt-5-mini`). Hosted `web_search` uses the same key (OpenAI bills search separately).
3. Without a key, chat dark-ships: `/auth/me` → `chat_enabled: false`, `POST …/api/chat` → 503.

Deploy: compute injects `OPENAI_SECRET_JSON` from Secrets Manager `tripmap/openai` and sets `OPENAI_MODEL=gpt-5-mini`. Bake `users.csv` with chat ACL into the image (same as Hellō allowlist).

## Local smoke

```bash
# Terminal A
set -a && source .env && set +a
export PUBLIC_BASE_URL=http://127.0.0.1:8080
export ROUTE_MODE=straight
unset ITINERARIES_BUCKET COMMENTS_BUCKET
go run ./cmd/tripmapd

# Terminal B
set -a && source .env && set +a
BASE_URL=http://127.0.0.1:8080 ./scripts/smoke-chat.sh
```

Requires `AGENT_BEARER_TOKEN`, `HELLO_SESSION_SECRET`, `OPENAI_API_KEY`, and a `chat=yes` email in `users.csv` (or `CHAT_EMAIL=…`).

## Logs (CloudWatch / stdout)

Structured `log/slog` lines with `component=viewerchat` and `request_id`:

| `msg` | Meaning |
|-------|---------|
| `turn_start` | trip, sub, day, msg count, truncated user text |
| `model_call` | latency, model, response_id, function `tool_calls`, `web_search` count (or error) |
| `tool_call` | tool name, truncated args (~500 runes), latency, result bytes, mutate flag |
| `turn_end` | total ms, trip_updated, outcome `done` / `error` |
| `viewerchat feedback` | thumbs up/down (stdlib `log`, truncated texts) |

Never log API keys or full YAML. Filter Insights by `trip_id` or `tool`.

## SSE contract

`POST /me/trips/{id}/api/chat` → `text/event-stream`: keepalive comments (`: ping`), then `text` / `trip_updated` / `error` / `done`. Heartbeats every ~10s for the whole tool loop.
