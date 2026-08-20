# Runbook: ChatGPT Agent MCP (tripmap)

Primary agent surface is **Streamable HTTP MCP** at `https://tripmap.sheffer.org/mcp`, authenticated with the same **agent Bearer** as `/api/agent/*`. Hellō login is for human viewers only (not agents).

**One secret name everywhere:** `AGENT_BEARER_TOKEN` (raw token, no `Bearer ` prefix). Used by tripmapd, curl/smoke scripts, ChatGPT Agent MCP config, Cursor, and the ADK experiment.

**Verified client: ChatGPT Agent** (desktop app; verified on **26.814.41957**). The product is ChatGPT; the agent runtime is branded as powered by Codex — treat “Codex” in older notes as the same stack unless a separate Codex CLI/IDE install is meant.

Do **not** put the Bearer into a coding-agent workspace MCP config that shares this repo’s secrets casually; keep the token in your password manager / shell env.

## Endpoints

| Surface | Auth |
|---------|------|
| `POST https://tripmap.sheffer.org/mcp` | Bearer (`Authorization: Bearer …`) |
| `GET /openapi.yaml` | Public (schema only) |
| `/api/agent/*` | Same Bearer (scripts / optional clients) |
| Viewer `/me/trips/…` | Viewer session (after Hellō login) |

## ChatGPT Agent setup

Docs: [Model Context Protocol (ChatGPT Learn)](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

Configure a remote MCP server with:

- **URL:** `https://tripmap.sheffer.org/mcp`
- **Auth:** Bearer from env var **`AGENT_BEARER_TOKEN`** (UI stores the *name*; OS env holds the secret)

Example `config.toml` shape (path depends on the ChatGPT / Codex-powered app install):

```toml
[mcp_servers.tripmap]
url = "https://tripmap.sheffer.org/mcp"
bearer_token_env_var = "AGENT_BEARER_TOKEN"
tool_timeout_sec = 120
```

### Windows vs WSL (important)

The **Windows desktop** ChatGPT Agent app does **not** see variables from WSL (including this repo’s `.env`). Set the same name on Windows:

1. Copy the value from WSL `.env` → `AGENT_BEARER_TOKEN`.
2. Windows user environment variable `AGENT_BEARER_TOKEN` = raw token (no `Bearer ` prefix).
3. Fully quit and relaunch ChatGPT so it picks up the env.
4. In the MCP UI / config, the field is only the **name** `AGENT_BEARER_TOKEN`, not the secret itself.

```powershell
[System.Environment]::SetEnvironmentVariable('AGENT_BEARER_TOKEN', '…paste-token…', 'User')
```

If you previously used `tripmap_mcp_bearer_token`, rename the Windows env var and the `bearer_token_env_var` / Cursor header to `AGENT_BEARER_TOKEN`.

For **CLI inside WSL**: `set -a && source .env && set +a` (already defines `AGENT_BEARER_TOKEN`).

### Smoke in ChatGPT Agent

Ask “List my tripmap trips.” — expect a `listTrips` tool call. Server `instructions` steer toward `patchTrip` / `update_day` and `places.<id>.info` (not stuffing enrichment into notes).

## Cursor (optional)

Same URL + Bearer header. Prefer a **separate** MCP profile that does not live in this coding workspace.

```json
{
  "mcpServers": {
    "tripmap": {
      "url": "https://tripmap.sheffer.org/mcp",
      "headers": {
        "Authorization": "Bearer ${env:AGENT_BEARER_TOKEN}"
      }
    }
  }
}
```

## Smoke (HTTP)

```bash
set -a && source .env && set +a
# initialize
curl -fsS -X POST "https://tripmap.sheffer.org/mcp" \
  -H "Authorization: Bearer $AGENT_BEARER_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'
# tools/list (stateless: no session header required)
curl -fsS -X POST "https://tripmap.sheffer.org/mcp" \
  -H "Authorization: Bearer $AGENT_BEARER_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

From ChatGPT Agent after connect: “List trips” → `listTrips`; then a small `patchTrip` with `update_day` on a throwaway field if you want a write smoke.

## Implementation notes

- Module: [`github.com/yaronf/mcpopenapi`](https://github.com/yaronf/mcpopenapi) — OpenAPI → tools; `tools/call` → in-process `ServeHTTP` on the agent mux (no second Bearer hop).
- Missing `Idempotency-Key` is generated server-side for mutating ops.
- Classic ChatGPT Custom GPT Actions / OAuth Mixed is out of scope for this MCP path.
