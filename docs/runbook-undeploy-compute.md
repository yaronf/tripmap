# Runbook: undeploy seasonal compute

Tear down `tripmap-compute` to stop EC2/EIP charges off-season. **Do not** delete `tripmap-data`.

**Region:** `eu-central-1`  
**Identity:** `tripmap-deploy` (or `yaron-admin`).

## What stays / what dies

| Kept | Gone until next deploy |
|------|-------------------------|
| `tripmap-data` (YAML, comments, ECR, secrets) | EC2 instance, EIP, SG, AMI-refresh Lambda/rule |
| `tripmap-edge` (CloudFront + ACM) | Working origin behind CF |
| GoDaddy `tripmap` → CloudFront | Live API / viewer / MCP |
| GoDaddy `tripmap-origin` A record (optional) | |

`https://tripmap.sheffer.org` stays in DNS but returns origin errors (e.g. 502) until compute is back.

## 1. Optional: note current EIP

```bash
aws cloudformation describe-stacks \
  --stack-name tripmap-compute --region eu-central-1 \
  --query "Stacks[0].Outputs[?OutputKey=='ElasticIP'].OutputValue" \
  --output text
```

## 2. Delete compute stack (`tripmap-deploy`)

```bash
aws cloudformation delete-stack \
  --stack-name tripmap-compute \
  --region eu-central-1

aws cloudformation wait stack-delete-complete \
  --stack-name tripmap-compute \
  --region eu-central-1
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
- [ ] Trip prefixes still listed

## 4. Communicate downtime

- [ ] Expect API / MCP / viewer to fail until redeploy
- [ ] Do **not** rotate secrets just because compute is down

## 5. Next season

Follow [runbook-deploy-compute.md](runbook-deploy-compute.md). Update GoDaddy `tripmap-origin` if the EIP changed; durable URL stays `https://tripmap.sheffer.org`.
