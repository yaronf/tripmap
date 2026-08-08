# Plan: shorter time-to-deploy

**Goal:** wall-clock until the new image is serving traffic.  
**Status:** Express API “fast path” **rejected** (2026-08-08) — not faster, adds ImageTag drift.

## Rejected approach: bypass CloudFormation

`UpdateExpressGatewayService` after ECR push returns in ~2s, but Express Mode’s canary/rollout still took **~8–9 minutes** before the new revision was `SUCCESSFUL`. A prior full `cloudformation deploy` image roll was ~545s end-to-end — same bottleneck.

Keeping a non-CFN image path would leave `ImageTag` in `tripmap-compute` stale. That complexity is not worth it for this project. **Stay on:** build/push → `aws cloudformation deploy` with `ImageTag` (see [`runbook-deploy-compute.md`](runbook-deploy-compute.md)).

One-off note: after the rejected experiment, live image was `sw-geo-20260808222659` while the stack parameter may still name an older tag. The **next** normal CFN deploy (any new tag) reconciles that; no separate process.

## Where the time actually goes

Confirmed: **`AWS::ECS::ExpressGatewayService` rollout / canary**, not docker build/push or S3/IAM.

## Next ideas (only if we optimize time again)

Prefer changing *how Express rolls*, not adding a second deploy tool:

1. **Express / ECS deployment settings** — shorter bake time, disable or tighten canary/alarm waits if the console/API exposes them for Express Gateway (measure before/after).
2. **Skip canary for this service** if Express allows a simpler rolling replace — TripMap is a single low-risk app; an 8-minute safety canary may be overkill.
3. **Keep CFN** as the only app deploy path; optional CI job that times `cloudformation deploy` + stack events so we know which resource ate the minutes after each change.

Do **not** reintroduce a parallel image-update script unless measurement shows CFN itself (not Express) owns a large share of the wait.
