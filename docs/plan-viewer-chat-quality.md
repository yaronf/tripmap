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

Use a long ChatGPT itinerary / product thread once to seed **edits to `prompt.txt`** or **agent learnings you approve in chat**—not a parallel `lessons.txt` embed.

Paste this into that ChatGPT thread (adjust the “already known” list if it drifts):

````
You have been helping design and edit a road-trip itinerary app called tripmap (YAML days/places, overnight continuity, in-viewer chat tools).

Distill ONLY durable rules from THIS conversation. Ignore one-off trip facts (specific towns, day numbers, restaurant names) unless they illustrate a reusable rule.

Split output into exactly three sections. Use short imperative bullets (max ~20 words each). Cap each section at 15 bullets. Prefer fewer high-value items over completeness.

### A. Agent learnings (how to operate the app)
Rules about tools, patch shape, stop types, undo/restore, verify-before-claim, continuity procedure.
These are procedural — not food/pace taste.
Format each as:
- LEARNING: <rule>
  WHY: <one line from this thread>

### B. User preferences (taste / constraints)
Standing likes/dislikes that should follow the traveler across trips (diet, budget, driving length, lodging style, etc.).
Format each as:
- PREF: <statement>
  WHY: <one line>

### C. Prompt bootstrap candidates (product/tool facts for developers)
Rules that belong in the static system prompt (tool recipes, never/always for itinerary integrity). Skip anything already covered below.

Already known — DO NOT repeat:
- Prefer replaceDayRoutes for overnight/endpoint or full route replacement; never swap_days for that; don’t use upsert_stop to change overnight ends.
- Route lodging ends use type overnight (never via/depart); mid road towns via; viewer derives Depart.
- For day N+1 after an overnight change, keep mid/end stops; only change the start; don’t shrink a travel day to one stop unless asked.
- Undo = restoreVersion of the first non-latest version; never restore is_latest to “undo”.
- Verify with getTripYAML before claiming success; never invent successful mutations.
- Metric units only; day hero photos via setDayPhoto; don’t paste image URLs in chat.
- Prefs = taste (savePreference after asking); learnings = procedure (saveLearning after asking).

Also:
- No Greymouth/Punakaiki/Franz Josef (or other trip-specific) copy-paste.
- No Markdown tables.
- End with a 5-bullet “highest impact to try first” shortlist mixing A/B/C.
````

**What to do with the answer**

1. **A** → in viewer chat, approve and `saveLearning` (or ask the agent to save after you paste the shortlist).
2. **B** → same with `savePreference`.
3. **C** → hand-edit [`internal/viewerchat/prompt.txt`](../internal/viewerchat/prompt.txt) only if it isn’t already there; then redeploy.


## Golden scenarios (manual)

After deploy, new Persona thread each time; watch CloudWatch `viewerchat tool` lines:

1. Change overnight → expect `replaceDayRoutes`, not `swap_days` / solo `upsert_stop`
2. Undo last change → `restoreVersion` of first non-latest
3. “I always want vegetarian dinners” → explicit prefs offer → `savePreference`
4. Correct a procedure → learning offer → `saveLearning`
5. Thumbs down → next turn mentions saving a learning

## Related

- [plan-viewer-openai-chat.md](plan-viewer-openai-chat.md) — Persona + Responses architecture
