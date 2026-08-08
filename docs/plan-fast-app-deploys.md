# Plan: faster app deploys (bypass CloudFormation for image rolls)

**Status:** backlog — not an immediate fix.  
**Context:** Day-to-day `tripmap-compute` updates often take ~5 minutes. That is normal for stacks whose update path goes through slow, stateful resources (here: ECS Express Gateway). Goal: **deploy infrastructure less often** and make **application changes bypass CloudFormation**.

AWS guidance: organize stacks by [lifecycle and ownership](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/best-practices.html); use [change sets](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/using-cfn-updating-stacks-changesets.html) for preview/safety.

---

## Current layout (already mostly right)

| Stack | Template | Lifecycle | Touch for app code? |
|-------|----------|-----------|---------------------|
| `tripmap-data` | `infra/data.yaml` | Persistent (S3, secrets, ECR, IAM) | No |
| `tripmap-edge` | `infra/edge.yaml` | Semi-static (CloudFront → Express origin) | Only when `Endpoint` changes |
| `tripmap-deploy-iam` | `infra/deploy-iam.yaml` | Rare | No |
| `tripmap-compute` | `infra/compute.yaml` | Seasonal + every image tag change today | **Yes — bottleneck** |

Image build/push already runs **before** CloudFormation in [`runbook-deploy-compute.md`](runbook-deploy-compute.md). The slow step is almost certainly **`AWS::ECS::ExpressGatewayService`** reconciling a new `ImageTag` (rolling deploy + health), not S3/ECR/IAM.

**First diagnostic (when we pick this up):**

```bash
aws cloudformation describe-stack-events \
  --stack-name tripmap-compute --region eu-central-1 \
  --max-items 40 \
  --query 'StackEvents[].{t:Timestamp,s:ResourceStatus,r:LogicalResourceId,m:ResourceStatusReason}' \
  --output table
```

Confirm which resource owns most of the wall clock. Expect `ExpressService` → `UPDATE_IN_PROGRESS` for nearly the whole window.

---

## Target release paths

```text
foundation / data / edge / deploy-iam   rare CFN updates
app image roll                          ECR push → UpdateExpressGatewayService (no CFN)
infra / env / scaling / HelloClientID   CFN change set → deploy compute
seasonal create/delete                  existing compute runbooks
```

```text
release pipeline (desired)
  1. docker build + push immutable tag (+ optional :latest)
  2. aws ecs update-express-gateway-service  (fast path)
  3. smoke + optional bundle regen
  4. periodically: CFN deploy with ImageTag=that tag  (reconcile drift)
```

---

## Recommended work (ordered)

### 1. Fast “app-only” path (highest leverage)

Use the Express API instead of updating the stack for routine image rolls:

- [`UpdateExpressGatewayService`](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_UpdateExpressGatewayService.html) / CLI [`update-express-gateway-service`](https://docs.aws.amazon.com/cli/latest/reference/ecs/update-express-gateway-service.html)
- Resolve service ARN from stack output / `describe-express-gateway-services`
- Pass new image URI; keep env/secrets unchanged
- Wait on Express deployment status (or poll `/health` via CloudFront), not on CloudFormation

Add a runbook section + thin script, e.g. `scripts/deploy-app-image.sh`:

1. Build/push tag (same as today)
2. `update-express-gateway-service --primary-container image=…`
3. Smoke (`/health`, OpenAPI version, `smoke-agent.sh`)
4. Regenerate `holland` / `nz-4weeks` bundles when the viewer embedded in the image changes

**Drift policy:** CloudFormation’s `ImageTag` parameter will lag after fast-path deploys. Accept that for in-season iteration; reconcile on the next intentional CFN deploy (seasonal recreate, env change, or weekly “truth” deploy). Document “source of truth for running image = ECS Express revision; CFN ImageTag may be stale.”

**IAM:** extend `tripmap-deploy` policy with `ecs:UpdateExpressGatewayService` (and any Describe needed). Keep CFN update rights for structural changes.

### 2. Keep CFN for lifecycle / structure only

Continue using CloudFormation when:

- Creating/deleting seasonal compute
- Changing CPU/memory/scaling, `HelloClientID`, secrets wiring, roles
- Recreating after undeploy
- Updating edge when Express `Endpoint` hostname changes

Do **not** move buckets, secrets, ECR, or ACM into the app path. Retention policies on data resources already match “treat slow resources as durable.”

Optional later split (only if data stack grows): pull networking/DNS/ACM into a true `foundation` stack. Today edge + GoDaddy CNAME already isolate DNS from app rolls; low priority.

### 3. Move long work out of deploy (already partly done)

| Work | Keep out of CFN |
|------|-----------------|
| Docker build/push | CI or laptop **before** any deploy (current) |
| Bundle regen | Agent `PUT …/yaml` after new image is healthy (current) |
| YAML migrations / seed | Separate idempotent steps, never custom resources |
| Readiness | App `/health` + smoke script, not CFN waiters beyond Express’s own |

Avoid adding CloudFormation custom resources that wait on OSRM, Hellō, or bundle rebuilds.

### 4. Dependency hygiene

`compute.yaml` is already a single resource — little to strip. When adding alarms/queues later, avoid artificial `DependsOn` and prefer implicit references only where required. Keep new durable stores in `tripmap-data`, not compute.

### 5. Safer / quieter CFN paths

When CFN **is** used:

- Prefer change sets for parameter/template edits that might replace resources
- Use `--no-fail-on-empty-changeset` in any CI `cloudformation deploy` so no-op runs are green
- Never delete `tripmap-data`; keep Retain on buckets (already)

### 6. Optional CI shape

`workflow_dispatch` jobs (still open in aws-deployment Phase D):

| Job | Steps |
|-----|--------|
| `deploy-app` | build → push → `update-express-gateway-service` → smoke |
| `deploy-compute` | change set / CFN (ImageTag + structural params) |
| `destroy-compute` | existing undeploy runbook |

Local Cursor deploys can call the same `deploy-app` script.

---

## Non-goals (for now)

- Migrating off Express Mode to plain ECS/ALB solely for speed
- CDK / `cdk watch` (repo is raw CloudFormation; hotswap is N/A unless we adopt CDK)
- Merging edge into compute (would make seasonal undeploy harder)

---

## Done when

- [ ] One slow update’s Events timeline confirms `ExpressService` dominates time
- [ ] `scripts/deploy-app-image.sh` (or runbook equivalent) rolls a new image without `cloudformation deploy`
- [ ] Deploy IAM allows Express update APIs
- [ ] Day-to-day viewer/API changes use the fast path; CFN reserved for infra/seasonal
- [ ] Drift + reconcile notes live in `runbook-deploy-compute.md`
- [ ] Optional: GHA `deploy-app` + `--no-fail-on-empty-changeset` on CFN jobs

---

## References

- [CloudFormation best practices (lifecycle)](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/best-practices.html)
- [Change sets](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/using-cfn-updating-stacks-changesets.html)
- [Update Express Gateway service](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/express-service-update-full.html)
- Existing: [`runbook-deploy-compute.md`](runbook-deploy-compute.md), [`aws-deployment.md`](aws-deployment.md)
