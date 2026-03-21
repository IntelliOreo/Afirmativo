# Phase 1 Deployment Contract

Status: completed baseline. This document is the runtime/deploy truth that Phase 2 automation builds on top of.

This document defines the truthful runtime contract for the first honest deployment shape:

```text
browser
  -> same-origin /api/*

local
  -> frontend runtime proxies /api/* to backend

gcp
  -> load balancer routes /api/* to backend
  -> load balancer routes everything else to frontend
```

## Why the browser contract changes first

The browser cannot call a private Cloud Run backend directly.

That means production cannot be:

```text
browser -> public backend URL
```

The browser-facing contract must become:

```text
browser -> /api/*
```

Once that is true:
- local development can simulate ingress with a frontend-side proxy
- production can replace that proxy with a load balancer
- app code does not need to change again when moving from local to GCP

## Local routing contract

Local development and local container validation use:

- `API_PROXY_TARGET=http://localhost:8080` for `next dev`
- `API_PROXY_TARGET=http://backend:8080` or similar for container-to-container tests

The browser should never need to know that backend origin directly.
In GCP, do not set `API_PROXY_TARGET`; the load balancer should route `/api/*`.

## Backend runtime contract

`backend/internal/config/config.go` is the source of truth.

Terraform now injects the deploy-time runtime contract through Cloud Run env vars and Secret Manager references. `terraform output` is not enough to verify loaded env; use Cloud Run `Variables & Secrets` or `gcloud run services describe ... --format=export` when you need the actual applied runtime shape.

### Required secrets and startup-critical values

- `DATABASE_URL`
- `JWT_SECRET`
- `AI_AREA_CONFIG`
- `AI_API_KEY` when `AI_PROVIDER=claude`
- `VOICE_AI_API_KEY`
- `VERTEX_AI_API_KEY` when `AI_PROVIDER=vertex` and `VERTEX_AI_AUTH_MODE=api_key`

### Prompt and content env vars commonly set explicitly in production

- `AI_INTERVIEW_SYSTEM_PROMPT`
- `AI_REPORT_PROMPT`
- `AI_INTERVIEW_PROMPT_LAST_QUESTION`
- `AI_INTERVIEW_PROMPT_CLOSING`
- `AI_INTERVIEW_PROMPT_OPENING_TURN`

### Plain env vars set explicitly in Terraform for the current deploy shape

- `FRONTEND_URL`
  - must match the public app origin used by the browser
  - used by backend CORS and auth-cookie behavior
- `AI_PROVIDER`
- `AI_MODEL`
- `AI_MAX_TOKENS`
- `AI_INTERVIEW_LAST_QUESTION_SECONDS`
- `AI_INTERVIEW_CLOSING_SECONDS`
- `AI_INTERVIEW_MIDPOINT_AREA_INDEX`
- `AI_INTERVIEW_PROMPT_CACHING_ENABLED`
- `AI_REPORT_MAX_TOKENS`
- `AI_TIMEOUT_SECONDS`
- `LOG_LEVEL`
- `LOG_FORMAT`
- `SESSION_EXPIRY_HOURS`
- `SESSION_AUTH_MAX_TTL_MINUTES`
- `SESSION_AUTH_COOKIE_NAME`
- `INTERVIEW_BUDGET_SECONDS`
- `ANSWER_TIME_LIMIT_SECONDS`
- `INTERVIEW_OPENING_DISCLAIMER_EN`
- `INTERVIEW_OPENING_DISCLAIMER_ES`
- `INTERVIEW_READINESS_QUESTION_EN`
- `INTERVIEW_READINESS_QUESTION_ES`
- async answer worker/runtime settings
- verification rate-limit settings
- voice token rate-limit settings

### Provider-specific plain env vars

- `AI_PROVIDER=ollama`
  - `OLLAMA_BASE_URL`
- `AI_PROVIDER=vertex`
  - `VERTEX_AI_AUTH_MODE`
  - `VERTEX_AI_PROJECT_ID`
  - optional `VERTEX_AI_LOCATION`
- OTel runtime
  - `OTEL_ENABLED`
  - `GCP_PROJECT_ID` when `OTEL_ENABLED=true`
- Voice runtime
  - `VOICE_AI_BASE_URL`
  - `VOICE_AI_MODEL`
  - `VOICE_AI_TOKEN_TIMEOUT_SECONDS`

### Vars that still intentionally rely on code defaults unless we explicitly need an override

- `PORT`
- HTTP timeout settings
- async report worker settings
- `ALLOW_SENSITIVE_DEBUG_LOGS`
- `ADMIN_CLEANUP_ENABLED`
- `OTEL_ENABLED`

### Local-dev-only or convenience vars

- `.env` file loading
- `MOCK_API_URL`
- `AFIRMATIVO_TEST_DATABASE_URL`

These must not be relied on as part of the production runtime contract.

## Migration contract

The backend does **not** run migrations on startup.

Production deploy order must be:

```text
apply schema changes
  ->
deploy backend
  ->
deploy frontend / routing
```

This keeps service startup simple and avoids hidden app-start side effects.

Terraform workspace discipline must also stay aligned with the var file:

```text
default workspace -> dev.tfvars
prod workspace    -> prod.tfvars
```

Current truthful Terraform target for both environments:

- `ai_provider = "vertex"`
- `vertex_ai_auth_mode = "adc"`
- `vertex_ai_project_id = <environment project_id>`
- `vertex_ai_location = "global"`

These are plain deploy-time env vars, not secrets.

## Current Phase 1 ingress and domain shape

Phase 1 is no longer only a raw-IP HTTP bootstrap.

- `app_domain` can be set in Terraform per environment
- when `app_domain` is set:
  - the external HTTP load balancer gets a Google-managed SSL certificate
  - port `443` serves the real app URL map
  - port `80` redirects to HTTPS
  - backend `FRONTEND_URL` must match the HTTPS public origin

For the current dev environment, the intended public entrypoint is:

```text
https://asilo-afirmativo.com
```

Certificate issuance depends on DNS pointing the domain at the load balancer IP and may take additional time after Terraform apply.

## Container expectations

### Backend image

- binds to `PORT`
- expects runtime env injection
- exposes `/api/health`
- must not contain `.env`

### Frontend image

- runs in Next standalone mode
- serves app pages
- supports same-origin `/api` contract
- may proxy `/api` locally when `API_PROXY_TARGET` is set
- should not require a public backend URL at build time
- should honor runtime `PORT` from the environment rather than baking a Cloud Run-specific port assumption into the image

Frontend env is intentionally split:

- build-time public config:
  - `NEXT_PUBLIC_*`
- runtime server-side local proxy config:
  - `API_PROXY_TARGET`

That means:

- changing frontend code or `next.config.ts` requires rebuilding the frontend image
- changing `NEXT_PUBLIC_*` values requires rebuilding the frontend image
- changing only `API_PROXY_TARGET` only requires restarting the frontend container
- changing the local container port should be done at `docker run` time via `PORT=...`, not by rebuilding the image

## Production ingress expectation

The first honest GCP topology is:

```text
public entrypoint
  -> frontend service for /
  -> backend service for /api/*
```

The backend remains non-public by ingress.
The load balancer becomes the browser-facing routing layer.

GitHub Actions automation now uses this contract through a cross-repo release flow:

- this public app repo builds candidate images on `main`
- this public app repo dispatches deploys from `release-dev-*` and `release-prod-*`
- the private Terraform repo runs `terraform plan/apply`, smoke tests, and environment approvals

See [doc/deployment-phase2-github-actions.md](deployment-phase2-github-actions.md).
