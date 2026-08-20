# ADK Go + MCP Agent Experiment

## Goal

Determine whether an ADK Go agent using an OpenAI reasoning model can match the behavior of ChatGPT Agent when both use the same existing MCP server.

This experiment should isolate the agent runtime. Do not redesign, extend, or reinterpret the backend or MCP interface.

## Comparison

- **Baseline:** ChatGPT Agent (desktop; verified **26.814.41957**, powered by Codex) + existing MCP server
- **Candidate:** ADK Go + OpenAI reasoning model + the same MCP server
- Run both against the same backend, data, user prompts, and MCP tool definitions.

## MCP contract (do not invent another path)

| Item | Value |
|------|--------|
| URL | `https://tripmap.sheffer.org/mcp` (override with `TRIPMAP_MCP_URL` only for local/dev) |
| Transport | Streamable HTTP |
| Auth | `Authorization: Bearer <token>` — secret env name: `AGENT_BEARER_TOKEN` |
| Tools | OpenAPI → MCP via audience `mcp` (see live `tools/list`) |
| Client setup | [runbook-mcp.md](runbook-mcp.md) |

## Models (pin both sides)

Record exact IDs before scoring:

- **Baseline:** whatever ChatGPT Agent is using for the run (product setting / visible model name). Note app version (e.g. 26.814.41957).
- **Candidate:** `OPENAI_MODEL` (default in this experiment: `o4-mini`). Change only deliberately; a model mismatch is a known confound in the conclusion.

## Data freeze

1. Pick one scratch trip id (or create one) for mutating cases; do not use production itineraries for writes.
2. Before each mutating prompt (or each of 3 repeats): note `listVersions` head, or `restoreVersion` to a known good version id.
3. Read-only prompts may share the live trip; still record trip id and approximate clock time.
4. Run baseline and candidate against the same trip id and the same restore point for paired comparisons.

## Candidate implementation

Code lives in [`experiments/adk-mcp/`](../experiments/adk-mcp/) (separate Go module). The smallest runnable ADK Go agent that:

1. Connects directly to the existing MCP server (Streamable HTTP + Bearer).
2. Discovers and exposes its current tools to one plain LLM agent.
3. Uses an OpenAI reasoning model through ADK's `openaimodel` (Responses API).
4. Provides ADK's existing launcher (console CLI and optional web UI) for interactive testing.
5. Can run a one-shot prompt and write JSONL of events (prompts, model text, tool calls/args/results, errors, elapsed time).

Keep the scaffolding minimal. Do not add graph workflows, multi-agent routing, custom planning, memory, retries, tool wrappers, prompt optimization, or a production UI unless required merely to run the experiment.

## Setup

1. Experiment is isolated under `experiments/adk-mcp/` so the main tripmap module stays untouched.
2. Configuration from environment: `OPENAI_API_KEY`, `OPENAI_MODEL`, `TRIPMAP_MCP_URL`, `AGENT_BEARER_TOKEN`.
3. Reuse the MCP server exactly as deployed today.
4. See that directory’s README for install/run commands.
5. Sample env: `experiments/adk-mcp/.env.example` (names and placeholders only).

## Constraints

- Do not change backend code, business rules, stored data semantics, MCP tool names, descriptions, schemas, responses, error behavior, or authorization behavior.
- Do not add candidate-only tools or direct backend/API access that bypasses MCP.
- Do not modify tool outputs before giving them to the agent, except for transport decoding required by the MCP client.
- If an integration issue appears to require an MCP/backend change, stop and document it instead of changing semantics.
- Keep prompts equivalent across both systems. Any unavoidable system-instruction differences must be documented.
- Prefer the same system instruction text as the live MCP server `instructions` field (copied into the agent README).

## Test cases

Create a small repeatable suite (about 8–12 prompts) covering:

- A simple read/query using one tool.
- A multi-step task requiring two or more tool calls.
- A task requiring the model to select among similar tools.
- A task with missing information where the agent should ask a clarifying question.
- A request that should not invoke a tool.
- An invalid or impossible request that should fail clearly and safely.
- A tool/backend error.
- At least two representative real user workflows that ChatGPT Agent currently handles poorly (or that you care about most).

Run every prompt against both the baseline and candidate using the same initial data. Repeat nondeterministic cases at least three times.

Concrete prompts: [`experiments/adk-mcp/test-cases.md`](../experiments/adk-mcp/test-cases.md).

## Evaluation

For each run, record:

- Task completed correctly: yes/no/partial.
- Tool choice and sequencing.
- Tool-argument correctness.
- Unsupported assumptions or hallucinations.
- Quality of clarification and error recovery.
- Number of model turns and tool calls.
- End-to-end latency and approximate model cost, when available.
- Tool count from `tools/list` (tool-flood context).

Success emphasis: no worse on the two critical real workflows, and no extra destructive/wrong writes — not only “≥ N tasks correct.”

## Success criteria

The candidate is promising if it:

- Completes critical workflows at least as well as ChatGPT Agent, with no material safety or data-integrity regression.
- Uses the correct MCP tools and valid arguments reliably.
- Handles clarification and tool errors comparably to the baseline.
- Requires only the minimal ADK agent configuration described above.
- Does not depend on backend or MCP semantic changes.

## Deliverables

- Minimal runnable ADK Go experiment (`experiments/adk-mcp/`).
- README and safe sample environment file.
- Repeatable test-case file.
- Results table comparing baseline and candidate.
- Brief conclusion: proceed, iterate on the agent layer, or reject—with observed evidence and any integration blockers.

## Implementation rule for Cursor

Before coding, inspect the existing MCP transport and repository conventions. Make only the smallest changes needed to add this isolated experiment. Do not refactor unrelated code. If a required detail is unclear, state the assumption in the README rather than silently changing backend or MCP behavior.
