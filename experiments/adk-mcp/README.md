# ADK Go + tripmap MCP experiment

Isolated candidate agent for [docs/adk-go-mcp-experiment.md](../../docs/adk-go-mcp-experiment.md).

Connects to the **live** tripmap Streamable HTTP MCP server, discovers tools, and runs a single ADK `llmagent` on an OpenAI Responses model. No candidate-only tools and no backend changes.

## Prerequisites

- Go 1.25+ (module may request a newer toolchain)
- `OPENAI_API_KEY`
- Agent Bearer: `AGENT_BEARER_TOKEN` (same secret as production MCP / ChatGPT Agent)

## Configure

```bash
cd experiments/adk-mcp
cp env.example .env   # fill secrets locally; never commit .env
set -a && source .env && set +a
```

| Variable | Required | Default |
|----------|----------|---------|
| `OPENAI_API_KEY` | yes* | — |
| `OPENAI_MODEL` | no | `gpt-4o` |
| `OPENAI_BASE_URL` | no | OpenAI default (*or* set this instead of API key for a compatible endpoint) |
| `TRIPMAP_MCP_URL` | no | `https://tripmap.sheffer.org/mcp` |
| `AGENT_BEARER_TOKEN` | yes | — |

## Run

Interactive ADK console (default):

```bash
go run .
# same as: go run . console
```

ADK web UI:

```bash
go run . web
```

One-shot prompt with JSONL event log (for the evaluation table):

```bash
go run . --prompt 'List my tripmap trips.' --log /tmp/adk-listTrips.jsonl
```

Multi-turn context-confusion scenario (see [suite-mt/](suite-mt/)):

```bash
mkdir -p runs/mt
go run . --scenario suite-mt/scenarios/MT01_rejected_italy.json --log runs/mt/MT01.jsonl
```

Each JSONL line is one record: `run_start` / `scenario_start`, `event` (model text / function calls / function responses / usage), `turn_*`, `check`, `run_end` / `scenario_end`.

## System instruction

The agent instruction is copied from the live MCP server `instructions` string in `tripmapd` (prefer `patchTrip` / `places.*.info`, etc.). One deliberate delta for ADK: `/me/trips/{id}/` is written as `/me/trips/<id>/` because ADK injects `{name}` as session state and would fail on missing `id`. Record that in the results table if scoring prompt parity.

## Assumptions

- MCP auth is raw Bearer via `AGENT_BEARER_TOKEN` (`oauth2.StaticTokenSource`).
- `DisableStandaloneSSE: true` — tripmap MCP is used as request/response Streamable HTTP (matches curl smoke in the runbook). If a future server requires the GET SSE stream, flip this and retest.
- Baseline ChatGPT Agent app version and model ID are recorded by the human operator; this binary only knows `OPENAI_MODEL`.
- **Multi-turn / ADK OpenAI connector:** Released ADK-Go `openaimodel` (through at least v2.2.0) serializes replayed assistant text as typed `input_text`, which the Responses API rejects with HTTP 400 from the **second user turn** onward ([adk-go#1197](https://github.com/google/adk-go/issues/1197)). Fixes are open but unmerged as of 2026-08-20 ([PR #1205](https://github.com/google/adk-go/pull/1205), [PR #1291](https://github.com/google/adk-go/pull/1291)). This experiment applies a **local HTTP rewrite** (`openai_fix.go`: assistant `input_text` → `output_text`) so suite MT can run; that is not an official ADK fix and does **not** preserve Responses reasoning items (`encrypted_content` / `phase`) for native chaining. One-shot suites T/S never hit this path. Do not treat patched replay as equivalent to ChatGPT Agent or `previous_response_id` chaining until ADK preserves full output items.
## Test cases

- One-shot suites: [test-cases.md](test-cases.md), [prompts.md](prompts.md), [prompts-s2.md](prompts-s2.md)
- Multi-turn context suite: [suite-mt/README.md](suite-mt/README.md)
- Results template: [RESULTS-mt.md](RESULTS-mt.md)
