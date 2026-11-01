#!/usr/bin/env bash
# Build/push tripmapd to ECR and update tripmap-compute (EC2 + Docker).
#
# Viewer JS/CSS/icons are served from this image, not copied onto S3. To
# update the PWA, deploy a new tag — do not aws s3 cp viewer files.
#
# Identity: tripmap-deploy (aws login). Requires Phase B deploy-iam update.
#
#   ./scripts/deploy-compute.sh
#   ./scripts/deploy-compute.sh --prefix rebuild
#   ./scripts/deploy-compute.sh --skip-build --tag existing-tag
#   ./scripts/deploy-compute.sh --refresh-ami   # force new AL2023 AMI (brief downtime)
#
# Optional env:
#   TAG_PREFIX=manual  ACCOUNT=077804408159  REGION=eu-central-1
#   DESCOPE_PROJECT_ID=P2xxx...
#   GOOGLE_SITE_VERIFICATION=...
#   VPC_ID=  SUBNET_ID=  CLOUDFRONT_PREFIX_LIST_ID=  (auto-detected if unset)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export PATH="${HOME}/.local/bin:${PATH}"
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy

# Load local secrets if present (DESCOPE_PROJECT_ID, etc.). Explicit env wins.
if [[ -f "$ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/.env"
  set +a
fi

ACCOUNT="${ACCOUNT:-077804408159}"
REGION="${REGION:-eu-central-1}"
DESCOPE_PROJECT_ID="${DESCOPE_PROJECT_ID:-}"
GOOGLE_SITE_VERIFICATION="${GOOGLE_SITE_VERIFICATION:-}"
REPO="${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com/tripmapd"
STACK=tripmap-compute

PREFIX="${TAG_PREFIX:-manual}"
TAG=""
SKIP_BUILD=0
REFRESH_AMI=0

usage() {
  sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
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
    --refresh-ami)
      REFRESH_AMI=1
      shift
      ;;
    --patch-viewer)
      echo "error: --patch-viewer was removed." >&2
      echo "Viewer files are served from the container image; deploy a new tag instead." >&2
      exit 2
      ;;
    -h|--help)
      usage
      ;;
    *)
      echo "error: unexpected argument: $1" >&2
      usage
      ;;
  esac
done

if [[ -z "$TAG" ]]; then
  TAG="${PREFIX}-$(date +%Y%m%d%H%M%S)"
fi

echo "== identity =="
CALLER="$(aws sts get-caller-identity --query Arn --output text)"
echo "$CALLER"
if [[ "$CALLER" != *tripmap-deploy* ]]; then
  echo "warning: expected tripmap-deploy identity; continuing with $CALLER" >&2
fi

if [[ "$SKIP_BUILD" -eq 0 ]]; then
  echo "== ecr login =="
  aws ecr get-login-password --region "$REGION" \
    | docker login --username AWS --password-stdin "${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com"

  echo "== docker build linux/arm64 $TAG =="
  docker build --platform linux/arm64 \
    --build-arg TARGETARCH=arm64 \
    -t "$REPO:$TAG" -t "$REPO:latest" .

  echo "== docker push =="
  docker push "$REPO:$TAG"
  docker push "$REPO:latest"
else
  echo "== skip build; using tag $TAG =="
fi

if [[ -z "$DESCOPE_PROJECT_ID" ]]; then
  echo "error: DESCOPE_PROJECT_ID is empty (set it in .env or the environment)." >&2
  echo "Passing an empty value clears Google sign-in on the home page." >&2
  exit 1
fi

# Network params: prefer env, else detect default VPC / CloudFront prefix list.
if [[ -z "${VPC_ID:-}" ]]; then
  VPC_ID="$(aws ec2 describe-vpcs --region "$REGION" --filters Name=isDefault,Values=true \
    --query 'Vpcs[0].VpcId' --output text)"
fi
if [[ -z "${SUBNET_ID:-}" ]]; then
  SUBNET_ID="$(aws ec2 describe-subnets --region "$REGION" \
    --filters "Name=vpc-id,Values=$VPC_ID" Name=default-for-az,Values=true \
    --query 'Subnets[0].SubnetId' --output text)"
fi
if [[ -z "${CLOUDFRONT_PREFIX_LIST_ID:-}" ]]; then
  CLOUDFRONT_PREFIX_LIST_ID="$(aws ec2 describe-managed-prefix-lists --region "$REGION" \
    --query "PrefixLists[?PrefixListName=='com.amazonaws.global.cloudfront.origin-facing'].PrefixListId | [0]" \
    --output text)"
fi
if [[ -z "$VPC_ID" || "$VPC_ID" == "None" || -z "$SUBNET_ID" || "$SUBNET_ID" == "None" ]]; then
  echo "error: could not resolve VPC_ID/SUBNET_ID (set them explicitly)." >&2
  exit 1
fi
if [[ -z "$CLOUDFRONT_PREFIX_LIST_ID" || "$CLOUDFRONT_PREFIX_LIST_ID" == "None" ]]; then
  echo "error: could not resolve CloudFront origin-facing prefix list id." >&2
  exit 1
fi

AMI_TOKEN="init"
PREV_AMI=""
if aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" &>/dev/null; then
  AMI_TOKEN="$(aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" \
    --query "Stacks[0].Parameters[?ParameterKey=='AmiRefreshToken'].ParameterValue | [0]" \
    --output text)"
  PREV_AMI="$(aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" \
    --query "Stacks[0].Parameters[?ParameterKey=='AmiId'].ParameterValue | [0]" \
    --output text)"
  if [[ -z "$AMI_TOKEN" || "$AMI_TOKEN" == "None" ]]; then
    AMI_TOKEN="init"
  fi
fi
if [[ "$REFRESH_AMI" -eq 1 ]]; then
  AMI_TOKEN="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "== AMI refresh requested; AmiRefreshToken=$AMI_TOKEN (instance will be replaced) =="
fi

# Resolve latest AL2023 arm64 AMI (GetParameter is allowed; CFN dynamic resolve is not).
AMI_ID="$(aws ssm get-parameter --region "$REGION" \
  --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64 \
  --query Parameter.Value --output text)"
if [[ "$REFRESH_AMI" -eq 0 && -n "$PREV_AMI" && "$PREV_AMI" != "None" ]]; then
  AMI_ID="$PREV_AMI"
fi
echo "== AmiId=$AMI_ID =="

CFN_PARAMS=(
  "ProjectName=tripmap"
  "ImageTag=$TAG"
  "DescopeProjectID=$DESCOPE_PROJECT_ID"
  "AmiRefreshToken=$AMI_TOKEN"
  "AmiId=$AMI_ID"
  "VpcId=$VPC_ID"
  "SubnetId=$SUBNET_ID"
  "CloudFrontPrefixListId=$CLOUDFRONT_PREFIX_LIST_ID"
)
if [[ -n "$GOOGLE_SITE_VERIFICATION" ]]; then
  CFN_PARAMS+=("GoogleSiteVerification=$GOOGLE_SITE_VERIFICATION")
fi

echo "== cloudformation deploy ImageTag=$TAG =="
echo "  VpcId=$VPC_ID SubnetId=$SUBNET_ID PrefixList=$CLOUDFRONT_PREFIX_LIST_ID"
aws cloudformation deploy \
  --stack-name "$STACK" \
  --template-file infra/compute.yaml \
  --region "$REGION" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides "${CFN_PARAMS[@]}"

INSTANCE_ID="$(aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" \
  --query "Stacks[0].Outputs[?OutputKey=='InstanceId'].OutputValue | [0]" --output text)"
echo "== instance $INSTANCE_ID =="

echo "== wait for SSM + docker =="
for i in $(seq 1 60); do
  ping_status="$(aws ssm describe-instance-information --region "$REGION" \
    --filters "Key=InstanceIds,Values=$INSTANCE_ID" \
    --query 'InstanceInformationList[0].PingStatus' --output text 2>/dev/null || echo None)"
  echo "  check $i PingStatus=$ping_status"
  if [[ "$ping_status" == "Online" ]]; then
    break
  fi
  sleep 15
done
if [[ "$ping_status" != "Online" ]]; then
  echo "error: instance not Online in SSM" >&2
  exit 1
fi
# User-data installs docker on first boot; wait before pull/restart.
for i in $(seq 1 40); do
  ready_cmd="$(aws ssm send-command --region "$REGION" --instance-ids "$INSTANCE_ID" \
    --document-name AWS-RunShellScript \
    --parameters 'commands=["command -v docker >/dev/null && systemctl is-active docker"]' \
    --query 'Command.CommandId' --output text)"
  sleep 8
  ready_st="$(aws ssm get-command-invocation --region "$REGION" \
    --command-id "$ready_cmd" --instance-id "$INSTANCE_ID" \
    --query '[Status,StandardOutputContent]' --output text 2>/dev/null || echo Pending)"
  echo "  docker check $i $ready_st"
  if [[ "$ready_st" == Success$'\t'active* ]] || [[ "$ready_st" == *"Success"*active* ]]; then
    break
  fi
done

ITINERARIES_BUCKET="$(aws cloudformation list-exports --region "$REGION" \
  --query "Exports[?Name=='tripmap-ItinerariesBucket'].Value | [0]" --output text)"
COMMENTS_BUCKET="$(aws cloudformation list-exports --region "$REGION" \
  --query "Exports[?Name=='tripmap-CommentsBucket'].Value | [0]" --output text)"
PUBLIC_BASE_URL="$(aws cloudformation list-exports --region "$REGION" \
  --query "Exports[?Name=='tripmap-PublicBaseURL'].Value | [0]" --output text)"
AGENT_ARN="$(aws cloudformation list-exports --region "$REGION" \
  --query "Exports[?Name=='tripmap-AgentBearerSecretArn'].Value | [0]" --output text)"
SESSION_ARN="$(aws cloudformation list-exports --region "$REGION" \
  --query "Exports[?Name=='tripmap-HelloSessionSecretArn'].Value | [0]" --output text)"
OPENAI_ARN="$(aws cloudformation list-exports --region "$REGION" \
  --query "Exports[?Name=='tripmap-OpenAISecretArn'].Value | [0]" --output text)"

# Idempotent pull/recreate (ImageTag-only updates without AMI replace). Values
# are expanded here — the instance role cannot call CloudFormation.
RESTART_SCRIPT=$(cat <<EOF
set -euxo pipefail
aws ecr get-login-password --region ${REGION} \
  | docker login --username AWS --password-stdin ${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com
docker pull ${REPO}:${TAG}
docker rm -f tripmapd 2>/dev/null || true
docker run -d --name tripmapd --restart=always \
  -p 80:8080 \
  -e ADDR=:8080 \
  -e AWS_REGION=${REGION} \
  -e ITINERARIES_BUCKET=${ITINERARIES_BUCKET} \
  -e COMMENTS_BUCKET=${COMMENTS_BUCKET} \
  -e ROUTE_MODE=osrm \
  -e PUBLIC_BASE_URL=${PUBLIC_BASE_URL} \
  -e DESCOPE_PROJECT_ID=${DESCOPE_PROJECT_ID} \
  -e GOOGLE_SITE_VERIFICATION=${GOOGLE_SITE_VERIFICATION} \
  -e OPENAI_MODEL=gpt-5-mini \
  -e AGENT_BEARER_SECRET_ARN=${AGENT_ARN} \
  -e SESSION_SECRET_ARN=${SESSION_ARN} \
  -e OPENAI_SECRET_ARN=${OPENAI_ARN} \
  ${REPO}:${TAG}
EOF
)

echo "== SSM pull/restart tag=$TAG =="
PARAMS_FILE="$(mktemp)"
trap 'rm -f "$PARAMS_FILE"' EXIT
jq -n --arg s "$RESTART_SCRIPT" '{commands:[$s]}' >"$PARAMS_FILE"
CMD_ID="$(aws ssm send-command --region "$REGION" \
  --instance-ids "$INSTANCE_ID" \
  --document-name AWS-RunShellScript \
  --parameters "file://$PARAMS_FILE" \
  --query 'Command.CommandId' --output text)"

for i in $(seq 1 40); do
  status="$(aws ssm get-command-invocation --region "$REGION" \
    --command-id "$CMD_ID" --instance-id "$INSTANCE_ID" \
    --query 'Status' --output text 2>/dev/null || echo Pending)"
  echo "  check $i SSM=$status"
  case "$status" in
    Success) break ;;
    Failed|Cancelled|TimedOut)
      aws ssm get-command-invocation --region "$REGION" \
        --command-id "$CMD_ID" --instance-id "$INSTANCE_ID" \
        --query '[Status,StandardOutputContent,StandardErrorContent]' --output text >&2
      exit 1
      ;;
  esac
  sleep 10
done

echo "== health =="
curl -fsS -m 20 "https://tripmap.sheffer.org/health"
echo
EIP="$(aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" \
  --query "Stacks[0].Outputs[?OutputKey=='ElasticIP'].OutputValue | [0]" --output text)"
echo "DONE tag=$TAG instance=$INSTANCE_ID eip=$EIP"
printf '%s\n' "$TAG" > /tmp/tripmap-deploy-tag.txt
