# Runbook: deploy seasonal compute

Stand up `tripmap-compute` (ECS Express Mode) for an in-season window. Data (`tripmap-data`) must already exist.

**Region:** `eu-central-1`  
**Identity:** prefer `tripmap-deploy` (`aws login`); use `yaron-admin` only if IAM/policy changes are needed.

After a full compute **delete**, recreate needs `ecs:CreateCluster` on the deploy policy (see `infra/deploy-iam.yaml`). If create fails with AccessDenied on `CreateCluster`, update `tripmap-deploy-iam` as `yaron-admin` first, then retry as `tripmap-deploy`.

## 0. Preconditions

- [ ] `tripmap-data` stack is `CREATE_COMPLETE` / `UPDATE_COMPLETE`
- [ ] Image exists in ECR `tripmapd` (build/push if you changed code)
- [ ] Local `.env` has `AGENT_BEARER_TOKEN` (never commit; not readable by `tripmap-deploy`)
- [ ] Custom GPT Bearer already configured (unchanged across seasons)

## 1. Build and push (if needed)

```bash
cd /path/to/tripmap
export PATH="$HOME/.local/bin:$PATH"
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy

TAG="manual-$(date +%Y%m%d%H%M%S)"
ACCOUNT=077804408159
REGION=eu-central-1
REPO="$ACCOUNT.dkr.ecr.$REGION.amazonaws.com/tripmapd"

aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "$ACCOUNT.dkr.ecr.$REGION.amazonaws.com"

docker build --platform linux/amd64 -t "$REPO:$TAG" -t "$REPO:latest" .
docker push "$REPO:$TAG"
docker push "$REPO:latest"
```

Skip this section if reusing an existing tag (e.g. last known-good `openapi-gpt-…`).

## 2. Create or update compute stack

```bash
TAG=…   # from step 1, or an existing ECR tag
aws cloudformation deploy \
  --stack-name tripmap-compute \
  --template-file infra/compute.yaml \
  --region eu-central-1 \
  --parameter-overrides ProjectName=tripmap ImageTag="$TAG"
```

Express Mode updates often take several minutes.

## 3. Record ServiceUrl

```bash
ENDPOINT=$(aws cloudformation describe-stacks \
  --stack-name tripmap-compute --region eu-central-1 \
  --query "Stacks[0].Outputs[?OutputKey=='Endpoint'].OutputValue" \
  --output text)
echo "https://$ENDPOINT"
```

- [ ] Save `https://$ENDPOINT` in password manager (hostname **may change** on recreate)
- [ ] Rebuild partner/capability links as `https://$ENDPOINT/t/{id}/{token}/` (tokens unchanged in S3)

## 4. Smoke

```bash
set -a && source .env && set +a
curl -fsS "https://$ENDPOINT/health"
curl -fsS "https://$ENDPOINT/openapi.yaml" | head
BASE_URL="https://$ENDPOINT" TOKEN="$AGENT_BEARER_TOKEN" ./scripts/smoke-agent.sh
```

- [ ] Open one saved capability URL in a browser
- [ ] Confirm shared notes still load

## 5. Custom GPT Actions

Hostname may have changed after a **stack recreate** (delete + create). Image-only updates often keep the same host.

- [ ] GPT → Actions → confirm **server** is `https://$ENDPOINT`
- [ ] Prefer **Import from URL**: `https://$ENDPOINT/openapi.yaml` (refresh schema after OpenAPI changes)
- [ ] Quick test: “List trips.”

**TODO:** durable public URL so this step is rarely needed — see [TODO.md](../TODO.md).

## 6. Done when

- `/health` OK, OpenAPI importable, GPT list-trips works, one capability URL + notes OK.
