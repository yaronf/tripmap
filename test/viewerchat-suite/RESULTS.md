# Results — viewer-chat e2e (MT + S)

Harness: [`test/viewerchat-suite`](./) · in-process tripmapd · Hellō + SSE · [`internal/mteval`](../../internal/mteval)

## MT (multi-turn) — last run 2026-08-21 · `gpt-5-mini`

| Scenario | Result |
|----------|--------|
| MT01 | **PASS** |
| MT02 | **PASS** |
| MT03 | **PASS** |
| MT04 | **PASS** |
| MT05 | **PASS** |
| MT06 | **PASS** |
| MT07 | **PASS** |
| MT08 | **PASS** |
| MT09 | **FAIL** — `no_tools_until_turn` (turn 9): early `getTripYAML` before approve |
| MT10 | **PASS** |

**MT score: 9/10**

## S (one-shot) — not yet scored on this harness

Scenarios `S01`–`S10` added from ADK prompts-s2 (viewer-adapted). Run:

```bash
go run ./test/viewerchat-suite --dir test/viewerchat-suite/scenarios
# or: --scenario test/viewerchat-suite/scenarios/S06_overnight_delta.json
```

Prior ADK MCP one-shot results (reference only): [`experiments/adk-mcp/RESULTS-s2-gpt-5-mini.md`](../../experiments/adk-mcp/RESULTS-s2-gpt-5-mini.md).
