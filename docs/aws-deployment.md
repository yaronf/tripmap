# AWS deployment plan

Authoritative plan for hosting tripmap **beyond** the current GitHub Pages static PWA.  
Companion: [itinerary-display-viewer.md](itinerary-display-viewer.md) (product/architecture), [itinerary-display-ux.md](itinerary-display-ux.md) (UI).

**Status:** Phase A–C live; durable URL + Custom GPT on `https://tripmap.sheffer.org`; OpenAPI 0.2.2.  
**Current production (static):** GitHub Pages (`www.sheffer.org/tripmap/`).  
**In-season compute:** ECS Express Mode behind CloudFront (`tripmap-edge` → `tripmap-compute`).

---

## Locked decisions

| Topic | Decision |
|-------|----------|
| Edge CDN | **CloudFront** for durable `tripmap.sheffer.org` only (`tripmap-edge`); **no WAF** in v1 |
| S3 encryption | **SSE-S3** (default); no SSE-KMS |
| Compute | **ECS Express Mode** (Fargate + managed ALB) — not App Runner (sunset / closed to many new accounts) |
| Scale to zero | **No** on Express Mode — instead **seasonal one-click deploy / undeploy** via CloudFormation |
| Infra as code | **CloudFormation** (two stacks). Agent maintains templates in-repo; you click Create/Delete stack (or one `aws cloudformation` command) |
| Viewer access | **Capability URL** (unguessable token in path) |
| Comments | Shared read/write for anyone with the URL; offline = read cache only |
| Custom tripmap MCP | **No** |
| Cursor | Repo + git; optional AWS MCP; optional same OpenAPI |
| ChatGPT | **Custom GPT + Actions (OpenAPI)**; Bearer encrypted in GPT editor |
| Agent API | OpenAPI on the container (`/openapi.yaml` + `/api/agent/*`) |
| Patch retries | **`Idempotency-Key` required** on mutating agent calls |
| Delete trip | **Omit** initially |
| Source of truth (live) | **S3** (persists across undeploys) |
| Schema evolution | **`schema_version`** in YAML |
| GitHub YAML | Cursor-maintained mirror under `itineraries/` |
| Region | **All tripmap resources in `eu-central-1` (Frankfurt)**. CLI default may stay `il-central-1`; always `--region eu-central-1` for tripmap |
| Public hostname | **`https://tripmap.sheffer.org`** via CloudFront (`tripmap-edge`) → Express origin. Express `Endpoint` may change; update CloudFront origin only (runbook). Static Pages remains `www.sheffer.org/tripmap/`. |

---

## Goals

| Requirement | Approach |
|-------------|----------|
| Run application code on AWS | **ECS Express Mode** when “in season” |
| Idle for months | **Delete compute stack** — keep data stack (~cents/mo) |
| One-click up/down | CloudFormation: `tripmap-data` (keep) + `tripmap-compute` (create/delete) |
| Canonical YAML | Versioned S3 |
| Comments | Unversioned S3 |
| ChatGPT | Actions → OpenAPI Bearer |
| Cursor / GH | Local YAML + git; publish/pull via OpenAPI |
| Viewer + comments | `/t/{id}/{token}/` on `https://tripmap.sheffer.org` |
| Offline comments | Cached read-only |

### Non-goals (this plan)

- Custom MCP server
- CloudFront / WAF
- Terraform/CDK (CloudFormation only for v1)
- Always-on compute year-round
- App Runner as primary (deprecated path)
- Multi-tenant SaaS

---

## Seasonal deploy / undeploy

```text
Off-season (most of the year)
  ✅ tripmap-data stack: S3 ×2, Secrets, IAM roles, ECR (+ image)
  ❌ tripmap-compute stack: deleted → no ALB, no Fargate ≈ no ~$25/mo

Trip season (or a test weekend)
  ✅ Create tripmap-compute → Express Mode service from ECR image
  ✅ Note new HTTPS base URL → update Custom GPT Actions + partner links
  ✅ Smoke /health + one capability URL
```

| Action | How (one-click) |
|--------|------------------|
| **Deploy compute** | Console → CloudFormation → Create stack → upload `infra/compute.yaml` (or `aws cloudformation deploy --stack-name tripmap-compute ...`) |
| **Undeploy compute** | Console → stack `tripmap-compute` → **Delete** (or `aws cloudformation delete-stack`) |
| **Never delete** | Stack `tripmap-data` (unless retiring the project) |

Capability **tokens** stay in S3 meta across seasons. Only the **hostname** may change after a fresh compute stack — path `/t/{id}/{token}/` stays valid on the new host once bundles are served again (regenerate on first boot or via agent API).

---

## CloudFormation layout

Repo path (to add): `infra/`

| Template | Stack name | Lifecycle |
|----------|------------|-----------|
| `infra/data.yaml` | `tripmap-data` | **Persistent** — create once |
| `infra/compute.yaml` | `tripmap-compute` | **Ephemeral** — create/delete seasonally |

### `tripmap-data` (keep)

- S3 itineraries (versioning ON, Block Public Access)
- S3 comments (versioning OFF)
- Secrets Manager `tripmap/agent-bearer` (generate or pass as parameter — prefer generate-once and leave)
- Secrets Manager `tripmap/hello-session` (HMAC for Hellō session cookies; separate from the agent Bearer)
- ECR repository `tripmapd`
- IAM task/execution roles used by compute (or export role ARNs via stack outputs)
- Outputs: bucket names, secret ARN, ECR URI, role ARNs

### `tripmap-compute` (deploy/undeploy)

- ECS Express Mode service (or equivalent CFN resources Express Mode documents) pointing at ECR image tag (e.g. `latest` or immutable digest parameter)
- Env: bucket names, secret ARN, `AWS_REGION`, `PUBLIC_BASE_URL` (from stack output after create — or set in a second update), `OSRM_BASE_URL`, `MAX_YAML_BYTES`
- Health check `/health`
- Outputs: **`ServiceUrl`** (HTTPS base) — copy into GPT Actions + password manager

**Parameter:** `ImageTag` so redeploy can pin a digest without rebuilding the template.

Optional later: GitHub Actions `workflow_dispatch` jobs `deploy-compute` / `destroy-compute` that call CloudFormation — still one click from the Actions tab.

---

## Target architecture

```mermaid
flowchart TB
  subgraph season [In season — tripmap-compute]
    ecs[ECS Express Mode<br/>tripmapd]
  end

  subgraph always [Always — tripmap-data]
    s3y[(S3 itineraries)]
    s3c[(S3 comments)]
    sm[Secrets Manager]
    ecr[ECR image]
  end

  partner[Partner] -->|capability URL| ecs
  gpt[ChatGPT Actions] -->|Bearer OpenAPI| ecs
  cursor[Cursor] -->|git / OpenAPI / AWS MCP| s3y
  ecs --> s3y
  ecs --> s3c
  ecs --> sm
  ecs --> ecr
```

When compute is deleted, GPT Actions and capability URLs are simply **offline** until the next deploy (expected off-season).

---

## Components

| Component | Stack | Responsibility |
|-----------|-------|----------------|
| **tripmapd container** | compute | Viewer, comments, OpenAPI, bundle regenerate |
| **S3 itineraries / comments** | data | Live data |
| **ECR** | data | Image kept between seasons |
| **Secrets Manager** | data | Agent Bearer + Hellō session HMAC |
| **Task role** | data (or compute) | S3 + secret access |
| **AWS MCP (optional)** | — | Cursor → S3 under your IAM |

### Request surface (when compute is up)

| Surface | Auth |
|---------|------|
| `GET /t/{id}/{token}/…` + comments API | Capability URL |
| `GET /health` | Public |
| `GET /openapi.yaml` | Public (spec) |
| `/api/agent/*` | Bearer |

---

## Data model

Unchanged in spirit:

- Versioned S3 YAML + `schema_version` + hashed capability token in `*.meta.json`
- Unversioned comments
- Write-through bundles when compute is running

Off-season: YAML/comments remain; bundles may be absent until next deploy regenerates them.

### Agent API (v1)

| Operation | Effect |
|-----------|--------|
| `GET/PUT .../trips/{id}/yaml` | Full YAML (**Idempotency-Key** on PUT) |
| `PATCH .../trips/{id}` | Structured patch |
| `POST .../trips` | Create + viewer URL |
| `GET .../schema` | Schema + version |
| `POST .../rotate-token` / `restore` | Token / S3 version ops |

---

## AuthN / AuthZ

- Capability URL for viewers/comments (shared edit).
- Agent Bearer in Secrets Manager + Custom GPT Actions.
- Cursor: git; publish via OpenAPI when compute is up.

### IAM (task role)

```text
s3: List/Get/Put itineraries; GetObjectVersion + ListBucketVersions
s3: List/Get/Put/Delete comments
secretsmanager:GetSecretValue on agent Bearer ARN
```

---

## Security (short)

Same threat model as before; add: **undeployed compute** means agent API is unreachable (good). Don’t leave an old ALB/DNS pointing at nothing without updating GPT — runbook step after delete.

---

## PWA

Capability-URL based; offline comment reads from cache; no queued writes. After redeploy with new host, users need the **new** full URL once (or custom domain later).

---

## Manual config work (agent-assisted)

Prefer **CloudFormation** over click-ops for buckets/roles/compute. You still approve stack creates in console (or run CLI). Agent authors/updates `infra/*.yaml` and provides exact commands.

### M1 — Account hygiene

- [x] Budget configured
- [x] Region: work in **eu-central-1** for all tripmap stacks
- [ ] Optional: CLI default `il-central-1`; alias or always pass `--region eu-central-1`
- [x] Skip App Runner gate — compute is Express Mode
- [x] Daily work as `yaron-admin` (not root)
- [x] Day-to-day Cursor deploys as `tripmap-deploy` (see `infra/deploy-iam.yaml`) — not `AdministratorAccess`

### Deploy IAM (`tripmap-deploy`)

Least-privilege user for ECR push, `tripmap-compute` create/update/delete, and S3 inspect/seed. **Cannot** delete `tripmap-data`, read `tripmap/agent-bearer` or `tripmap/hello-session`, or administer the account.

Stack `tripmap-deploy-iam` owns the **managed policy** + **user**. Group `tripmap-deploy` is CLI-managed (avoids CFN AlreadyExists): attach policy `tripmap-deploy` and AWS managed **`SignInLocalDevelopmentAccess`**, add the user as a member.

```bash
aws cloudformation deploy --stack-name tripmap-deploy-iam \
  --template-file infra/deploy-iam.yaml --region eu-central-1 \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides ProjectName=tripmap DeployUserName=tripmap-deploy
```

Enable **console access + MFA** on user `tripmap-deploy`. Use `aws login` as that user for tripmap work.

**Agent Bearer** stays out of IAM deploy rights: store in a password manager, put in a gitignored local `.env` for smoke/CLI, and paste into Custom GPT Actions (encrypted in the GPT editor). Capability viewer URLs are fine in Cursor chat.

### M2 — Data stack (once)

- [ ] Review `infra/data.yaml` (agent writes it)
- [ ] Create stack `tripmap-data` in eu-central-1
- [ ] Save stack **Outputs** (buckets, secret ARN, ECR)
- [ ] Confirm secret value in password manager; never commit

### M3 — First image

- [ ] `docker build` / `docker push` to ECR from Outputs
- [ ] Agent: exact commands with account/region/repo

### M4 — Compute stack (seasonal)

- [x] Create stack `tripmap-compute` with ImageTag + data stack exports
- [x] Durable public URL `https://tripmap.sheffer.org` (`tripmap-edge` + GoDaddy CNAME)
- [x] `curl https://tripmap.sheffer.org/health`
- [x] **Undeploy drill:** delete `tripmap-compute`; confirm data stack intact; recreate; update CloudFront origin

Agent API smoke (after image push). Load Bearer from a **local** `.env` (never commit; never fetch via `tripmap-deploy`):

```bash
# .env (gitignored) — set once from password manager / Secrets Manager as yaron-admin
# AGENT_BEARER_TOKEN=…

set -a && source .env && set +a
BASE_URL="https://tripmap.sheffer.org" TOKEN="$AGENT_BEARER_TOKEN" ./scripts/smoke-agent.sh
```
### M5 — Seed itineraries

- [x] Upload YAML + meta (tokens); regenerate via agent API once compute is up
- [x] Save capability URLs (host + token) — password manager / private note  
  Prefer `https://tripmap.sheffer.org/t/{id}/{token}/` (rewrite old `*.on.aws` hosts).

### M6 — Custom GPT

- [x] Actions → Import from URL (`https://tripmap.sheffer.org/openapi.yaml`) + Bearer
- [x] Durable host — no GPT server URL change on compute recreate (only CloudFront origin; see [runbook-deploy-compute.md](runbook-deploy-compute.md))
- [x] Agent: GPT instruction blurb + test prompts

#### Setup (ChatGPT → Create a GPT)

1. Confirm edge is up: `curl -fsS https://tripmap.sheffer.org/health`
2. **Actions** → **Import from URL**: `https://tripmap.sheffer.org/openapi.yaml`  
   (OpenAPI **3.1.0**, inline parameters, `servers[0].url` = durable host. Fallback: paste via `python3 scripts/export_openapi.py tripmap.sheffer.org`.)
3. **Authentication**: API Key → Auth Type **Bearer** → paste agent Bearer from password manager / `.env` (`AGENT_BEARER_TOKEN`). Never put this in the GPT instructions text. Recover from `.env` or Secrets Manager as `yaron-admin` if the GPT editor clears it.
4. **Instructions** (paste):

```text
You edit tripmap road-trip itineraries via the tripmap agent API Actions.

Rules:
- Always list or GET before changing a trip. Prefer GET /api/agent/trips/{id}/yaml before PUT/PATCH.
- Mutating calls require a unique Idempotency-Key header (use a new UUID each distinct user request; reuse only when retrying the same failed call).
- schema_version must be 1. Do not invent unsupported schema versions.
- Prefer PATCH for small edits (swap_days, days.{n}.title/notes/hike/ferry/route/stops, insert_day, delete_day). Use PUT /yaml only for full document replacement.
- After create/rotate-token, tell the user the viewer_url (capability link). Do not invent tokens.
- Trips holland and nz-4weeks are the main itineraries. Do not delete trips (API has no delete).
- If the API returns 401/404/5xx, report the error briefly; do not invent itinerary data.
- Keep answers short; show day titles and key fields when summarizing.
```

5. **Test prompts**: “List trips.” → “What’s on day 4 of holland?” → “Rename day 4 title to … (PATCH).”
6. Save the GPT. After compute recreate, re-point CloudFront origin (runbook); GPT Actions URL stays `https://tripmap.sheffer.org`.

### M7 — Cursor

- [ ] Rule/skill: publish/pull OpenAPI; seasonal “compute is down” awareness
- [ ] Optional AWS MCP for S3 inspection

### M8 — Cutover / Pages

- [ ] Share capability URLs when in season
- [x] README: note seasonal hosting + link to runbook
- [x] GitHub Pages: stub only (`pages-stub/`); no public trip bundles (live = `tripmap.sheffer.org`)

### M9 — Runbooks (in-repo)

- [x] [`docs/runbook-deploy-compute.md`](runbook-deploy-compute.md) — create/update + GPT URL checklist
- [x] [`docs/runbook-undeploy-compute.md`](runbook-undeploy-compute.md) — delete stack + “links offline until next season”

---

## Implementation phases

### Phase A — App + data template

- [x] `cmd/tripmapd` skeleton: health, Bearer middleware, static stub
- [x] `infra/data.yaml` + create `tripmap-data`
- [x] Push first ECR image

### Phase B — Compute template + OpenAPI

- [x] `infra/compute.yaml` (Express Mode)
- [x] OpenAPI agent API + schema_version + bundles
- [x] Deploy/undeploy drill (M4)
- [x] Seed + Custom GPT (M5–M6)

### Phase C — Comments + PWA

- [x] Comments under capability URL; offline read-only

Capability URL format: `https://tripmap.sheffer.org/t/{id}/{token}/` (token plaintext only on create/rotate). Shared notes: `GET|PUT …/api/notes`.

### Phase D — Ergonomics

- [x] Runbooks M9; optional `workflow_dispatch` deploy/destroy still open
- [ ] Cursor skill
- [x] Durable public URL: `tripmap.sheffer.org` + `infra/edge.yaml` (CloudFront)

### Phase E — Hardening

- [ ] Token/Bearer rotation; restore version; Budget still green off-season

---

## Configuration (runtime, compute)

| Name | Source |
|------|--------|
| `AGENT_BEARER_TOKEN` | Secrets Manager (data stack) |
| `HELLO_SESSION_SECRET` | Secrets Manager `tripmap/hello-session` (data stack) |
| Hellō allowlist | `config/hello-allowlist.csv` in the image (`HELLO_ALLOWLIST_FILE`) |
| `ITINERARIES_BUCKET` / `COMMENTS_BUCKET` | Data stack outputs |
| `AWS_REGION` | `eu-central-1` |
| `PUBLIC_BASE_URL` | Import from `tripmap-edge` → `https://tripmap.sheffer.org` |
| `OSRM_BASE_URL` / `MAX_YAML_BYTES` | Template parameters / defaults |

---

## Cost sketch

| State | Order of magnitude |
|-------|--------------------|
| **Off-season** (data only) | Cents–low $/mo |
| **In season** (Express Mode) | ~$25–40/mo while stack exists |
| CloudFormation | Free |

---

## Relation to existing roadmap

Live data in S3; ChatGPT via Actions; Cursor via git/OpenAPI. Compute is **ECS Express Mode**, stood up with **CloudFormation one-click** only when needed — avoids deprecated App Runner and avoids paying ALB/Fargate all year.

---

## Acceptance criteria

- [x] `tripmap-data` survives compute delete; YAML/comments intact
- [x] One-click (console or single CLI) create/delete of `tripmap-compute`
- [x] In season: capability URL + shared comments + GPT Actions on `tripmap.sheffer.org`
- [ ] Off season: no ALB/Fargate charges; GPT/links intentionally dead until redeploy
- [x] After redeploy: runbook updates CloudFront origin (GPT URL unchanged)
- [x] No custom MCP; no secrets in git; Budget on
- [ ] Manual checklist M1–M9 done once with agent help (M7 Cursor skill still open)
