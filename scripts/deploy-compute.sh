#!/usr/bin/env bash
# Build/push tripmapd to ECR and update tripmap-compute (ECS Express).
#
# Run this in your own shell (tripmap-deploy identity) — no agent approval loop:
#   ./scripts/deploy-compute.sh
#   ./scripts/deploy-compute.sh --prefix chat-remove
#   ./scripts/deploy-compute.sh --skip-build --tag chat-ux-20260810001535
#   ./scripts/deploy-compute.sh --patch-viewer          # also sync app.js/chat.js/style.css
#   ./scripts/deploy-compute.sh --patch-viewer holland nz-4weeks
#
# Optional env:
#   TAG_PREFIX=manual  ACCOUNT=077804408159  REGION=eu-central-1
#   HELLO_CLIENT_ID=app_slcuDgxAEmgXkpHPePh9Acgp_hi6
#   ITINERARIES_BUCKET=tripmap-itineraries-077804408159-eu-central-1
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export PATH="${HOME}/.local/bin:${PATH}"
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy

ACCOUNT="${ACCOUNT:-077804408159}"
REGION="${REGION:-eu-central-1}"
HELLO_CLIENT_ID="${HELLO_CLIENT_ID:-app_slcuDgxAEmgXkpHPePh9Acgp_hi6}"
ITINERARIES_BUCKET="${ITINERARIES_BUCKET:-tripmap-itineraries-${ACCOUNT}-${REGION}}"
REPO="${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com/tripmapd"

PREFIX="${TAG_PREFIX:-manual}"
TAG=""
SKIP_BUILD=0
PATCH_VIEWER=0
TRIPS=()

usage() {
  sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix)
      PREFIX="${2:?}"
      shift 2
      ;;
    --tag)
      TAG="${2:?}"
      shift 2
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    --patch-viewer)
      PATCH_VIEWER=1
      shift
      ;;
    -h|--help)
      usage
      ;;
    *)
      TRIPS+=("$1")
      shift
      ;;
  esac
done

if [[ ${#TRIPS[@]} -eq 0 ]]; then
  TRIPS=(nz-4weeks holland)
fi

if [[ -z "$TAG" ]]; then
  TAG="${PREFIX}-$(date +%Y%m%d%H%M%S)"
fi

echo "== identity =="
aws sts get-caller-identity --query Arn --output text

if [[ "$SKIP_BUILD" -eq 0 ]]; then
  echo "== ecr login =="
  aws ecr get-login-password --region "$REGION" \
    | docker login --username AWS --password-stdin "${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com"

  echo "== docker build $TAG =="
  docker build --platform linux/amd64 -t "$REPO:$TAG" -t "$REPO:latest" .

  echo "== docker push =="
  docker push "$REPO:$TAG"
  docker push "$REPO:latest"
else
  echo "== skip build; using tag $TAG =="
fi

echo "== cloudformation deploy ImageTag=$TAG =="
aws cloudformation deploy \
  --stack-name tripmap-compute \
  --template-file infra/compute.yaml \
  --region "$REGION" \
  --parameter-overrides \
    ProjectName=tripmap \
    ImageTag="$TAG" \
    HelloClientID="$HELLO_CLIENT_ID"

if [[ "$PATCH_VIEWER" -eq 1 ]]; then
  echo "== patch viewer assets → s3://$ITINERARIES_BUCKET =="
  for id in "${TRIPS[@]}"; do
    for f in app.js chat.js style.css; do
      src="internal/bundle/viewer/$f"
      [[ -f "$src" ]] || { echo "missing $src" >&2; exit 1; }
      ct="text/javascript; charset=utf-8"
      [[ "$f" == *.css ]] && ct="text/css; charset=utf-8"
      aws s3 cp "$src" "s3://${ITINERARIES_BUCKET}/trips/${id}/bundle/${f}" \
        --content-type "$ct" \
        --cache-control "private, max-age=60"
    done
    echo "  patched $id"
  done
fi

echo "== wait for ECS rollout =="
for i in $(seq 1 20); do
  read -r count rollout running desired < <(
    aws ecs describe-services \
      --region "$REGION" \
      --cluster default \
      --services tripmap \
      --query '[length(services[0].deployments), services[0].deployments[0].rolloutState, services[0].runningCount, services[0].desiredCount]' \
      --output text
  )
  echo "  check $i deployments=$count rollout=$rollout running=$running desired=$desired"
  if [[ "$count" == "1" && "$rollout" == "COMPLETED" && "$running" == "$desired" && "$running" != "0" ]]; then
    break
  fi
  sleep 12
done

echo "== health =="
curl -fsS -m 20 "https://tripmap.sheffer.org/health"
echo
echo "DONE tag=$TAG"
printf '%s\n' "$TAG" > /tmp/tripmap-deploy-tag.txt
