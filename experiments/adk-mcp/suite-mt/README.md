# Multi-turn context-confusion suite (MT)

Scripted **conversations**, not isolated hard prompts. Each scenario starts from a clean scratch trip and builds context over many turns so the **final request depends on information introduced gradually**.

Failure theme: **context confusion** (stale options, stale YAML, wrong trip pronoun, premature writes, preference overrides).

## How to run

```bash
cd experiments/adk-mcp
set -a && source ../../.env && set +a
export OPENAI_MODEL=gpt-5-mini   # or gpt-4o
go run . --scenario suite-mt/scenarios/MT01_rejected_italy.json --log runs/mt/MT01.jsonl
```

Restore is performed automatically when the scenario sets `setup.restore_version`.

## Scoring

Each scenario JSON ends with `checks` (heuristic). The runner prints pass/fail per check after the conversation. Human review still needed for soft failures (subtle wrong destination in prose).

| Check kind | Meaning |
|------------|---------|
| `never_tools` | These tools must not appear in any turn |
| `no_tools_until_turn` | No tools before turn index (0-based user turns) |
| `tools_after_turn_must_include` | From turn T onward, named tools must appear |
| `final_text_regex` | Last assistant text must match |
| `final_text_not_regex` | Last assistant text must not match |
| `any_text_regex` | Some assistant text in the convo matches |
| `patch_args_not_regex` | No `patchTrip`/`replaceDayRoutes` args match (stale id) |

## Context hazards baked into scripts

- Similar place names (`alpha` / Alpha Base vs hypothetical Alpha Coast)
- Reused day numbers across trips (`day 5` on adk-eval vs holland)
- Hypothetical vs committed plans (“we might…”, “don’t change yet”)
- Tool results that become stale after a later write
- Corrections (“No, I meant day 5”)
- Undo only the latest change
- Long YAML that conflicts with earlier speculation

## Scenarios

| ID | File | Core trap |
|----|------|-----------|
| MT01 | `MT01_rejected_italy.json` | Rejected option set; “second option” refers to the **new** list |
| MT02 | `MT02_dates_twice.json` | Dates changed twice; later edit must use **final** dates |
| MT03 | `MT03_stale_yaml.json` | Read YAML → write → discuss same day; must not use pre-write facts |
| MT04 | `MT04_two_trips.json` | Two trips; “that trip” must resolve or clarify |
| MT05 | `MT05_explore_then_approve.json` | Several edits explored; only one approved |
| MT06 | `MT06_later_wins.json` | Early preference vs later explicit instruction |
| MT07 | `MT07_failed_id_then_fix.json` | Failed tool id → user corrects → no retained bad id |
| MT08 | `MT08_early_constraint.json` | Long digression; recall exact early constraint |
| MT09 | `MT09_decided_so_far.json` | Separate confirmed decisions vs suggestions |
| MT10 | `MT10_ambiguous_final.json` | Final change ambiguous across two days/places → clarify |

Scratch trip defaults to **`adk-eval`** with seed version `iKfKl9ssQruzt3XvRsOLQ8e7ADhxewNk`.
