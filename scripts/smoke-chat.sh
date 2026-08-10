#!/usr/bin/env bash
# Local (or remote) smoke for in-viewer chat SSE.
#
# Terminal A — start tripmapd (mem store, no S3):
#   set -a && source .env && set +a
#   export PUBLIC_BASE_URL=http://127.0.0.1:8080
#   export HELLO_CLIENT_ID="${HELLO_CLIENT_ID:-app_local_smoke}"
#   # HELLO_SESSION_SECRET, AGENT_BEARER_TOKEN, OPENAI_API_KEY required in .env
#   export ROUTE_MODE=straight
#   unset ITINERARIES_BUCKET COMMENTS_BUCKET
#   go run ./cmd/tripmapd
#
# Terminal B:
#   set -a && source .env && set +a
#   BASE_URL=http://127.0.0.1:8080 ./scripts/smoke-chat.sh
#
# Optional:
#   MSG='Find a pub for day 1 evening' TRIP_ID=existing-id ./scripts/smoke-chat.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
BASE_URL="${BASE_URL%/}"
TOKEN="${TOKEN:-${AGENT_BEARER_TOKEN:-}}"
EMAIL="${CHAT_EMAIL:-}"
MSG="${MSG:-Say hi in one short sentence. Do not use tools.}"
DAY="${DAY:-1}"

if [[ -z "$TOKEN" ]]; then
  echo "set AGENT_BEARER_TOKEN or TOKEN" >&2
  exit 2
fi
if [[ -z "${HELLO_SESSION_SECRET:-}" ]]; then
  echo "set HELLO_SESSION_SECRET (must match tripmapd)" >&2
  exit 2
fi
if [[ -z "$EMAIL" ]]; then
  EMAIL="$(awk -F, 'NR>1 && $1!=""{print $1; exit}' config/chat-allowlist.csv)"
fi
if [[ -z "$EMAIL" ]]; then
  echo "set CHAT_EMAIL or add an email to config/chat-allowlist.csv" >&2
  exit 2
fi

echo "== health =="
curl -fsS -m 10 "$BASE_URL/health" | jq -e '.status == "ok"' >/dev/null
echo ok

ID="${TRIP_ID:-}"
if [[ -z "$ID" ]]; then
  ID="chat-smoke-$(date +%s)"
  KEY="chat-smoke-$(date +%s)-$$"
  yaml=$(cat <<'YAML'
trip: Chat Smoke
description: local chat SSE smoke
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
  echo "== create $ID =="
  body=$(jq -n --arg id "$ID" --arg yaml "$yaml" '{id: $id, yaml: $yaml}')
  curl -fsS -m 120 -X POST \
    -H "Authorization: Bearer $TOKEN" \
    -H "Idempotency-Key: $KEY" \
    -H "Content-Type: application/json" \
    -d "$body" \
    "$BASE_URL/api/agent/trips" | jq -e --arg id "$ID" '.id == $id' >/dev/null
  echo ok
else
  echo "== using trip $ID =="
fi

echo "== mint session cookie for $EMAIL =="
COOKIE="$(HELLO_SESSION_SECRET="$HELLO_SESSION_SECRET" go run ./scripts/mint-session-cookie -email "$EMAIL")"
echo ok

echo "== POST /me/trips/$ID/api/chat (SSE) =="
echo "msg: $MSG"
echo "----"
# -N: no buffer; show EventStream as it arrives
set +e
HTTP_CODE=$(curl -sS -N -m 120 -o /tmp/tripmap-chat-sse.txt -w '%{http_code}' -X POST \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -H "Cookie: $COOKIE" \
  -d "$(jq -n --arg m "$MSG" --argjson d "$DAY" '{messages:[{role:"user",content:$m}],day:$d}')" \
  "$BASE_URL/me/trips/$ID/api/chat")
CURL_RC=$?
set -e
echo "----"
echo "http=$HTTP_CODE curl_rc=$CURL_RC"
if [[ -s /tmp/tripmap-chat-sse.txt ]]; then
  cat /tmp/tripmap-chat-sse.txt
  echo
else
  echo "(empty body)" >&2
fi
if [[ "$HTTP_CODE" != "200" ]]; then
  exit 1
fi
if ! grep -q 'data: ' /tmp/tripmap-chat-sse.txt; then
  echo "no SSE data: lines seen — stream did not start?" >&2
  exit 1
fi
echo "== smoke-chat ok =="
