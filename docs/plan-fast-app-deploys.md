# Plan: shorter time-to-deploy

**Goal:** wall-clock until the new image is serving traffic.  
**Status:** Express API “fast path” **rejected** (2026-08-08) — not faster, adds ImageTag drift.

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

Caveat: this uses classic `update-service` on an Express service (not officially documented on `update-express-gateway-service`). Re-apply after seasonal recreate if defaults return.

Do **not** reintroduce a parallel non-CFN image-update script.
