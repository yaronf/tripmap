# Plan: viewer chat quality (catch up to ChatGPT)

**Status:** implemented (transcript curation, prefs nudge, agent learnings, trip_fragment + tool hygiene, thumbs feedback).  
**Overview:** Close the quality gap vs ChatGPT by context engineering and chat-owned memory—not a second static lessons file.

## Taxonomy (do not conflate)

| Layer | What it is | Owner | Example |
|-------|------------|-------|---------|
| **`prompt.txt`** | Static bootstrap: identity, tools, tone, core edit recipes | Repo | Use `replaceDayRoutes` for overnight changes |
| **User preferences** | Standing taste / constraints | User (Hellō `sub`) | Prefer vegetarian dinners |
| **Agent learnings** | How to operate this app after corrections | Agent, with user agreement | Don’t set lodging start as `via` |
| **Tools** | Read/mutate surface | Repo (OpenAPI + handlers) | Curated ops, useful returns |
| **Thread memory** | This Persona session | Ephemeral (+ summary) | User asked for Punakaiki |

ChatGPT “learns” ≈ agent learnings + thread. Prefs are taste. Tools are how the agent acts.

## Context stack (every turn)

1. `prompt.txt`
2. User preferences (injected; offer `savePreference` when taste is stated)
3. Agent learnings (injected; offer `saveLearning` on procedural corrections)
4. Tools + light server orientation (`trip_fragment` for day N±1)
5. Thread: last 8 verbatim + heuristic summary of older turns

## Phases

0. Taxonomy / docs (this file)
1. Transcript curation
2. Prefs offer nudge
3. Agent learnings store + tools
4. Tool improvements (fragment, OpenAPI hygiene, soft warnings)
5. Eval + thumbs (down → learning offer on next turn)

## Optional ChatGPT distill

Use a long ChatGPT thread once to seed **edits to `prompt.txt`** or **agent learnings you approve in chat**—not a parallel `lessons.txt` embed or PR-gated lessons pipeline.

## Golden scenarios (manual)

After deploy, new Persona thread each time; watch CloudWatch `viewerchat tool` lines:

1. Change overnight → expect `replaceDayRoutes`, not `swap_days` / solo `upsert_stop`
2. Undo last change → `restoreVersion` of first non-latest
3. “I always want vegetarian dinners” → explicit prefs offer → `savePreference`
4. Correct a procedure → learning offer → `saveLearning`
5. Thumbs down → next turn mentions saving a learning

## Related

- [plan-viewer-openai-chat.md](plan-viewer-openai-chat.md) — Persona + Responses architecture
