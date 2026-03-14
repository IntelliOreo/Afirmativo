# Afirmativo Codebase Deep-Dive

This document is the committed implementation walkthrough for the current codebase. It is intentionally more detailed and more volatile than the root `README.md`.

## 1. What This Repo Is

Afirmativo is a bilingual AI-assisted practice tool for asylum interview preparation.

Core product constraints:

- no user accounts, only session code + PIN access
- English and Spanish output throughout the flow
- typed and voice answers
- AI evaluation is slow enough that answer processing must be async
- users may close the browser mid-interview, so the flow must recover safely

## 2. High-Level Architecture

```text
+-------------------+      HTTP / JSON      +--------------------+      SQL      +--------------+
| frontend/         | <-------------------> | backend/           | <-----------> | PostgreSQL   |
| Next.js + React   |                       | Go stdlib HTTP     |               |              |
+---------+---------+                       +----------+---------+               +--------------+
          |                                             |
          | voice token minting                         | AI calls
          v                                             v
   +--------------+                             +-------------------+
   | Deepgram     |                             | Claude / Ollama / |
   | voice APIs   |                             | Vertex AI         |
   +--------------+                             +-------------------+
```

At a high level:

- `frontend/` owns the interview UI, client-side state machine, polling, local persistence, and voice UX
- `backend/` owns session auth, interview state, async workers, report generation, voice token minting, payment stubs, and admin cleanup
- `database/` owns schema migration tooling and a local DB studio

## 3. Repo Layout

```text
frontend/
  src/app/                       route-based app pages
  src/app/interview/[code]/      interview UI, reducer, hooks, DTO mapping
  src/lib/                       API client, storage, language helpers

backend/
  cmd/server/                    composition root
  internal/config/               env parsing and validation
  internal/session/              session verification and auth
  internal/interview/            interview workflow, async jobs, AI adapters, stores
  internal/report/               report generation and retrieval
  internal/payment/              stable 501 payment handlers
  internal/voice/                Deepgram token flow
  internal/admin/                gated cleanup tools
  internal/shared/               middleware, auth, rate limiting, responses

database/
  migrations/                    schema history
  main.go                        migration CLI, coupon loader, DB studio

doc/
  commands.md                    developer setup and common commands
  deployment-phase1.md           deployment/runtime contract
```

## 4. Main Runtime Routes

```text
GET  /api/health
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

Current truthful notes:

- payment endpoints are intentionally stable `501` stubs
- report PDF is intentionally `501`
- the browser contract is same-origin `/api/*`, with local proxying handled by the frontend during development

## 5. Frontend Interview Flow

Primary files:

- `frontend/src/app/interview/[code]/page.tsx`
- `frontend/src/app/interview/[code]/hooks/useInterviewMachine.ts`
- `frontend/src/app/interview/[code]/hooks/useAsyncAnswerPolling.ts`

The interview UI is driven by a reducer-based finite state machine.

```text
guard
  -> loading
     -> active
        -> submitting
           -> active
           -> done

loading -> error
submitting -> error
```

Page-level lifecycle:

```text
page load
  -> verify session / boot interview
  -> recover pending answer if localStorage says one exists
  -> show active question
  -> checkpoint pending answer locally before submit
  -> submit async
  -> poll until terminal job state
  -> render next question or done
```

The important browser-side safety properties are:

- reducer stays pure
- side effects live in hooks
- pending answers are persisted before network submission
- boot-time recovery can resume or re-submit safely

## 6. Interview State Machine

Backend interview flow steps:

```text
FlowStepDisclaimer
  -> FlowStepReadiness
  -> FlowStepCriterion
  -> FlowStepDone
```

Area status flow:

```text
pending
  -> in_progress
  -> complete
  -> insufficient
  -> not_assessed

pre_addressed is an overlay flag, not a separate terminal flow
```

Interview lifecycle at a glance:

```text
session verified
  -> start interview
  -> disclaimer question
  -> readiness question
  -> first criterion opening question
  -> criterion loop
     -> follow up in same area
     -> or move to next area
  -> done
  -> report generation
```

Start and resume view:

```text
start request
  -> load session
  -> load flow state
  -> refresh area state
  -> already done?
     -> return done
  -> active question already exists?
     -> resume current question
  -> else
     -> issue disclaimer or readiness opening
```

Primary backend files:

- `backend/internal/interview/service.go`
- `backend/internal/interview/service_start.go`
- `backend/internal/interview/service_turn_disclaimer.go`
- `backend/internal/interview/service_turn_readiness.go`
- `backend/internal/interview/service_turn_criterion.go`

## 7. Async Answer Pipeline

Primary backend files:

- `backend/internal/interview/service_async_api.go`
- `backend/internal/interview/service_async_runtime.go`
- `backend/internal/interview/service_async_processor.go`

The pipeline exists because AI evaluation is slow enough that a normal request-response flow would be fragile.

Full flow:

```text
browser
  -> POST /api/interview/answer-async
  -> backend validates turn and upserts answer job
  -> backend enqueues job id
  -> browser receives queued response
  -> browser polls GET /api/interview/answer-jobs/{jobId}

worker
  -> claim queued job
  -> process interview turn
  -> mark job succeeded / failed / canceled / conflict

browser
  -> sees terminal job state
  -> clears pending local state
  -> renders next question or done state
```

Lane view:

```text
browser                     backend API                 worker
-------                     -----------                 ------
submit answer
  -> POST /answer-async
                            -> upsert answer job
                            -> enqueue job id
  <- queued response
poll job status
                            -> GET /answer-jobs/{id}
                                                      -> claim queued job
                                                      -> process turn
                                                      -> mark terminal status
  <- succeeded / failed / canceled / conflict
render next state
```

Job lifecycle:

```text
queued
  -> running
  -> succeeded

queued
  -> running
  -> failed

queued
  -> running
  -> canceled

queued
  -> running
  -> conflict
```

Recovery loop view:

```text
every recovery tick
  -> find stale running jobs
  -> requeue them
  -> list queued jobs
  -> push ids back into worker channel
```

This is an at-least-once processing model backed by idempotency and turn conflict checks.

## 8. Idempotency And Concurrency

Server-side answer submission uses two guards:

1. `clientRequestId`
   - async job upsert is idempotent for repeated submissions
   - the backend rejects the same idempotency key with different payload
2. `turnID`
   - each issued question has an expected turn id
   - stale or duplicate answers are rejected with turn conflict semantics

Concurrency safety summary:

```text
same request id + same payload
  -> same async job

same request id + different payload
  -> idempotency conflict

old turn id
  -> turn conflict
```

This combination protects both duplicate network submission and stale browser state after reload or parallel tabs.

## 9. Retry And Fallback Strategy

Primary file:

- `backend/internal/interview/service_ai_retry.go`

AI calls use bounded retry with backoff. If retries exhaust, the system prefers a recoverable degraded path over wedging the interview.

Fallback behavior includes:

- fallback criterion evaluation
- fallback per-area question text
- substitution flags propagated up to async job status
- explicit `canceled` job outcome when fallback substitution means the browser should reload

Fallback flow:

```text
AI call
  -> success
     -> use AI response

AI call
  -> retry exhausted
     -> use fallback evaluation or fallback question
     -> mark substituted
     -> surface canceled async outcome when reload is required
```

There is also a criterion mismatch guard:

```text
AI returns evaluation for wrong criterion id
  -> discard that criterion evaluation
  -> replace with fallback evaluation for the current area
  -> mark substituted
```

## 10. Opening Questions vs Follow-Up Questions

This distinction explains why some criterion answers use one AI call and others use two.

Same-area follow-up:

```text
criterion answer
  -> AI call evaluates answer and suggests next question
  -> decision = stay
  -> reuse AI next question
```

Move to next area:

```text
criterion answer
  -> AI call evaluates answer
  -> decision = next
  -> build opening-turn context for the new area
  -> second AI call generates opening question for that new area
```

Latency implication:

```text
stay in same area
  -> 1 AI call
  -> lower latency

move to next area
  -> 2 AI calls
  -> higher latency
```

The shared selection policy now lives in:

- `backend/internal/interview/service_turn_opening.go`

Callers still differ:

- readiness opening loads persisted answers
- criterion next-area opening uses projected answers and projected area state

That is why the current refactor only shares opening-question selection policy, not the entire caller workflow.

## 11. Criterion Turn Flow

Primary files:

- `backend/internal/interview/service_turn_criterion.go`
- `backend/internal/interview/service_turn_criterion_helpers.go`
- `backend/internal/interview/service_turn_policy.go`

Current shape:

```text
load criterion inputs
  -> call AI for evaluation
  -> guard fallback / mismatch
  -> plan stay vs next transition
  -> choose next question
     -> same-area follow-up
     -> or next-area opening question
  -> persist transaction
  -> map answer result
```

Branching view:

```text
criterion answer
  -> evaluateCriterionTurn
  -> planCriterionTransition
     -> action = stay
        -> selectCriterionFollowUpQuestion
        -> ensureCriterionQuestionText
        -> buildIssuedCriterionQuestion
     -> action = next
        -> generateNextAreaOpeningQuestion
        -> ensureCriterionQuestionText
        -> buildIssuedCriterionQuestion
  -> persistCriterionTurn
```

Decision table view:

```text
criterion answer
  -> evaluation says stay
     -> follow-up in same area
     -> usually 1 AI call total

criterion answer
  -> evaluation says next
     -> move to next area
     -> opening question for new area
     -> may use 2 AI calls total

criterion answer
  -> AI retry exhausted or mismatch fallback
     -> substituted path
     -> recoverable degraded behavior
```

This is still the densest backend path, but it is now more explicitly split than earlier revisions.

## 12. Recovery Model

Frontend recovery relies on local storage.

```text
before submit
  -> write pending answer locally

on reload
  -> if job id exists, poll existing job
  -> else resubmit using stored clientRequestId
  -> backend de-duplicates by idempotency key
```

Boot recovery view:

```text
page load
  -> inspect pendingAnswerStore
     -> job id present
        -> poll job
     -> no job id
        -> resubmit pending answer
  -> if recovery resolves
     -> continue interview
  -> else
     -> retry normal boot or show error
```

Backend recovery relies on:

- persisted flow state
- persisted active question
- persisted async job table
- periodic async job recovery

## 13. Dependency Injection And Layering

The backend follows interface-based composition from `backend/cmd/server/main.go`.

Package pattern:

```text
domain.go          types and constants
handler.go         parse / validate / delegate / respond
service*.go        use-case logic
postgres*.go       store implementation
```

The interview package is a same-package mini-module rather than a single giant file. That is a deliberate refactor direction:

- `service.go` stays thin
- turn handlers stay explicit
- helper files carry planning and selection logic
- store files stay separated by persistence concern

## 14. Security Model

```text
JWT cookie auth
  + session-code binding
  + rate limiting
  + failed-attempt lockout
  + CORS allowlist
  + security headers
  + bounded request body sizes
```

The backend also redacts or constrains sensitive logging and expects strong secrets for auth and provider integration.

## 15. Current Caveats

This is the most important truthful snapshot to keep in mind while reading the code:

- async workers, request de-duplication maps, and rate limiting are still process-local
- payment is scaffold/stub level
- PDF report download is not implemented yet
- `internal/interview` is still the main backend complexity hotspot
- observability is still modest compared with a production-hardened platform

## 16. Recommended Reading Order

If you are onboarding into the codebase, this order gives the fastest useful model:

1. `backend/cmd/server/main.go`
2. `backend/internal/config/config.go`
3. `backend/internal/interview/service.go`
4. `backend/internal/interview/service_start.go`
5. `backend/internal/interview/service_turn_readiness.go`
6. `backend/internal/interview/service_turn_criterion.go`
7. `backend/internal/interview/service_async_api.go`
8. `backend/internal/interview/service_async_runtime.go`
9. `backend/internal/interview/service_async_processor.go`
10. `frontend/src/app/interview/[code]/hooks/useInterviewMachine.ts`

## 17. Quick Flow Cheatsheet

Interview lifecycle:

```text
verify session
  -> start interview
  -> disclaimer
  -> readiness
  -> first criterion question
  -> criterion loop
  -> done
  -> generate report
```

Criterion answer lifecycle:

```text
submit answer
  -> queue async job
  -> worker processes turn
  -> 1 AI call if staying in same area
  -> 2 AI calls if moving to a new area
  -> persist next active question or finish interview
  -> browser receives terminal job result
```
