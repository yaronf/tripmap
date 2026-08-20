# Runbook: deploy seasonal compute

Stand up `tripmap-compute` (ECS Express Mode) for an in-season window. Data (`tripmap-data`) must already exist.

**Region:** `eu-central-1`  
**Identity:** prefer `tripmap-deploy` (`aws login`); use `yaron-admin` only if IAM/policy changes are needed.

After a full compute **delete**, recreate needs `ecs:CreateCluster` on the deploy policy (see `infra/deploy-iam.yaml`). If create fails with AccessDenied on `CreateCluster`, update `tripmap-deploy-iam` as `yaron-admin` first, then retry as `tripmap-deploy`.

## 0. Preconditions

- [ ] `tripmap-data` stack is `CREATE_COMPLETE` / `UPDATE_COMPLETE`
- [ ] `tripmap-edge` (CloudFront) exists — durable URL `https://tripmap.sheffer.org`
- [ ] Image exists in ECR `tripmapd` (build/push if you changed code)
- [ ] Local `.env` has `AGENT_BEARER_TOKEN` (never commit; not readable by `tripmap-deploy`)
- [ ] Agent Bearer already configured for MCP / scripts (unchanged across seasons)

## 1–2. Build, push, and update compute (preferred)

As `tripmap-deploy`, from the repo root:

```bash
./scripts/deploy-compute.sh
# or: ./scripts/deploy-compute.sh --prefix rebuild
```

Viewer `app.js` / CSS / icons are **served from this image**, not patched onto S3. A laptop `aws s3 cp` of those files is overwritten the next time an itinerary regen runs `bundle.Build`. Deploy a new tag to change the PWA.

That script ECR-logins, builds `linux/amd64`, pushes the tag + `latest`, runs `aws cloudformation deploy` on `tripmap-compute`, waits for ECS, and hits `/health`.

Manual steps below remain if you need to reuse an existing tag or change only one piece.

### Manual: build and push

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

Skip build if reusing an existing tag (e.g. last known-good `openapi-gpt-…`), or:
`./scripts/deploy-compute.sh --skip-build --tag existing-tag`.

### Manual: create or update compute stack

```bash
TAG=…   # from step 1, or an existing ECR tag
# HelloClientID is public (Hellō console). Empty disables /auth/hello/*.
HELLO_CLIENT_ID=app_slcuDgxAEmgXkpHPePh9Acgp_hi6

aws cloudformation deploy \
  --stack-name tripmap-compute \
  --template-file infra/compute.yaml \
  --region eu-central-1 \
  --parameter-overrides \
    ProjectName=tripmap \
    ImageTag="$TAG" \
    HelloClientID="$HELLO_CLIENT_ID"
```

Hellō console must list redirect URI `https://tripmap.sheffer.org/auth/hello/callback` (exact path).

`HELLO_SESSION_SECRET` is injected from Secrets Manager `tripmap/hello-session` (data stack). Do not reuse the agent Bearer. Signed-in ACL is `config/users.csv` (gitignored; baked into the image from the deploy machine). Start from `config/users.example.csv`.

In-viewer chat (optional): `OPENAI_SECRET_JSON` from `tripmap/openai` + `OPENAI_MODEL=gpt-5-mini`; require `chat=yes` rows. See [runbook-viewer-chat.md](runbook-viewer-chat.md).

Day-to-day **image-only** updates (same stack, new `ImageTag`) typically finish in ~3 minutes after the Express bake tweak below. A full seasonal **create** still needs edge (step 3) and the bake step.

## 2b. Express bake settings (required after create / recreate)

Express Mode defaults to a slow canary (~3 min bake at 5% + ~3 min full bake ≈ 8–10 min per deploy). We shorten that with classic `update-service` (survives later CFN `ImageTag` updates; **resets on seasonal delete+create**).

**Always run after a new `tripmap-compute` create** (and whenever deploys feel ~9 minutes again):

```bash
REGION=eu-central-1
CLUSTER=default
SERVICE=tripmap   # Express ServiceName = ProjectName

# Confirm current settings
aws ecs describe-services --region "$REGION" --cluster "$CLUSTER" --services "$SERVICE" \
  --query 'services[0].deploymentConfiguration.{bake:bakeTimeInMinutes,canary:canaryConfiguration,strategy:strategy}' \
  --output json

# Fast path: 100% canary, zero bake (TripMap is low-risk / single-tenant)
aws ecs update-service --region "$REGION" --cluster "$CLUSTER" --service "$SERVICE" \
  --deployment-configuration \
  '{"bakeTimeInMinutes":0,"canaryConfiguration":{"canaryPercent":100,"canaryBakeTimeInMinutes":0}}' \
  --query 'service.deploymentConfiguration.{bake:bakeTimeInMinutes,canary:canaryConfiguration}' \
  --output json
```

Expect after apply: `bakeTimeInMinutes: 0`, `canaryPercent: 100`, `canaryBakeTimeInMinutes: 0`.

Background and measured timings: [`plan-fast-app-deploys.md`](plan-fast-app-deploys.md).

## 3. Point CloudFront at the new Express origin

Express TLS is on `*.ecs.*.on.aws` (stack `Endpoint`), not the raw ALB DNS name. After create/recreate:

```bash
ENDPOINT=$(aws cloudformation describe-stacks \
  --stack-name tripmap-compute --region eu-central-1 \
  --query "Stacks[0].Outputs[?OutputKey=='Endpoint'].OutputValue" \
  --output text)
CERT_ARN=arn:aws:acm:us-east-1:077804408159:certificate/db2fef17-5f20-4863-a0ef-3819c83a8ec9

aws cloudformation deploy \
  --stack-name tripmap-edge \
  --template-file infra/edge.yaml \
  --region eu-central-1 \
  --parameter-overrides \
    ProjectName=tripmap \
    AlternateDomainName=tripmap.sheffer.org \
    AcmCertificateArn="$CERT_ARN" \
    OriginDomainName="$ENDPOINT"

DID=$(aws cloudformation describe-stacks --stack-name tripmap-edge --region eu-central-1 \
  --query "Stacks[0].Outputs[?OutputKey=='DistributionId'].OutputValue" --output text)
aws cloudfront wait distribution-deployed --id "$DID"
```

Public URL stays **`https://tripmap.sheffer.org`** (GoDaddy CNAME → CloudFront; do not change DNS on recreate).

## 4. Smoke

```bash
set -a && source .env && set +a
curl -fsS "https://tripmap.sheffer.org/health"
curl -fsS "https://tripmap.sheffer.org/openapi.yaml" | head
BASE_URL="https://tripmap.sheffer.org" TOKEN="$AGENT_BEARER_TOKEN" ./scripts/smoke-agent.sh
# Optional (needs OPENAI_* + chat=yes ACL): BASE_URL=… ./scripts/smoke-chat.sh
```

- [ ] If `HelloClientID` set: open `https://tripmap.sheffer.org/` → Continue with Hellō → open `/me/trips/{id}/`; confirm notes + comments
- [ ] `GET /auth/me` returns `authenticated: true` (and `chat_enabled: true` when OpenAI + `chat=yes`)
- [ ] Optional: Ask pane + [runbook-viewer-chat.md](runbook-viewer-chat.md)

## 5. ChatGPT Agent MCP

- [ ] Connector: Streamable HTTP `https://tripmap.sheffer.org/mcp` + Bearer env `AGENT_BEARER_TOKEN` — see [runbook-mcp.md](runbook-mcp.md)
- [ ] Quick test in ChatGPT Agent: “List trips.”

## 6. Done when

- `https://tripmap.sheffer.org/health` OK, `/mcp` lists tools with Bearer, ChatGPT Agent list-trips works, signed-in viewer + comments OK.
- After **create/recreate**: step **2b** bake settings applied (or verified already `0` / `100%`).

## Day-to-day image roll (compute already up)

Same as steps **1 → 2 → 4** (skip edge unless `Endpoint` changed). No need to re-run **2b** unless describe-services shows bake times back at `3` / canary `5`.
