# Runbook: undeploy seasonal compute

Tear down `tripmap-compute` to stop ALB/Fargate charges off-season. **Do not** delete `tripmap-data`.

**Region:** `eu-central-1`  
**Identity:** `tripmap-deploy` or `yaron-admin`.

## What stays / what dies

| Kept (`tripmap-data`) | Gone until next deploy |
|----------------------|-------------------------|
| Itinerary YAML + versions | HTTPS Express endpoint |
| Capability token **hashes** in meta | Live capability URLs |
| Comments bucket objects | Agent API + GPT Actions |
| ECR images, agent Bearer secret | Viewer PWA on compute host |

Tokens themselves are unchanged; only the **hostname** is offline. After the next deploy, the same `/t/{id}/{token}/` paths work on the new (or same) host.

## 1. Optional: note current endpoint

```bash
aws cloudformation describe-stacks \
  --stack-name tripmap-compute --region eu-central-1 \
  --query "Stacks[0].Outputs[?OutputKey=='Endpoint'].OutputValue" \
  --output text
```

Useful so you know which GPT server URL / shared links will break.

## 2. Delete compute stack

```bash
aws cloudformation delete-stack \
  --stack-name tripmap-compute \
  --region eu-central-1

aws cloudformation wait stack-delete-complete \
  --stack-name tripmap-compute \
  --region eu-central-1
```

If a later create fails with **AlreadyExists** for Express service `tripmap`, an orphan survived rollback. Delete it, then recreate:

```bash
aws ecs delete-express-gateway-service \
  --service-arn arn:aws:ecs:eu-central-1:077804408159:service/default/tripmap \
  --region eu-central-1
# wait until describe-services status is INACTIVE / gone, then deploy again
```

## 3. Confirm data intact

```bash
aws cloudformation describe-stacks \
  --stack-name tripmap-data --region eu-central-1 \
  --query "Stacks[0].StackStatus" --output text

aws s3 ls s3://tripmap-itineraries-077804408159-eu-central-1/trips/
aws s3 ls s3://tripmap-comments-077804408159-eu-central-1/ --recursive | head
```

- [ ] `tripmap-data` still `CREATE_COMPLETE` / `UPDATE_COMPLETE`
- [ ] Trip prefixes (e.g. `holland/`, `nz-4weeks/`) still listed

## 4. Communicate downtime

- [ ] Expect Custom GPT Actions to fail (connection errors) until redeploy
- [ ] Expect old capability links to fail until redeploy + (if host changed) URL rewrite
- [ ] Do **not** rotate tokens just because compute is down

## 5. Next season

Follow [runbook-deploy-compute.md](runbook-deploy-compute.md). After recreate, update GPT Actions server URL if `Endpoint` changed, and share the new host + existing tokens.
