# Viewer-chat e2e suite

Drives conversation scripts through **in-viewer chat** (Hellō cookie + SSE) against an
**in-process** `tripmapd` on localhost. Scores with shared [`internal/mteval`](../../internal/mteval).

Two families share the same harness:

| Family | Prefix | What it stresses |
|--------|--------|------------------|
| **MT** | `MT01`–`MT10` | Multi-turn context confusion (stale options, premature writes, pronouns) |
| **S** | `S01`–`S10` | One-shot sophistication (place info vs notes, overnight via `replaceDayRoutes`, version forensics, …) |

S prompts are adapted from [`experiments/adk-mcp/prompts-s2.md`](../../experiments/adk-mcp/prompts-s2.md) for a fixed open trip (no `listTrips`).

## Prerequisites

From the repo root, with a normal tripmap `.env`:

- `OPENAI_API_KEY` or `OPENAI_SECRET_JSON`
- `OPENAI_MODEL` (optional; default `gpt-5-mini`)
- `HELLO_SESSION_SECRET`
- `AGENT_BEARER_TOKEN`
- `ITINERARIES_BUCKET` (optional): if set, restores seed in S3; if unset, uses **mem store** and seeds YAML from `TRIPMAP_SEED_URL` (default `https://tripmap.sheffer.org`) via agent `getVersion`
- `config/users.csv` with a `chat=yes` email (or set `CHAT_EMAIL`)

`ROUTE_MODE` defaults to `straight` if unset.

## Run one scenario

```bash
set -a && source .env && set +a
go run ./test/viewerchat-suite \
  --scenario test/viewerchat-suite/scenarios/S06_overnight_delta.json \
  --log test/viewerchat-suite/runs/S06.jsonl
```

## Run the full suite

```bash
go run ./test/viewerchat-suite --dir test/viewerchat-suite/scenarios
# logs → test/viewerchat-suite/runs/<id>.jsonl
```

## Flags

| Flag | Meaning |
|------|---------|
| `--scenario` | Path to one scenario JSON |
| `--dir` | Run every `*.json` in a directory (sorted) |
| `--log` | Single-scenario JSONL path; with `--dir`, treat as a log **directory** |
| `--day` | Viewer day context (default 1) |

## Checks (`internal/mteval`)

| Kind | Meaning |
|------|---------|
| `never_tools` | Named tools must not appear |
| `no_tools_until_turn` | No tools before turn index (0-based) |
| `tools_must_include` | Named tools must appear somewhere |
| `tools_after_turn_must_include` | Named tools from turn T onward |
| `final_text_regex` / `final_text_not_regex` | Last assistant text |
| `any_text_regex` | Any assistant text |
| `patch_args_not_regex` | No mutate args (`patchTrip` / `replaceDayRoutes` / `restoreVersion`) match |
| `patch_args_regex` | At least one mutate arg matches |

## Scenarios

All JSON lives under [`scenarios/`](./scenarios/). MT04/MT09/MT10 are viewer forks (no `listTrips` / holland). Scratch trip: **`adk-eval`** seed `iKfKl9ssQruzt3XvRsOLQ8e7ADhxewNk`.
