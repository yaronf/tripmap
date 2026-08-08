# Runbook: undeploy seasonal compute

Tear down `tripmap-compute` to stop ALB/Fargate charges off-season. **Do not** delete `tripmap-data`.

**Region:** `eu-central-1`  
**Identity:** `tripmap-deploy` or `yaron-admin`.

## What stays / what dies

| Kept | Gone until next deploy |
|------|-------------------------|
| `tripmap-data` (YAML, comments, ECR, secret) | Express / ALB / Fargate |
| `tripmap-edge` (CloudFront + ACM) | Working origin behind CF |
| GoDaddy `tripmap` → CloudFront | Live API / viewer / MCP |
| Capability token **hashes** | |

`https://tripmap.sheffer.org` stays in DNS but returns origin errors (e.g. 502) until compute is back and CloudFront origin is updated (see deploy runbook). Tokens unchanged; paths `/t/{id}/{token}/` work again on the same durable host.

## 1. Optional: note current endpoint

```bash
aws cloudformation describe-stacks \
  --stack-name tripmap-compute --region eu-central-1 \
  --query "Stacks[0].Outputs[?OutputKey=='Endpoint'].OutputValue" \
  --output text
```

Useful so you know which Express origin will change on the next deploy.

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

- [ ] Expect agent API / MCP and Hellō viewer to fail (origin errors) until redeploy
- [ ] Do **not** rotate tokens just because compute is down

## 5. Next season

Follow [runbook-deploy-compute.md](runbook-deploy-compute.md) end-to-end, including **step 2b** (Express bake settings). Seasonal delete+create restores AWS canary defaults (~9 min deploys) until that step runs again. Update CloudFront origin if `Endpoint` changed; durable URL stays `https://tripmap.sheffer.org`.
