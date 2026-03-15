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
database/     Migration CLI, coupon loader, and local DB studio
doc/          Committed developer docs
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

## Current Notes

- The backend still assumes a low-traffic single-server shape for async workers, in-memory request de-duplication, and process-local rate limiting.
- Payment endpoints return stable `501 PAYMENT_NOT_IMPLEMENTED` responses.
- `GET /api/report/{code}/pdf` currently returns `501 NOT_IMPLEMENTED`.
- The interview package remains the most complex backend area and is the main focus of the maintainability refactors.

## Docs

- Deep-dive walkthrough: [doc/codebase-deep-dive.md](doc/codebase-deep-dive.md)
- Local setup and commands: [doc/commands.md](doc/commands.md)
- Deployment/runtime contract: [doc/deployment-phase1.md](doc/deployment-phase1.md)

## Useful Entry Points

- Frontend interview page: `frontend/src/app/interview/[code]/page.tsx`
- Frontend state machine: `frontend/src/app/interview/[code]/hooks/useInterviewMachine.ts`
- Frontend active screen orchestrator: `frontend/src/app/interview/[code]/components/InterviewActiveScreen.tsx`
- Frontend answer timeout hook: `frontend/src/app/interview/[code]/hooks/useAnswerTimeout.ts`
- Frontend answer draft hook: `frontend/src/app/interview/[code]/hooks/useAnswerDraft.ts`
- Backend composition root: `backend/cmd/server/main.go`
- Backend interview service shell: `backend/internal/interview/service.go`
- Backend async pipeline: `backend/internal/interview/service_async_api.go`, `service_async_runtime.go`, `service_async_processor.go`
