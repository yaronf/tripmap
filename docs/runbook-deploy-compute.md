# Runbook: deploy seasonal compute

Stand up `tripmap-compute` (EC2 `t4g.micro` + Docker) for an in-season window. Data (`tripmap-data`) must already exist.

**Region:** `eu-central-1`  
**Identities:**
- **`yaron-admin`** — once, before first EC2 create: deploy updated `tripmap-deploy-iam` (see Phase B below).
- **`tripmap-deploy`** — day-to-day image/compute/edge (`aws login`).

## 0. Preconditions

- [ ] `tripmap-data` stack is `CREATE_COMPLETE` / `UPDATE_COMPLETE`
- [ ] `tripmap-edge` (CloudFront) exists — durable URL `https://tripmap.sheffer.org`
- [ ] Phase B: `tripmap-deploy-iam` includes EC2/SSM/instance-role permissions (admin)
- [ ] Local `.env` has `DESCOPE_PROJECT_ID` and `AGENT_BEARER_TOKEN` (never commit)
- [ ] `config/users.csv` present on the build machine (baked into the image)

## Phase B — admin prerequisite (`yaron-admin`)

Required **before** the first EC2 `tripmap-compute` create. `tripmap-deploy` cannot update this stack.

```bash
aws cloudformation deploy \
  --stack-name tripmap-deploy-iam \
  --template-file infra/deploy-iam.yaml \
  --region eu-central-1 \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides ProjectName=tripmap DeployUserName=tripmap-deploy
```

ECS Express permissions remain in the policy until [runbook-ec2-cleanup.md](runbook-ec2-cleanup.md) (Phase D).

## 1–2. Build, push, and update compute (`tripmap-deploy`)

```bash
./scripts/deploy-compute.sh
# or: ./scripts/deploy-compute.sh --prefix rebuild
# force new AL2023 AMI (brief downtime):
#   ./scripts/deploy-compute.sh --refresh-ami
```

The script builds **`linux/arm64`**, pushes to ECR, deploys `infra/compute.yaml` (auto-detects default VPC/subnet + CloudFront prefix list), waits for SSM, pulls/recreates the container, and hits `/health`.

Viewer assets are **served from this image**. Deploy a new tag to change the PWA.

Reuse a tag: `./scripts/deploy-compute.sh --skip-build --tag existing-tag`.

### Manual CFN knobs

| Parameter | Notes |
|-----------|--------|
| `ImageTag` | ECR tag; also written to SSM `/tripmap/compute/image-tag` |
| `AmiRefreshToken` / `AmiId` | Bumped together on refresh; Launch Template new version **replaces** the instance (real AMI refresh). Monthly EventBridge or `--refresh-ami` |
| `VpcId` / `SubnetId` | Default VPC public subnet |
| `CloudFrontPrefixListId` | `com.amazonaws.global.cloudfront.origin-facing` |
| `DescopeProjectID` | Public; empty disables login |
| `OriginHostname` | Default `tripmap-origin.sheffer.org` (DNS at GoDaddy) |

Secrets are **not** in CFN. The instance role reads SM by ARN; `tripmapd` uses `AGENT_BEARER_SECRET_ARN`, `SESSION_SECRET_ARN`, `OPENAI_SECRET_ARN`.

## 3. Point CloudFront at the EC2 origin (`tripmap-deploy`)

After first create (or EIP change):

1. Read the Elastic IP from compute outputs.
2. CloudFront **rejects raw IP origins**. Use a hostname:
   - Preferred: GoDaddy **A** `tripmap-origin` → EIP, then `ORIGIN=tripmap-origin.sheffer.org`
   - Temporary: `ORIGIN=${EIP}.sslip.io` (works immediately; switch to GoDaddy when ready)
3. Update edge:

```bash
EIP=$(aws cloudformation describe-stacks \
  --stack-name tripmap-compute --region eu-central-1 \
  --query "Stacks[0].Outputs[?OutputKey=='ElasticIP'].OutputValue" --output text)
ORIGIN="${ORIGIN:-${EIP}.sslip.io}"
# After GoDaddy A record: ORIGIN=tripmap-origin.sheffer.org

CERT_ARN=arn:aws:acm:us-east-1:077804408159:certificate/db2fef17-5f20-4863-a0ef-3819c83a8ec9

aws cloudformation deploy \
  --stack-name tripmap-edge \
  --template-file infra/edge.yaml \
  --region eu-central-1 \
  --parameter-overrides \
    ProjectName=tripmap \
    AlternateDomainName=tripmap.sheffer.org \
    AcmCertificateArn="$CERT_ARN" \
    OriginDomainName="$ORIGIN"

DID=$(aws cloudformation describe-stacks --stack-name tripmap-edge --region eu-central-1 \
  --query "Stacks[0].Outputs[?OutputKey=='DistributionId'].OutputValue" --output text)
aws cloudfront wait distribution-deployed --id "$DID"
```

Public URL stays **`https://tripmap.sheffer.org`** (GoDaddy CNAME → CloudFront). Edge uses **HTTP :80** to the origin; SG allows only the CloudFront prefix list.

## 4. Smoke (`tripmap-deploy` / browser)

```bash
set -a && source .env && set +a
curl -fsS "https://tripmap.sheffer.org/health"
curl -fsS "https://tripmap.sheffer.org/openapi.yaml" | head
BASE_URL="https://tripmap.sheffer.org" TOKEN="$AGENT_BEARER_TOKEN" ./scripts/smoke-agent.sh
```

- [ ] Sign-in → `/me/trips/{id}/`; notes + comments
- [ ] Optional chat: [runbook-viewer-chat.md](runbook-viewer-chat.md)
- [ ] MCP: [runbook-mcp.md](runbook-mcp.md)

## 5. Done when

- `/health` OK, MCP + viewer OK
- Edge origin is `tripmap-origin.sheffer.org` (or EIP) over HTTP
- Monthly AMI refresh rule exists on the compute stack (`tripmap-ec2-ami-refresh`)

## Day-to-day image roll

`./scripts/deploy-compute.sh` (or `--skip-build --tag …`). No edge change unless EIP changed. AMI replace is automatic monthly (EventBridge) or `--refresh-ami`.

## Off-season

See [runbook-undeploy-compute.md](runbook-undeploy-compute.md). After Express is fully retired, see [runbook-ec2-cleanup.md](runbook-ec2-cleanup.md).
