# Viewer-chat multi-turn e2e

Drives suite-mt conversation scripts through **in-viewer chat** (Hellō cookie + SSE) against an **in-process** `tripmapd` on localhost. Scores with shared [`internal/mteval`](../../internal/mteval) checks. Tool calls are captured from `slog` (`tool_call`).

## Prerequisites

From the repo root, with a normal tripmap `.env`:

- `OPENAI_API_KEY` or `OPENAI_SECRET_JSON`
- `OPENAI_MODEL` (optional; default `gpt-5-mini`)
- `HELLO_SESSION_SECRET`
- `AGENT_BEARER_TOKEN`
- `ITINERARIES_BUCKET` (optional): if set, restores seed in S3; if unset, uses **mem store** and seeds YAML from `TRIPMAP_SEED_URL` (default `https://tripmap.sheffer.org`) via agent `getVersion`
- `config/users.csv` with a `chat=yes` email (or set `CHAT_EMAIL`)

`ROUTE_MODE` defaults to `straight` if unset. `HELLO_CLIENT_ID` defaults to a local smoke id if unset (users file still required).

## Run one scenario

```bash
set -a && source .env && set +a
go run ./test/viewerchat-mt \
  --scenario experiments/adk-mcp/suite-mt/scenarios/MT01_rejected_italy.json \
  --log test/viewerchat-mt/runs/MT01.jsonl
```

Viewer-specific forks (no `listTrips` / holland):

```bash
go run ./test/viewerchat-mt --scenario test/viewerchat-mt/scenarios/MT04_that_day.json
go run ./test/viewerchat-mt --scenario test/viewerchat-mt/scenarios/MT09_decided_so_far.json
go run ./test/viewerchat-mt --scenario test/viewerchat-mt/scenarios/MT10_ambiguous_final.json
```

Reuse as-is from suite-mt: MT01–MT03, MT05–MT08.

## Flags

| Flag | Meaning |
|------|---------|
| `--scenario` | Path to scenario JSON (required) |
| `--log` | JSONL output path (default: stdout) |
| `--day` | Viewer day context (default 1) |
