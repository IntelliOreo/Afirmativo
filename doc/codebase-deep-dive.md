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

At a high level:

- `frontend/` owns the interview UI, client-side state machine, polling, local persistence, and voice UX
- `backend/` owns session auth, interview state, async workers, report generation, voice token minting, payment stubs, and config-gated admin cleanup
- `utils/database/` owns schema migration tooling and a local DB studio

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
  internal/admin/                config-gated cleanup tools
  internal/shared/               middleware, auth, health, rate limiting, OTel, logging

utils/database/
  migrations/                    schema history
  main.go                        migration CLI, coupon loader, DB studio

doc/
  commands.md                    developer setup and common commands
  deployment-phase1.md           deployment/runtime contract
  deployment-phase2-github-actions.md GitHub Actions automation contract
```

## 4. Main Runtime Routes

```text
GET  /api/health                            DB ping + pool stats + async queue stats
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
- `frontend/src/app/interview/[code]/hooks/useAnswerTimeout.ts`
- `frontend/src/app/interview/[code]/hooks/useAnswerDraft.ts`
- `frontend/src/app/interview/[code]/components/InterviewActiveScreen.tsx`

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

### Component and hook ownership

Each concern lives in exactly one place. `page.tsx` owns the reducer and creates typed callbacks; child components and hooks never touch `InterviewAction` directly.

```text
page.tsx
  |
  |  useInterviewMachine        -- reducer, bootstrap, async polling
  |    returns: state, dispatch, requestSubmit
  |
  |  creates typed callbacks:
  |    handleTextChange      --> dispatch(TEXT_CHANGED)
  |    handleInputModeChange --> dispatch(INPUT_MODE_CHANGED)
  |
  +---> InterviewActiveScreen
          |
          |  props: onTextChange, onInputModeChange, requestSubmit
          |  (no dispatch, no InterviewAction import)
          |
          +---> useVoiceRecorder          -- mic lifecycle, recording, transcription
          +---> useMicrophoneDialogState   -- mic opt-in dialog
          +---> useAnswerTimeout           -- timeout state machine (5 effects, 4 refs)
          +---> useAnswerDraft             -- localStorage draft persistence (2 effects, 1 ref)
          |
          +---> InputModeSwitch
          +---> TextAnswerPanel
          +---> VoiceAnswerSection
          +---> TimeoutDialog
          +---> MicrophoneWarmupDialog
```

### Dispatch boundary

`dispatch` is only used inside two files. Everything else receives narrow typed callbacks.

```text
                  +-------------------+
                  |   page.tsx        |
                  |                   |
                  |  dispatch --------+---> handleTextChange(value)
                  |  (InterviewAction)|     handleInputModeChange(mode)
                  +-------------------+
                          |
              +-----------+-----------+
              |                       |
     onTextChange(v)        onInputModeChange(m)
              |                       |
     +--------v-----------+-----------v--------+
     | InterviewActiveScreen                   |
     |   uses onTextChange / onInputModeChange |
     |   passes same to useAnswerDraft         |
     |   NO dispatch, NO InterviewAction       |
     +----+------------------------------------+
          |
     +----v--------------+
     | useAnswerDraft     |
     |   onTextChange     |   (restore saved draft)
     |   onInputModeChange|   (restore voice_review mode)
     +--------------------+
```

### Answer timeout state machine

The timeout flow was the most implicit part of the old code -- 3 effects, 4 refs, and a pure function spread across InterviewActiveScreen. It is now encapsulated in `useAnswerTimeout`.

```text
                        answerSecondsLeft
                              |
              +---------------v----------------+
              |        useAnswerTimeout         |
              |                                 |
              |  tracks: timedOutTurnRef         |
              |          timeoutReviewTurnRef    |
              |          liveExpiredTurnRef      |
              |          previousAnswerWindowRef |
              |                                 |
              |  detects >0 --> <=0 transition  |
              +---+----------+----------+-------+
                  |          |          |
                  v          v          v
           show timeout   auto-stop   auto-review
             dialog       recording   voice audio
                  |                     |
                  v                     v
           handleTimeoutSubmit   onAutoReviewVoice
              |                     |
              v                     v
         requestSubmit        reviewVoiceRecording
                               --> onTextChange
```

### Answer draft persistence

`useAnswerDraft` handles saving and restoring in-progress answers across page reloads.

```text
question mount                         textAnswer changes
     |                                       |
     v                                       v
clearStale(code, turnId)              text empty?
     |                                  +--yes--> clear(code, turnId)
     v                                  +--no---> write(code, draft)
read(code, turnId)
     |
     +-- draft found?
     |     +--yes--> onTextChange(draftText)
     |     |         source === "voice_review"?
     |     |           +--yes--> onInputModeChange("voice")
     |     +--no---> (noop)
```

### Page-level lifecycle

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
- side effects live in dedicated hooks, each owning one concern
- `InterviewAction` never leaks past `page.tsx` -- children use typed callbacks
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
                  +--------+
                  | queued |
                  +---+----+
                      |
                      v
                  +---------+
            +---->| running |<----+
            |     +----+----+     |
            |          |          |
     recovery          |          recovery
     loop requeue      |          loop requeue
     (stale)           |          (stale)
            |     +----+----+----+----+-----+
            |     |         |         |     |
            |     v         v         v     |
       +---------+  +------+  +----------+  |
       |succeeded|  |failed|  | canceled |  |
       +---------+  +------+  +----------+  |
                                            |
                                  +---------+
                                  | conflict|
                                  +---------+
```

## 8. Async Error Propagation And Recovery

This section documents how the async pipeline handles failures at each stage, and how the recovery loop provides at-least-once delivery.

### Session completion failure

When the interview is done and `finishSession` (CompleteSession) fails:

```text
worker processes turn
  -> processTurnCore returns FlowStepDone
  -> finishSession calls CompleteSession
  -> CompleteSession fails (DB error)
  -> processTurnForAsyncJob returns ErrSessionCompleteFailed
  -> processAnswerJob detects ErrSessionCompleteFailed
  -> does NOT mark job as terminal
  -> job stays in "running"
                    |
                    v
         +--------------------+
         | recovery loop tick |
         +--------+-----------+
                  |
                  v
         job is stale running
           -> requeued to "queued"
           -> re-enqueued to worker channel
           -> worker retries from scratch
```

### Max retry cap

To prevent infinite retry when CompleteSession is permanently broken:

```text
worker claims job
  -> check job.Attempts > 5?
     +-- yes --> mark failed with SESSION_COMPLETE_FAILED (permanent)
     +-- no  --> process turn normally
```

### Full error classification flow

```text
processAnswerJob
  |
  +-- claim fails?            --> log, return (job stays queued)
  |
  +-- attempts > 5?           --> mark FAILED: SESSION_COMPLETE_FAILED
  |
  +-- processTurnForAsyncJob
  |     |
  |     +-- ErrSessionCompleteFailed?  --> log, leave running (recovery retries)
  |     +-- ErrTurnConflict?           --> mark CONFLICT
  |     +-- ErrInvalidFlow?            --> mark FAILED: FLOW_INVALID
  |     +-- ErrAIRetryExhausted?       --> mark CANCELED: AI_RETRY_EXHAUSTED
  |     +-- other error?               --> mark FAILED: INTERNAL_ERROR
  |
  +-- answerResult.Substituted?  --> mark CANCELED: AI_RETRY_EXHAUSTED
  |
  +-- encode payload fails?      --> mark FAILED: SERIALIZATION_ERROR
  |
  +-- MarkSucceeded fails?       --> log, leave running (recovery retries)
  |
  +-- success                    --> mark SUCCEEDED
```

### Recovery loop

```text
every recovery tick (default 10s)
  |
  +-- 1. find running jobs older than stale_after (default 180s)
  |      -> UPDATE status = 'queued' WHERE status = 'running' AND started_at < staleBefore
  |      -> these are jobs where the worker crashed or timed out
  |
  +-- 2. list all queued job IDs (batch limit)
  |
  +-- 3. push each ID into the worker channel
  |      -> if channel full, job stays queued for next tick
  |
  +-- worker picks up job -> claim -> process -> terminal
```

## 9. Retry Inventory

```text
+-------------+----------------------------+------+----------------+
| Pipeline    | What retries               | Max  | Backoffs       |
+-------------+----------------------------+------+----------------+
| Interview   | AI GenerateTurn calls      | 3    | 3s, 7s         |
| Interview   | Stale running jobs         | *    | Every 10s tick |
| Interview   | Session completion failure | 5    | Recovery loop  |
| Report      | AI GenerateReport calls    | 3    | 3s, 7s         |
| Report      | Stale running reports      | *    | Every 10s tick |
| Report      | Report generation failure  | 5    | Recovery loop  |
+-------------+----------------------------+------+----------------+

* = unlimited by the recovery loop, but bounded by job timeout + stale threshold
```

What has NO retry and fails permanently:

- `MarkAnswerJobSucceeded` DB failure: job stays running, recovery retries the whole turn
- `MarkReportReady` DB failure: report is marked failed with `PERSIST_READY_FAILED`
- Turn conflict, invalid flow: these are logic errors, not transient

## 10. Idempotency And Concurrency

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

## 11. Retry And Fallback Strategy

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

## 12. Opening Questions vs Follow-Up Questions

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

## 13. Criterion Turn Flow

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

## 14. Recovery Model

Frontend recovery relies on two local storage mechanisms:

- **pendingAnswerStore**: persists in-flight async job state so polling can resume after reload
- **answerDraftStore**: persists the current text/voice draft so in-progress answers survive reload (managed by `useAnswerDraft` hook)

```text
during active question
  -> useAnswerDraft writes draft to answerDraftStore on every change
  -> on reload, useAnswerDraft restores draft text and input mode

before submit
  -> write pending answer to pendingAnswerStore

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
- periodic async job recovery loop (requeues stale running jobs)
- max retry cap (5 attempts before permanent failure)

## 15. Observability Stack

Primary files:

- `backend/internal/shared/otel.go` (OTel initialization)
- `backend/internal/shared/gcplog.go` (GCP-structured JSON logging)
- `backend/internal/shared/middleware.go` (HTTP trace middleware)
- `backend/internal/shared/health.go` (health endpoint with extensible stats)

### Logging

```text
LOG_FORMAT=text (default)     plain slog text output for local dev
LOG_FORMAT=json               GCP Cloud Logging structured JSON
                              severity field mapped from slog level
                              msg renamed to "message" for Cloud Logging
```

### Tracing

```text
OTEL_ENABLED=false (default)  noop tracer and meter, zero overhead
OTEL_ENABLED=true             GCP Cloud Trace exporter + Cloud Monitoring

Spans created:
  afirmativo-http / {method} {path}     per HTTP request (middleware)
  afirmativo-async / async.answer_job   per async answer worker job
  afirmativo-ai / ai.generate_turn      per AI retry call
```

### Health endpoint

```text
GET /api/health returns:
  {
    "status": "ok",
    "version": "0.1.0",
    "db": "connected",
    "async_answer_queue_depth": 0,
    "async_answer_queue_capacity": 256,
    "async_answer_workers": 4,
    "async_report_queue_depth": 0,
    "async_report_queue_capacity": 256,
    "async_report_workers": 4,
    "db_pool_total_conns": 10,
    "db_pool_idle_conns": 8,
    "db_pool_acquired_conns": 2,
    "db_pool_max_conns": 10
  }
```

Stats are provided via the `HealthStatsProvider` interface, making it easy to add new providers.

## 16. Graceful Shutdown

Primary files:

- `backend/cmd/server/main.go` (shutdown sequence)
- `backend/internal/shared/drain.go` (`WaitForWorkers` shared by both pipelines)

The shutdown sequence is ordered to avoid losing in-flight work:

```text
SIGTERM / SIGINT received
  |
  v
+------------------------------------------+
| 1. srv.Shutdown (10s timeout)            |
|    - stop accepting new connections      |
|    - drain in-flight HTTP requests       |
|    - after this, no new async jobs can   |
|      be submitted via HTTP handlers      |
+------------------------------------------+
  |
  v
+------------------------------------------+
| 2. asyncRuntimeCancel()                  |
|    - cancel the context for all workers  |
|    - workers exit their select loop      |
|    - recovery loop stops                 |
+------------------------------------------+
  |
  v
+------------------------------------------+
| 3. WaitForDrain (15s timeout)            |
|    - interview workers: workerWg.Wait()  |
|    - report workers: workerWg.Wait()     |
|    - workers finishing current job will   |
|      complete before exit                |
|    - if timeout, in-progress jobs stay   |
|      in "running" and the recovery loop  |
|      will requeue them on next startup   |
+------------------------------------------+
  |
  v
+------------------------------------------+
| 4. pool.Close() + otelShutdown()         |
|    - close DB connections                |
|    - flush OTel spans and metrics        |
+------------------------------------------+
```

Why this order matters:

```text
BAD (old):  cancel workers -> shutdown HTTP
            workers die -> HTTP still accepts -> new jobs -> nobody processes them

GOOD (new): shutdown HTTP -> cancel workers -> drain workers
            no new jobs arrive -> workers finish current work -> clean exit
```

## 17. Dependency Injection And Layering

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

## 18. Security Model

```text
JWT cookie auth (golang-jwt/v5)
  + session-code binding
  + rate limiting (token bucket per IP, per session)
  + failed-attempt lockout
  + CORS allowlist
  + security headers
  + bounded request body sizes
```

The backend also redacts or constrains sensitive logging and expects strong secrets for auth and provider integration.

## 19. Report Pipeline

Primary files:

- `backend/internal/report/service.go`

The report pipeline mirrors the interview async pipeline:

```text
browser
  -> POST /api/report/{code}/generate
  -> backend creates or re-queues report record
  -> worker claims report
  -> builds area summaries from interview data
  -> AI generates bilingual report (3 attempts, 3s/7s backoffs)
  -> marks report ready
  -> browser polls GET /api/report/{code}
```

Report job lifecycle:

```text
  +--------+
  | queued |
  +---+----+
      |
      v
  +---------+
  | running |<---- recovery loop requeue (stale)
  +----+----+
       |
  +----+----+
  |         |
  v         v
+-------+ +--------+
| ready | | failed |
+-------+ +--------+
```

Error handling:

```text
AI retry exhausted       -> mark FAILED: AI_RETRY_EXHAUSTED
session not found        -> mark FAILED: SESSION_NOT_FOUND
session not completed    -> mark FAILED: NOT_COMPLETED
MarkReportReady DB fail  -> mark FAILED: PERSIST_READY_FAILED
attempts > 5             -> mark FAILED: GENERATION_ERROR (permanent)
context canceled         -> leave running for recovery (capped at 5 retries)
```

## 20. Current Caveats

This is the most important truthful snapshot to keep in mind while reading the code:

- async workers, request de-duplication maps, and rate limiting are still process-local
- payment is scaffold/stub level
- PDF report download is not implemented yet
- `internal/interview` is still the main backend complexity hotspot

## 21. Recommended Reading Order

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
10. `backend/internal/interview/service_finish.go`
11. `backend/internal/report/service.go`
12. `frontend/src/app/interview/[code]/hooks/useInterviewMachine.ts`
13. `frontend/src/app/interview/[code]/components/InterviewActiveScreen.tsx`
14. `frontend/src/app/interview/[code]/hooks/useAnswerTimeout.ts`
15. `frontend/src/app/interview/[code]/hooks/useAnswerDraft.ts`

## 22. Quick Flow Cheatsheet

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

Async error lifecycle:

```text
transient failure (DB, session complete)
  -> job stays running
  -> recovery loop requeues after stale threshold
  -> worker retries
  -> max 5 attempts before permanent failure

permanent failure (turn conflict, invalid flow)
  -> job marked terminal immediately
  -> no retry
```
