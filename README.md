# Afirmativo

Afirmativo is a bilingual AI-assisted practice tool for asylum interview preparation. The app is session-based, supports typed and voice answers, evaluates interview turns asynchronously, and produces a bilingual report after the interview is complete.

## Architecture Snapshot

```text
+-------------------+      HTTP / JSON      +--------------------+      SQL      +--------------+
| frontend/         | <-------------------> | backend/           | <-----------> | PostgreSQL   |
| Next.js + React   |                       | Go stdlib HTTP     |               | (pgxpool)    |
+---------+---------+                       +----------+---------+               +--------------+
          |                                            |  |
          | voice token minting                        |  | OTel traces + metrics
          v                                            |  v
   +--------------+                             +------+--------+     +-------------------+
   | Deepgram     |                             | Claude/Ollama/ |     | GCP Cloud Trace   |
   | voice APIs   |                             | Vertex AI      |     | + Cloud Monitoring |
   +--------------+                             +---------------+     +-------------------+
```

## Repo Map

```text
frontend/     Next.js app and browser-side interview state machine
backend/      Go API for sessions, interview flow, reports, payments, admin, voice
utils/database/     Migration CLI, coupon loader, and local DB studio
utils/terraform/    optional local checkout of the private infra repo for operators
doc/          Committed developer docs
run-*.sh / rebuild-*.sh     Local container verification helpers
```

## Main Backend Routes

```text
GET  /api/health                            health + pool stats + async queue stats
POST /api/coupon/validate
POST /api/session/verify
GET  /api/session/access
POST /api/interview/start
POST /api/interview/answer-async
GET  /api/interview/answer-jobs/{jobId}
POST /api/voice/token
POST /api/report/{code}/generate
GET  /api/report/{code}
GET  /api/report/{code}/pdf
POST /api/payment/checkout
POST /api/payment/webhook
POST /api/admin/cleanup-db
```

## Interview Flow

```text
start
  -> disclaimer
  -> readiness
  -> criterion turns (loop across evaluation areas)
  -> done
  -> report generation
```

Criterion turns are the core of the product:

- same-area follow-ups usually use one AI call
- moving to a new interview area may use a second AI call to generate the opening question for that area
- answers are submitted asynchronously, so the browser polls job status instead of waiting on one long request

## Async Answer Pipeline

```text
browser submit
  -> POST /api/interview/answer-async
  -> backend upserts async job (idempotent by clientRequestId)
  -> worker claims job
  -> interview service processes turn
  -> job marked succeeded / failed / canceled / conflict
  -> browser polls GET /api/interview/answer-jobs/{jobId}
```

Recovery: stale running jobs are requeued by a periodic recovery loop. Jobs that exceed max retry attempts are marked permanently failed.

## Observability

- **Logging**: structured JSON for GCP Cloud Logging (`LOG_FORMAT=json`) or plain text for local dev
- **Tracing**: OpenTelemetry spans on HTTP requests and AI calls, exported to GCP Cloud Trace when `OTEL_ENABLED=true`
- **Health endpoint**: DB ping, async queue depths, DB connection pool stats (`db_pool_*`)

## Graceful Shutdown

```text
SIGTERM received
  1. stop accepting new HTTP connections, drain in-flight requests (10s)
  2. cancel async runtimes (no new jobs can be enqueued)
  3. wait for in-progress workers to finish current jobs (15s)
  4. close DB pool, flush OTel
```

## Versioning

Both frontend and backend carry their own version, currently kept in sync:

- Backend: `backend/cmd/server/version.go` (compiled into the binary, exposed in `GET /api/health`)
- Frontend: `frontend/package.json` `"version"` field (read at build time via `next.config.ts`)

To bump: edit both files. They can be decoupled later by bumping independently.

## Current Notes

- The backend still assumes a low-traffic single-server shape for async workers, in-memory request de-duplication, and process-local rate limiting.
- Payment endpoints return stable `501 PAYMENT_NOT_IMPLEMENTED` responses.
- `GET /api/report/{code}/pdf` currently returns `501 NOT_IMPLEMENTED`.
- The interview package remains the most complex backend area and is the main focus of the maintainability refactors.

## Docs

- Deep-dive walkthrough: [doc/codebase-deep-dive.md](doc/codebase-deep-dive.md)
- Local setup and commands: [doc/commands.md](doc/commands.md)
- Deployment/runtime baseline: [doc/deployment-phase1.md](doc/deployment-phase1.md)
- Release automation contract: [doc/deployment-phase2-github-actions.md](doc/deployment-phase2-github-actions.md)
- Local container verification runbook: [utils/design/local-container-verification-2026-03-19.md](utils/design/local-container-verification-2026-03-19.md)

## Deploy Status

Phase 1 is complete and remains the truthful runtime baseline:

- browser contract is same-origin `/api/*`
- GCP load balancer routes `/api/*` to backend and everything else to frontend
- manual Terraform deploys use explicit image refs, environment-specific `tfvars`, and Secret Manager for sensitive runtime values
- when `app_domain` is set, the load balancer terminates HTTPS with a Google-managed certificate and redirects HTTP to HTTPS

Phase 2 is implemented as a cross-repo release flow:

- `main` in this public repo builds immutable candidate images in the dev Artifact Registry
- `release-dev-*` and `release-prod-*` in this repo dispatch the private Terraform repo
- the private `afirmativo-tf` repo runs Terraform `plan/apply`, smoke tests, and environment approvals
- backend/runtime secrets stay in GCP Secret Manager; this repo does not read local `.env` files during CI

## Local Workflows

Use two local workflows on purpose:

- Native development:
  - `go run ./cmd/server`
  - `npm run dev`
  - or `./dev-all.sh`
  - fastest iteration path for daily coding
- Local container verification:
  - `./rebuild-local-containers.sh`
  - `./run-backend-container.sh`
  - `./run-frontend-container.sh`
  - deployment-parity path for validating Docker + Cloud Run assumptions

Important:

- frontend code or `next.config.ts` changes require a frontend image rebuild before the container reflects them
- backend code changes require a backend image rebuild before the container reflects them
- runtime-only env changes can often be tested by restarting the container without rebuilding
- the frontend image should not hardcode a Cloud Run port assumption; local container runs should pass `PORT=3000`, while Cloud Run injects its own `PORT` at runtime

## Useful Entry Points

- Frontend interview page: `frontend/src/app/interview/[code]/page.tsx`
- Frontend state machine: `frontend/src/app/interview/[code]/hooks/useInterviewMachine.ts`
- Frontend active screen orchestrator: `frontend/src/app/interview/[code]/components/InterviewActiveScreen.tsx`
- Frontend answer timeout hook: `frontend/src/app/interview/[code]/hooks/useAnswerTimeout.ts`
- Frontend answer draft hook: `frontend/src/app/interview/[code]/hooks/useAnswerDraft.ts`
- Backend composition root: `backend/cmd/server/main.go`
- Backend interview service shell: `backend/internal/interview/service.go`
- Backend async pipeline: `backend/internal/interview/service_async_api.go`, `service_async_runtime.go`, `service_async_processor.go`
