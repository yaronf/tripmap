# Results — viewer-chat multi-turn e2e

Harness: [`test/viewerchat-mt`](./) · in-process tripmapd (mem store seeded from prod `getVersion`) · Hellō + SSE · model `gpt-5-mini` · [`internal/mteval`](../../internal/mteval)

Run date: 2026-08-21 · logs under `runs/`

| Scenario | Source | Result |
|----------|--------|--------|
| MT01 | suite-mt | **PASS** |
| MT02 | suite-mt | **PASS** |
| MT03 | suite-mt | **PASS** |
| MT04 | viewer fork | **PASS** |
| MT05 | suite-mt | **PASS** |
| MT06 | suite-mt | **PASS** |
| MT07 | suite-mt | **PASS** |
| MT08 | suite-mt | **PASS** |
| MT09 | viewer fork | **FAIL** — `no_tools_until_turn` (turn 9): early `getTripYAML` before approve |
| MT10 | viewer fork | **PASS** |

**Score: 9/10**

MT09 note: model read YAML while summarizing “decided so far” before the approve turn; other checks passed.
