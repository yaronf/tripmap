#!/usr/bin/env bash
# Fast app-image deploy: ECR push + UpdateExpressGatewayService (no CloudFormation).
# Prints wall-clock seconds per step for experiment / comparison with CFN deploys.
#
# Usage (from repo root, as tripmap-deploy):
#   ./scripts/deploy-app-image.sh
#   TAG=my-tag ./scripts/deploy-app-image.sh
#   SKIP_SMOKE=1 ./scripts/deploy-app-image.sh
#   REGEN_TRIPS="holland nz-4weeks" ./scripts/deploy-app-image.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export PATH="${HOME}/.local/bin:${PATH}"
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy

ACCOUNT="${ACCOUNT:-077804408159}"
REGION="${REGION:-eu-central-1}"
REPO="${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com/tripmapd"
TAG="${TAG:-sw-geo-$(date +%Y%m%d%H%M%S)}"
STACK="${STACK:-tripmap-compute}"
BASE_URL="${BASE_URL:-https://tripmap.sheffer.org}"
SKIP_SMOKE="${SKIP_SMOKE:-0}"
REGEN_TRIPS="${REGEN_TRIPS:-holland nz-4weeks}"

TOTAL_START=$(date +%s)
step() {
  STEP_NAME="$1"
  STEP_START=$(date +%s)
  echo ""
  echo "==> [${STEP_NAME}] start $(date -Iseconds)"
}
end_step() {
  local end elapsed
  end=$(date +%s)
  elapsed=$((end - STEP_START))
  printf "==> [%s] done in %ds\n" "$STEP_NAME" "$elapsed"
  TIMINGS+=("${STEP_NAME}:${elapsed}")
}
TIMINGS=()

echo "deploy-app-image: TAG=${TAG}"
echo "repo=${REPO}"
echo "base=${BASE_URL}"

step "aws-identity"
aws sts get-caller-identity --region "$REGION"
end_step

step "ecr-login"
aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com"
end_step

step "docker-build"
docker build --platform linux/amd64 -t "${REPO}:${TAG}" -t "${REPO}:latest" .
end_step

step "docker-push-tag"
docker push "${REPO}:${TAG}"
end_step

step "docker-push-latest"
docker push "${REPO}:latest"
end_step

step "resolve-service-arn"
SERVICE_ARN=$(aws cloudformation describe-stacks \
  --stack-name "$STACK" --region "$REGION" \
  --query "Stacks[0].Outputs[?OutputKey=='ServiceArn'].OutputValue" \
  --output text)
if [[ -z "$SERVICE_ARN" || "$SERVICE_ARN" == "None" ]]; then
  echo "error: could not resolve ServiceArn from stack ${STACK}" >&2
  exit 1
fi
echo "ServiceArn=${SERVICE_ARN}"
end_step

step "ecs-update-express-image"
# Image-only roll; Express keeps env/secrets/scaling from the current revision.
aws ecs update-express-gateway-service \
  --region "$REGION" \
  --service-arn "$SERVICE_ARN" \
  --primary-container "image=${REPO}:${TAG}"
end_step

step "wait-health"
# Poll public health until the new task is serving (or timeout).
OK=0
for i in $(seq 1 60); do
  if curl -fsS -m 10 "${BASE_URL}/health" | jq -e '.status == "ok"' >/dev/null 2>&1; then
    # Prefer seeing the new OpenAPI once the roll has progressed; health alone
    # may still hit an old task briefly during canary.
    echo "health ok (attempt ${i})"
    OK=1
    break
  fi
  sleep 5
done
if [[ "$OK" -ne 1 ]]; then
  echo "error: health did not become ok within timeout" >&2
  exit 1
fi
end_step

if [[ "$SKIP_SMOKE" != "1" ]]; then
  step "smoke-agent"
  if [[ -f .env ]]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
  fi
  if [[ -z "${AGENT_BEARER_TOKEN:-}" ]]; then
    echo "warn: AGENT_BEARER_TOKEN unset; skipping smoke" >&2
  else
    BASE_URL="$BASE_URL" TOKEN="$AGENT_BEARER_TOKEN" ./scripts/smoke-agent.sh
  fi
  end_step

  if [[ -n "${REGEN_TRIPS// /}" && -n "${AGENT_BEARER_TOKEN:-}" ]]; then
    step "regen-bundles"
    for ID in $REGEN_TRIPS; do
      KEY="regen-${TAG}-${ID}"
      YAML=$(curl -fsS -m 30 -H "Authorization: Bearer ${AGENT_BEARER_TOKEN}" \
        "${BASE_URL}/api/agent/trips/${ID}/yaml")
      echo "$YAML" | curl -fsS -m 180 -X PUT \
        -H "Authorization: Bearer ${AGENT_BEARER_TOKEN}" \
        -H "Idempotency-Key: ${KEY}" \
        -H "Content-Type: application/yaml" \
        --data-binary @- \
        "${BASE_URL}/api/agent/trips/${ID}/yaml" \
        | jq -c '{id, bundle_ok, bundle_error}'
    done
    end_step
  fi
fi

TOTAL_END=$(date +%s)
TOTAL=$((TOTAL_END - TOTAL_START))
echo ""
echo "======== TIMINGS (seconds) ========"
for t in "${TIMINGS[@]}"; do
  printf "  %-28s %s\n" "${t%%:*}" "${t##*:}"
done
printf "  %-28s %s\n" "TOTAL" "$TOTAL"
echo "TAG=${TAG}"
echo "NOTE: CloudFormation ImageTag may now be stale; running image = Express revision."
echo "==================================="
