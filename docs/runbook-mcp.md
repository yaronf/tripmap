# Runbook: Codex MCP (tripmap)

Primary agent surface is **Streamable HTTP MCP** at `https://tripmap.sheffer.org/mcp`, authenticated with the same **agent Bearer** as `/api/agent/*` (`AGENT_BEARER_TOKEN`). Hellō remains human-viewer only.

**Verified client: Codex** (desktop / CLI / IDE). Ordinary ChatGPT chat may not list or use local MCP servers at all — treat Codex as the working path.

Do **not** put the Bearer into a coding-agent workspace MCP config that shares this repo’s secrets casually; keep the token in your password manager / shell env.

## Endpoints

| Surface | Auth |
|---------|------|
| `POST https://tripmap.sheffer.org/mcp` | Bearer (`Authorization: Bearer …`) |
| `GET /openapi.yaml` | Public (schema only) |
| `/api/agent/*` | Same Bearer (scripts / optional clients) |
| Viewer `/me/trips/…` | Hellō session |

## Codex setup

Docs: [Model Context Protocol (ChatGPT Learn)](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

### `config.toml`

```toml
[mcp_servers.tripmap]
url = "https://tripmap.sheffer.org/mcp"
bearer_token_env_var = "tripmap_mcp_bearer_token"
tool_timeout_sec = 120
```

On Windows, Codex usually reads config under its app data dir (not your WSL `~`). In Codex, `/mcp` lists connected servers (that slash command is Codex-only — not ChatGPT).

### Windows vs WSL (important)

Codex **desktop on Windows** does **not** see variables from WSL (including this repo’s `.env`). Set the token in **Windows**:

1. Copy the agent Bearer (same value as `AGENT_BEARER_TOKEN` in WSL `.env`).
2. Windows user environment variable:
   - Name: `tripmap_mcp_bearer_token`
   - Value: the raw token (no `Bearer ` prefix)
3. Fully quit and relaunch Codex so it picks up the env.
4. In the MCP UI / config, the field is only the **name** `tripmap_mcp_bearer_token`, not the secret itself.

```powershell
[System.Environment]::SetEnvironmentVariable('tripmap_mcp_bearer_token', '…paste-token…', 'User')
```

For **Codex CLI inside WSL**: `export tripmap_mcp_bearer_token="$AGENT_BEARER_TOKEN"` after `source .env`.

WSL `.env` remains correct for `curl` / smoke scripts from Linux.

### Smoke in Codex

Ask “List my tripmap trips.” — expect a `listTrips` tool call. Server `instructions` steer toward `patchTrip` / `update_day` and `places.<id>.info` (not stuffing enrichment into notes).

### ChatGPT chat (not working for this)

Confirmed in practice: ChatGPT does not show local MCP servers and has no `/mcp` slash command. Codex does both. Prefer Codex; revisit ChatGPT only if OpenAI wires local Streamable HTTP MCP into that client.

## Cursor (optional)

Same URL + Bearer header. Prefer a **separate** MCP profile that does not live in this coding workspace.

Example shape (client-specific):

```json
{
  "mcpServers": {
    "tripmap": {
      "url": "https://tripmap.sheffer.org/mcp",
      "headers": {
        "Authorization": "Bearer ${env:tripmap_mcp_bearer_token}"
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

From Codex after connect: “List trips” → `listTrips`; then a small `patchTrip` with `update_day` on a throwaway field if you want a write smoke.

## Implementation notes

- Module: [`mcpopenapi/`](../mcpopenapi/) — OpenAPI → tools; `tools/call` → in-process `ServeHTTP` on the agent mux (no second Bearer hop).
- Missing `Idempotency-Key` is generated server-side for mutating ops.
- Classic ChatGPT OAuth/Mixed is out of scope for v1.
