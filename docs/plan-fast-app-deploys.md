# Plan: shorter time-to-deploy

**Goal:** wall-clock until the new image is serving traffic.  
**Ops (what to run day-to-day / after recreate):** [`runbook-deploy-compute.md`](runbook-deploy-compute.md) §2b.  
**Status:** Express API “fast path” **rejected** (2026-08-08). Bake tweak **adopted** (2026-08-09).

## Rejected approach: bypass CloudFormation

`UpdateExpressGatewayService` after ECR push returns in ~2s, but Express Mode’s canary/rollout still took **~8–9 minutes** before the new revision was `SUCCESSFUL`. A prior full `cloudformation deploy` image roll was ~545s end-to-end — same bottleneck.

Keeping a non-CFN image path would leave `ImageTag` in `tripmap-compute` stale. That complexity is not worth it for this project. **Stay on:** build/push → `aws cloudformation deploy` with `ImageTag` (see [`runbook-deploy-compute.md`](runbook-deploy-compute.md)).

One-off note: after the rejected experiment, live image was `sw-geo-20260808222659` while the stack parameter may still name an older tag. The **next** normal CFN deploy (any new tag) reconciles that; no separate process.

## Where the time actually goes

Confirmed: **`AWS::ECS::ExpressGatewayService` rollout / canary**, not docker build/push or S3/IAM.

## Experiment — Express bake tweak (2026-08-09)

Keep CFN for image rolls. Before deploying `sanitize-20260809010146`, set:

```bash
aws ecs update-service --cluster default --service tripmap \
  --deployment-configuration \
  '{"bakeTimeInMinutes":0,"canaryConfiguration":{"canaryPercent":100,"canaryBakeTimeInMinutes":0}}'
```

| Step | Seconds |
|------|--------:|
| set bake/canary | 2 |
| docker-build + push | 20 |
| **cloudformation-deploy** | **158** |
| wait deployment SUCCESSFUL | 1 (already done) |
| smoke + regen | 9 |
| **TOTAL** | **~192** |

Baseline CFN image roll was ~545s; Express canary alone was ~518s. After the tweak: **~3.2 min** end-to-end, and the new bake settings **survived** the CFN stack update (`bakeTimeInMinutes: 0`, `canaryPercent: 100`).

### If things change

| Situation | What to do |
|-----------|------------|
| Seasonal **delete + create** of `tripmap-compute` | Bake settings reset to AWS defaults. Re-run runbook **§2b** before relying on fast deploys. |
| Next CFN `ImageTag` update feels ~9 minutes again | `describe-services` deploymentConfiguration; if bake is `3` / canary `5`, re-apply §2b. |
| Stack update somehow resets bake (unexpected) | Same as above. Image-only updates in 2026-08-09 test **kept** the tweak. |
| Want official Express API for bake | Track AWS; until then keep the `update-service` workaround in the runbook. |

Do **not** reintroduce a parallel non-CFN image-update script.
