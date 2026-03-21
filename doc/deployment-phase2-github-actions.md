# Phase 2 GitHub Actions Release Automation

Status: implemented release model.

Phase 2 no longer means "run Terraform from the public app repo." The actual release flow is now split across two repositories with different trust boundaries.

```text
public app repo
  -> build candidate images on main
  -> dispatch release requests from release-dev-* / release-prod-*

private infra repo
  -> authenticate to GCP via WIF
  -> promote prod images when needed
  -> terraform plan/apply
  -> smoke test the public app URL
```

## Why the split exists

- the app repo is public and should not hold broad deploy credentials
- the infra repo is private and is the correct place for Terraform state, deploy approvals, and GCP auth
- backend/runtime secrets remain in GCP Secret Manager; GitHub Actions should not read local `.env` files during deploys

## Branch Contract

### Public app repo

- `main`
  - builds immutable candidate images
  - pushes:
    - `backend:${sha}`
    - `frontend:${sha}-devcfg`
    - `frontend:${sha}-prodcfg`
- `release-dev-*`
  - dispatches the private infra repo to deploy dev using the exact candidate image refs for that commit
- `release-prod-*`
  - dispatches the private infra repo to deploy prod using the exact candidate image refs for that commit

### Private infra repo

- the deploy workflow runs from the infra repo's trusted branch, typically `main`
- app repo release branches do **not** need to exist in the infra repo
- WIF should trust the private infra repo and its deploy branch, not the public app repo release branches

## WIF

WIF means `Workload Identity Federation`.

Instead of storing a long-lived JSON key in GitHub:

```text
GitHub Actions
  -> presents short-lived OIDC identity
  -> GCP verifies repo/ref claims
  -> workflow impersonates a service account briefly
```

This is safer because:

- there is no static service-account key to leak or rotate
- repo/ref restrictions can be enforced in GCP IAM
- build and deploy roles can be separated cleanly

## Identity Split

### Public app repo identity

- purpose: build and push candidate images only
- access:
  - Artifact Registry write in the dev project
- no Terraform access
- no Cloud Run deploy access

### Private infra repo identity

- purpose: deploy only
- access:
  - Terraform remote state backend
  - Artifact Registry read/write as needed for promotion
  - Cloud Run deploy
  - `iam.serviceAccountUser` on runtime service accounts

## Secret and Config Contract

### What stays out of GitHub

- `DATABASE_URL`
- `JWT_SECRET`
- AI prompt/config secrets
- provider API keys
- other backend/runtime secrets

These stay in GCP Secret Manager and Terraform continues wiring Cloud Run to those secret names.

### What belongs in GitHub

- WIF provider IDs
- deploy/build service-account emails
- Terraform backend bucket/prefix
- cross-repo dispatch token

### Frontend build-time values

`NEXT_PUBLIC_*` values are public once shipped to the browser.

For the current build model they should be supplied through GitHub variables, because:

- the frontend consumes them at build time
- `main` builds both dev and prod frontend variants in one workflow
- the workflow should not read local `.env.local`

## Smoke Tests

Smoke tests are thin post-deploy checks that answer: "did the deployment actually come up in a basically healthy state?"

Minimum smoke checks:

- public app URL responds
- `/api/health` responds through the public entrypoint
- backend latest created revision is also latest ready revision
- frontend latest created revision is also latest ready revision

Why they matter:

- Terraform can succeed while the app is still broken
- bad frontend build-time config can produce a green infra deploy and a broken site
- secret wiring mistakes can produce a green apply and a crashing backend revision
- load balancer or routing regressions can leave `/api/health` broken even though both services exist

Without smoke tests, the pipeline can report success while production is already unhealthy.

## GCP Verification Points

### After WIF and service-account setup

Use:

- `IAM & Admin > Workload Identity Federation`
- `IAM & Admin > Service Accounts`

Verify:

- separate providers exist for app-build and infra-deploy
- app-build SA only has registry write
- infra-deploy SA has deploy/state access

### After remote state migration

Use:

- `Cloud Storage > Buckets`

Verify:

- state objects exist for the chosen backend bucket/prefix
- CI `terraform init` is reading shared remote state instead of local files

### After `main` candidate builds

Use:

- `Artifact Registry > Repositories > asilo-images`

Verify:

- backend `${sha}`
- frontend `${sha}-devcfg`
- frontend `${sha}-prodcfg`

### After `release-dev-*`

Use:

- `Cloud Run > Services`
- `Cloud Logging > Logs Explorer`

Verify:

- dev frontend/backend revisions use expected image refs
- new revisions are ready
- no obvious startup failures
- public app URL and `/api/health` respond

### After `release-prod-*`

Use:

- `Cloud Run > Services`
- `Artifact Registry > Repositories > asilo-images`
- `Cloud Logging > Logs Explorer`

Verify:

- prod deploy used the promoted image refs
- promoted prod images exist in prod Artifact Registry
- public app URL and `/api/health` respond

## GitHub Actions Minutes and Pricing

The split affects cost in a useful way:

- public app repo:
  - heavy Docker build minutes happen here
  - standard GitHub-hosted runners for public repositories are free
- private infra repo:
  - billed private minutes accrue here
  - keep these jobs thin: promotion, Terraform, smoke tests

Monthly cost estimate:

```text
private billed minutes
  ~= (release-dev job minutes * number of dev releases)
   + (release-prod job minutes * number of prod releases)
```

Current GitHub-hosted private-minute quotas to track in runbooks:

- GitHub Free for organizations: `2,000`
- GitHub Team: `3,000`
- GitHub Enterprise Cloud: `50,000`

## Completion Checklist

- `main` builds and publishes candidate images
- release branches dispatch the private infra repo
- private infra repo uses WIF and remote GCS state
- prod promotion uses exact candidate digests
- Terraform always receives explicit image refs
- smoke tests run after apply
- docs and README reflect the cross-repo release model
