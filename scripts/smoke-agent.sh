#!/usr/bin/env bash
# Post-deploy smoke for tripmapd agent API.
# Usage:
#   BASE_URL=https://tripmap.sheffer.org TOKEN=… ./scripts/smoke-agent.sh
#
# Put the agent Bearer in a gitignored .env:
#   AGENT_BEARER_TOKEN=…
#   set -a && source .env && set +a
#   BASE_URL=https://tripmap.sheffer.org TOKEN="$AGENT_BEARER_TOKEN" ./scripts/smoke-agent.sh
set -euo pipefail

BASE_URL="${BASE_URL:?set BASE_URL (https://… no trailing slash)}"
TOKEN="${TOKEN:?set TOKEN (agent Bearer)}"
BASE_URL="${BASE_URL%/}"
ID="smoke-$(date +%s)"
KEY="smoke-$(date +%s)-$$"

yaml=$(cat <<'YAML'
trip: Smoke Trip
description: agent API smoke
places:
  alpha:
    title: Alpha
    lat: 52.37
    lon: 4.90
    type: overnight
days:
  - day: 1
    title: Start
    stops:
      - { place: alpha }
YAML
)

echo "== health =="
curl -fsS -m 20 "$BASE_URL/health" | jq -e '.status == "ok"' >/dev/null
echo ok

echo "== schema =="
curl -fsS -m 20 -H "Authorization: Bearer $TOKEN" "$BASE_URL/api/agent/schema" \
  | jq -e '.schema_version == 2' >/dev/null
echo ok

echo "== list trips =="
curl -fsS -m 20 -H "Authorization: Bearer $TOKEN" "$BASE_URL/api/agent/trips" \
  | jq -e 'has("trips")' >/dev/null
echo ok

echo "== create $ID =="
body=$(jq -n --arg id "$ID" --arg yaml "$yaml" '{id: $id, yaml: $yaml}')
create_json=$(curl -fsS -m 120 -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d "$body" \
  "$BASE_URL/api/agent/trips")
echo "$create_json" | jq -e --arg id "$ID" '.id == $id and .bundle_ok == true' >/dev/null
VIEWER_URL=$(echo "$create_json" | jq -r .viewer_url)
echo "$VIEWER_URL" | grep -q "/me/trips/${ID}/"
echo ok

echo "== get yaml =="
curl -fsS -m 20 -H "Authorization: Bearer $TOKEN" "$BASE_URL/api/agent/trips/$ID/yaml" \
  | grep -q "Smoke Trip"
echo ok

echo "== put yaml =="
curl -fsS -m 120 -X PUT \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: ${KEY}-put" \
  -H "Content-Type: application/yaml" \
  --data-binary "$yaml" \
  "$BASE_URL/api/agent/trips/$ID/yaml" | jq -e 'has("bundle_ok")' >/dev/null
echo ok

echo "== mcp initialize =="
curl -fsS -m 30 -X POST "$BASE_URL/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  | grep -q '"name":"tripmap"'
echo ok

echo "SMOKE PASS id=$ID"
echo "Viewer (Hellō sign-in): ${VIEWER_URL}"
