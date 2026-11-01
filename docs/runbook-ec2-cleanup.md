# Runbook: post-cutover EC2 cleanup (Phase D)

Do this **after** EC2 compute has soaked healthy and Express is no longer needed for rollback.

## D1 — retire Express (`tripmap-deploy`)

1. If an old Express `tripmap-compute` still exists (migration left it), delete that stack. The EC2 stack uses the same name once Express is gone—only one `tripmap-compute` should remain.
2. Confirm billing: no ALB/Fargate/Express charges.
3. Repo docs should already describe EC2 (this tree). Grep for leftover Express bake/`ecs describe-services` instructions and remove them if any remain.

## D2 — strip durable ECS IAM (`yaron-admin`)

`tripmap-deploy` cannot update `tripmap-data` or `tripmap-deploy-iam`.

### `infra/data.yaml`

Remove these resources and their exports (keep buckets, secrets, ECR, OpenAI param):

- `TaskExecutionRole` + export `TaskExecutionRoleArn`
- `TaskRole` + export `TaskRoleArn`
- `InfrastructureRole` + export `InfrastructureRoleArn`

Update the stack description so it no longer mentions ECS compute roles.

```bash
# as yaron-admin
aws cloudformation deploy \
  --stack-name tripmap-data \
  --template-file infra/data.yaml \
  --region eu-central-1 \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides ProjectName=tripmap
```

### `infra/deploy-iam.yaml`

Remove statements:

- `PassTripmapECSRoles`
- `ECSExpressGateway`

Keep EC2/SSM/Lambda/Events/`PassTripmapEC2Roles`/`ManageTripmapEC2Roles`, ECR, CFN compute/edge, CloudFront, S3.

```bash
aws cloudformation deploy \
  --stack-name tripmap-deploy-iam \
  --template-file infra/deploy-iam.yaml \
  --region eu-central-1 \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides ProjectName=tripmap DeployUserName=tripmap-deploy
```

## D3 — confirm (`tripmap-deploy`)

```bash
./scripts/deploy-compute.sh --skip-build --tag "$(cat /tmp/tripmap-deploy-tag.txt)"
curl -fsS https://tripmap.sheffer.org/health
```

Optional: seasonal delete + recreate of `tripmap-compute` against the slimmed data exports.
