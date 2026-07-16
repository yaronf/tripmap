#!/usr/bin/env bash
# Post-deploy smoke for tripmapd agent API + capability viewer.
# Usage:
#   BASE_URL=https://….ecs.eu-central-1.on.aws TOKEN=… ./scripts/smoke-agent.sh
#
# Fetch TOKEN without pasting it into chat:
#   TOKEN=$(aws secretsmanager get-secret-value --secret-id tripmap/agent-bearer \
#     --region eu-central-1 --query SecretString --output text | jq -r .token)
set -euo pipefail

BASE_URL="${BASE_URL:?set BASE_URL (https://… no trailing slash)}"
TOKEN="${TOKEN:?set TOKEN (agent Bearer)}"
BASE_URL="${BASE_URL%/}"
ID="smoke-$(date +%s)"
KEY="smoke-$(date +%s)-$$"

yaml=$(cat <<'YAML'
trip: Smoke Trip
description: agent API smoke
days:
  - day: 1
    title: Start
    stops:
      - { name: Alpha, type: overnight, lat: 52.37, lon: 4.90 }
YAML
)

echo "== health =="
curl -fsS -m 20 "$BASE_URL/health" | jq -e '.status == "ok"' >/dev/null
echo ok

echo "== schema =="
curl -fsS -m 20 -H "Authorization: Bearer $TOKEN" "$BASE_URL/api/agent/schema" \
  | jq -e '.schema_version == 1' >/dev/null
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
echo "$create_json" | jq -e --arg id "$ID" '.id == $id' >/dev/null
CAP_TOKEN=$(echo "$create_json" | jq -r .token)
VIEWER_URL=$(echo "$create_json" | jq -r .viewer_url)
if [[ -z "$CAP_TOKEN" || "$CAP_TOKEN" == null ]]; then
  echo "create response missing token" >&2
  exit 1
fi
# Normalize relative viewer_url
if [[ "$VIEWER_URL" == /* ]]; then
  VIEWER_URL="${BASE_URL}${VIEWER_URL}"
fi
VIEWER_URL="${VIEWER_URL%/}/"
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

echo "== capability index =="
curl -fsS -m 30 "${VIEWER_URL}index.html" | grep -q "<title>"
echo ok

echo "== capability notes =="
curl -fsS -m 20 "${VIEWER_URL}api/notes" | jq -e 'has("days")' >/dev/null
curl -fsS -m 20 -X PUT \
  -H "Content-Type: application/json" \
  -d '{"days":{"1":"smoke note"}}' \
  "${VIEWER_URL}api/notes" | jq -e '.days["1"] == "smoke note"' >/dev/null
echo ok

echo "== capability bad token =="
code=$(curl -sS -o /dev/null -w "%{http_code}" -m 20 "${BASE_URL}/t/${ID}/not-a-real-token/index.html" || true)
[[ "$code" == "404" ]] || { echo "want 404 got $code" >&2; exit 1; }
echo ok

echo "SMOKE PASS id=$ID"
echo "Open viewer (keep private): ${VIEWER_URL}"
