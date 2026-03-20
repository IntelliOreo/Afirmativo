# Phase 1 Deployment Contract

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

### Plain env vars to set explicitly in production

- `FRONTEND_URL`
  - must match the public app origin used by the browser
  - used by backend CORS and auth-cookie behavior
- `AI_PROVIDER`
- `AI_MODEL`
- `LOG_LEVEL`
- `LOG_FORMAT`

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

### Optional vars with safe defaults

These already have defaults in code and do not need to be set unless production needs non-default behavior:

- `PORT`
- HTTP timeout settings
- async worker settings
- rate limit settings
- interview timing settings
- `VOICE_AI_TOKEN_TIMEOUT_SECONDS`
- `ALLOW_SENSITIVE_DEBUG_LOGS`
- `ADMIN_CLEANUP_ENABLED`

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

## Production ingress expectation

The first honest GCP topology is:

```text
public entrypoint
  -> frontend service for /
  -> backend service for /api/*
```

The backend remains non-public by ingress.
The load balancer becomes the browser-facing routing layer.
