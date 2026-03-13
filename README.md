# Afirmativo — Codebase Deep-Dive 

## Context

This document is a comprehensive walkthrough of the Afirmativo codebase, written from the perspective of a staff+ engineer onboarding a less experienced colleague. It covers architecture, patterns, state transitions, idempotency guarantees, retry/fallback strategies, production readiness, and known weaknesses.

---

## 1. What Is This Thing?

Afirmativo is a **bilingual (English/Spanish) AI-powered practice tool for asylum interview preparation**. A user gets a session code + PIN, goes through a simulated asylum interview with AI-generated questions, and receives a bilingual assessment report at the end.

Key constraints that shaped the architecture:
- **No user accounts** — completely anonymous, session-based
- **Voice + text input** — users can speak or type answers
- **AI evaluation is slow** — so everything is async
- **Unreliable AI providers** — so everything has retries and fallbacks
- **Users may close their browser mid-interview** — so everything is recoverable

---

## 2. High-Level Architecture

```
+-------------------+     HTTP/JSON      +--------------------+     SQL      +--------------+
|  Next.js 16       | <----------------> |  Go stdlib HTTP    | <---------> | PostgreSQL   |
|  React 19         |                    |  Service Layer     |             |              |
|  (frontend/)      |                    |  (backend/)        |             |              |
+--------+----------+                    +----------+---------+             +--------------+
         |                                          |
         | WebSocket/REST                           | HTTP
         v                                          v
   +----------+                             +---------------+
   | Deepgram |                             | Claude/Ollama |
   | (voice)  |                             | /Vertex AI    |
   +----------+                             +---------------+
```

- **Frontend**: Next.js 16, React 19, TypeScript, Tailwind. Deployed as static export.
- **Backend**: Go 1.26, stdlib `net/http` (no framework), `pgx/v5` for Postgres. Clean layered architecture.
- **Database**: PostgreSQL with 28 migration files managed by a custom Go CLI tool.

---

## 3. Project Structure

### Frontend (`frontend/src/`)

```
frontend/src/
  app/
    layout.tsx                         -- Root layout
    page.tsx                           -- Landing/home page (session code entry)
    admin/                             -- Admin cleanup tools
    pay/                               -- Payment page (Stripe / coupon)
    beforeYouStart/                    -- Consent/disclaimer page
    interview/[code]/                  -- Main interview page
      page.tsx                         -- Interview container
      components/                      -- Interview UI components
      hooks/                           -- Custom hooks (state machine, voice, polling)
        useInterviewMachine.ts         -- THE core state machine
        useAsyncAnswerPolling.ts       -- Async submission + polling logic
        useVoiceRecorder.ts            -- Microphone management
      dto.ts                           -- Data transfer objects
      mappers.ts                       -- Response mappers
      models.ts                        -- Interview data models (brand types)
      constants.ts                     -- App constants (timeouts, backoffs)
      utils.ts                         -- Utility functions
      viewTypes.ts                     -- UI type definitions
    session/[code]/                    -- Session status page
  lib/
    api.ts                             -- HTTP client with logging
    sessionService.ts                  -- Session verification logic
    language.ts                        -- Language utilities
    storage/
      answerDraftStore.ts              -- Draft text persistence (localStorage)
      pendingAnswerStore.ts            -- Pending job persistence (localStorage)
      languageStore.ts                 -- Language preference
      sessionPinStore.ts              -- Session PIN
  components/                          -- Reusable UI components (NavHeader, Footer, Button, etc.)
  content/                             -- Static content (disclaimers)
```

### Backend (`backend/`)

```
backend/
  cmd/server/
    main.go                            -- Entry point, dependency wiring (composition root)
    report_adapters.go                 -- Data adapters for report service
  internal/
    config/config.go                   -- Config struct loaded from env vars
    session/
      domain.go                        -- Session, Coupon types
      service.go                       -- Session business logic
      handler.go                       -- HTTP handlers
      postgres.go                      -- PostgreSQL store implementation
    interview/
      domain.go                        -- Area status, flow steps, question types
      service.go                       -- Interview logic + turn processing
      service_async.go                 -- Async worker pool + recovery loop
      service_ai_retry.go             -- AI retry with backoff
      handler.go                       -- HTTP handlers
      ai.go                           -- AI client abstraction
      aiclient_vertex.go              -- Vertex AI implementation
      postgres*.go                     -- Database operations
    report/
      domain.go / service.go / handler.go / postgres.go
    payment/
      domain.go / service.go / handler.go / stripe.go / postgres.go
    voice/
      handler.go / client.go           -- Deepgram voice token minting
    admin/
      service.go / handler.go / postgres.go
    shared/
      auth.go                          -- JWT session authentication
      middleware.go                    -- CORS, security headers, logging
      ratelimit.go                     -- Token bucket rate limiters
      lockout.go                       -- Failed attempt lockout
      response.go                      -- HTTP response helpers
    sqlgen/                            -- Generated sqlc types
  sql/
    queries/                           -- SQL query files (session, interview, report, payment)
    schema.sql                         -- Schema dump
```

### Database (`utils/database/`)

```
database/
  migrations/                          -- 28 SQL migration files (up/down)
  main.go                              -- CLI tool (up, down, version, studio, load_coupon)
  reset.sh                             -- Helper to reset DB
```

---

## 4. API Routes

```
GET  /api/health                           -- Health check with DB ping
POST /api/coupon/validate                  -- Validate coupon code
POST /api/session/verify                   -- Verify session code + PIN (rate limited)
GET  /api/session/access                   -- Check session auth status
POST /api/interview/start                  -- Begin interview, get first question
POST /api/interview/answer-async           -- Submit answer, trigger async evaluation
GET  /api/interview/answer-jobs/{jobId}    -- Poll async answer job status
POST /api/voice/token                      -- Mint Deepgram voice token (rate limited)
POST /api/report/{code}/generate           -- Queue report generation
GET  /api/report/{code}                    -- Fetch generated report (JSON)
GET  /api/report/{code}/pdf                -- Fetch report as PDF (501 - not yet wired)
POST /api/payment/checkout                 -- Stripe checkout session (501 - not yet wired)
POST /api/payment/webhook                  -- Stripe webhook handler (501 - not yet wired)
POST /api/admin/cleanup-db                 -- Admin DB cleanup (gated by config)
```

---

## 5. The Interview State Machine (Frontend)

**File**: `frontend/src/app/interview/[code]/hooks/useInterviewMachine.ts`

**Pattern**: `useReducer`-based finite state machine (not XState -- simpler, no external dependency).

### Phases & Transitions

```
          +----------+
          |  guard   |  (unauthenticated)
          +----+-----+
               | START_REQUESTED
               v
          +----------+
          | loading  |  (fetching interview + attempting recovery)
          +----+-----+
               | START_SUCCEEDED / START_COMPLETED / START_UNAUTHORIZED
               v
    +----------+     +----------+     +----------+
    |  active  | --> |submitting| --> |  active  |  (next question)
    +----------+     +----------+     +----------+
         |                |                |
         v                v                v
    +----------+     +----------+     +----------+
    |   done   |     |  error   |     |   done   |
    +----------+     +----------+     +----------+
```

### Key Design Decisions

1. **The reducer is pure** -- all side effects happen in `useEffect` hooks that react to state changes.
2. `submitting` state carries `requestKind: "submit" | "recovery"` so effects know whether this is a fresh answer or a recovery from localStorage.
3. The `TICK` action decrements both `secondsLeft` (overall interview timer) and `answerSecondsLeft` (per-question window) -- the timer continues ticking even during submission.
4. Phase guards prevent impossible transitions: `if (state.phase !== "active") return state`. This is the safety net against race conditions in the UI.

### Action Types (exhaustive)

| Action | From Phase | To Phase | Purpose |
|--------|-----------|----------|---------|
| `START_REQUESTED` | guard | loading | Begin boot sequence |
| `START_UNAUTHORIZED` | loading | guard | Session not authenticated |
| `START_COMPLETED` | loading | done | Interview was already finished |
| `BOOT_RECOVERY_COMPLETED` | loading | done | Recovered pending answer, interview now done |
| `START_SUCCEEDED` | loading | active | First question ready |
| `START_FAILED` | loading | error | Boot failed |
| `TEXT_CHANGED` | active | active | User typing |
| `INPUT_MODE_CHANGED` | active | active | Switch text/voice |
| `TICK` | active/submitting | same | Decrement timers |
| `SUBMIT_REQUESTED` | active | submitting | User submitted answer |
| `RECOVERY_REQUESTED` | active | submitting | Recovering a localStorage pending answer |
| `SUBMIT_SUCCEEDED` | submitting | active/done | Answer processed, next question or done |
| `RECOVERY_SUCCEEDED` | submitting | active/done | Recovered answer processed |
| `SUBMIT_FAILED` | submitting | error | Submission failed |
| `RECOVERY_FAILED` | submitting | error | Recovery failed |

---

## 6. The Async Answer Pipeline

The core architectural challenge: AI evaluation takes 5-30 seconds. You cannot block the HTTP request.

### Frontend Side

**File**: `frontend/src/app/interview/[code]/hooks/useAsyncAnswerPolling.ts`

**Flow**:
1. User submits answer -> `POST /api/interview/answer-async` with a `clientRequestId`
2. Server returns `{ jobId, status: "queued" }` immediately
3. Frontend polls `GET /api/interview/answer-jobs/{jobId}` with backoff
4. When status is `succeeded`, extract the next question and continue

**Polling strategy** -- exponential backoff with jitter:
```
ASYNC_POLL_BACKOFF_MS = [1000, 2000, 3000, 5000, 8000, 15000, 20000, 30000]
withJitter(ms) = Math.max(250, ms * (0.85 + Math.random() * 0.3))
```

**Circuit breaker**: After `ASYNC_POLL_CIRCUIT_BREAKER_FAILURES` (3) consecutive transient failures (5xx, 429, TypeError/network), it waits 15s then throws `ASYNC_POLL_CIRCUIT_OPEN`. This prevents hammering a dying server.

**Timeout**: `ASYNC_POLL_TIMEOUT_MS` (180s) hard cap on polling. If the job hasn't completed, throws `ASYNC_POLL_TIMEOUT`.

### Backend Side

**File**: `backend/internal/interview/service_async.go`

**Pattern**: Bounded worker pool + periodic recovery loop.

```
                    +----------------+
 HTTP Handler ----> | async_answer_  | ----> Worker 1 --> processAnswerJob()
                    | jobs table     | ----> Worker 2 --> processAnswerJob()
                    | (PostgreSQL)   | ----> Worker 3 --> processAnswerJob()
                    |                | ----> Worker 4 --> processAnswerJob()
                    +-------+--------+
                            |
              Recovery Loop | (every 10s)
                            |
                    Requeue stale "running" jobs
                    + Pick up orphaned "queued" jobs
```

**Worker lifecycle**:
1. `ClaimQueuedAnswerJob()` -- atomic claim via UPDATE ... WHERE status = 'queued' (prevents double-processing)
2. `processTurnCore()` -- the actual AI call + state mutation
3. `MarkAnswerJobSucceeded()` or `MarkAnswerJobFailed()` -- terminal state

**Recovery loop** (`recoverAsyncAnswerJobs`): Every 10s, it:
1. Requeues any jobs stuck in `running` for > 3min (the worker probably crashed)
2. Lists all `queued` jobs and pushes them into the in-memory channel
3. If the channel is full, jobs remain in `queued` in the DB and will be picked up next cycle

This is a simple but effective **at-least-once processing guarantee**.

### Job Status Lifecycle

```
queued --> running --> succeeded
                  --> failed
                  --> canceled (AI retries exhausted with fallback applied)
                  --> conflict (turn was stale)
```

---

## 7. Idempotency Guarantees

### Client-Side Idempotency

**File**: `frontend/src/app/interview/[code]/models.ts`

**Brand types** for type safety -- you cannot accidentally pass a `JobId` where a `ClientRequestId` is expected:
```typescript
export type ClientRequestId = Brand<string, "ClientRequestId">;
export type JobId = Brand<string, "JobId">;
export type TurnId = Brand<string, "TurnId">;
```

Each answer submission generates a `clientRequestId` via `crypto.randomUUID()` (with `Date.now()-Math.random()` fallback). This ID is stored in localStorage alongside the pending answer.

**Deduplication guard in the state machine** (useInterviewMachine.ts:402-403):
```typescript
const submissionKey = `${submissionRequestKind}:${submissionPendingJob.clientRequestId}`;
if (activeSubmissionKeyRef.current === submissionKey) return; // Skip duplicate effect
```

### Server-Side Idempotency

**File**: `backend/internal/interview/service_async.go:204-259`

`SubmitAnswerAsync()` uses `UpsertAnswerJob()` -- an `INSERT ... ON CONFLICT (client_request_id) DO NOTHING` pattern. If the same `clientRequestId` comes in again:
1. It returns the existing job (idempotent response)
2. It **validates** the payload matches -- if the same key arrives with different `turnId`, `answerText`, or `questionText`, it returns `ErrIdempotencyConflict`

This is textbook idempotency: same key + same payload = same result, same key + different payload = rejection.

### Turn-Based Optimistic Concurrency

The `turnID` acts as an optimistic concurrency control token. Each question issued gets a random 32-hex turn ID (via `crypto/rand`). When an answer comes in:
```go
if turnID != flowState.ExpectedTurnID {
    return nil, ErrTurnConflict
}
```
This prevents stale/duplicate answers from corrupting the interview flow. The flow state is only advanced when the turn ID matches.

---

## 8. Retry & Fallback Strategy

### Backend AI Retry

**File**: `backend/internal/interview/service_ai_retry.go`

**Strategy**: 3 attempts total, with fixed backoffs of [3s, 7s]:

```go
aiRetryBackoffs = []time.Duration{3 * time.Second, 7 * time.Second}
```

Each failure is recorded to the job's `failed_reasons` column (append-only text field) for post-mortem debugging. Context cancellation is respected between retries:
```go
select {
case <-ctx.Done():
    return nil, fmt.Errorf("AI retry aborted: %w", ctx.Err())
case <-timer.C:
}
```

If all retries fail, `ErrAIRetryExhausted` is returned.

### Fallback Substitution

When AI retries exhaust, the system **does not crash** -- it substitutes:
- **Fallback evaluation**: marks criterion as `partial` with `CriterionRecFollowUp`
- **Fallback question**: uses a pre-configured generic question per area (e.g., "Please tell me about {area}")
- The `Substituted` flag propagates up to the async job handler
- The job is marked `canceled` with code `AI_RETRY_EXHAUSTED`

The interview **never gets stuck** on an AI failure -- it degrades gracefully. The frontend detects `AI_RETRY_EXHAUSTED` and prompts the user to reload.

### Criterion Mismatch Guard

After AI returns, the system validates that the evaluation refers to the correct criterion:
```go
if aiResult.Evaluation.CurrentCriterion.ID != areaCfg.ID {
    aiResult.Evaluation = s.fallbackEvaluation(areaCfg.ID)
    substituted = true
}
```
This prevents AI hallucinations from corrupting the wrong area's status.

### Frontend Circuit Breakers

**Polling circuit breaker**: After 3 consecutive transient failures on job status polling, the frontend stops polling and shows a "Reload to retry" message rather than continuing to hammer a failing server.

**Network error handling**: `TypeError` (network-level failure) is treated as transient and triggers backoff. Non-transient errors (4xx except 429) are terminal.

---

## 9. Backend Interview Flow (Server-Side State Transitions)

**File**: `backend/internal/interview/service.go`

### Flow Step State Machine

```
FlowStepDisclaimer --> FlowStepReadiness --> FlowStepCriterion --> FlowStepDone
```

Each step has its own processing path in `processTurnCore()`:

| Step | What Happens | Next Step |
|------|-------------|-----------|
| `Disclaimer` | Record acknowledgment, issue readiness question | `Readiness` |
| `Readiness` | Call AI for first criterion question, record readiness answer | `Criterion` |
| `Criterion` | Call AI to evaluate answer, decide follow-up or next area | `Criterion` or `Done` |
| `Done` | Mark session complete | Terminal |

### Area Status Lifecycle

```
pending --> in_progress --> complete
                       --> insufficient
                       --> not_assessed (timeout)
                ^
          pre_addressed (flagged by AI from another area's answer)
```

### Criterion Turn Decision Logic

The `DecideCriterionTurn()` function determines what happens after each criterion answer:
- **sufficient** -> mark area complete, move to next area
- **partial + follow-ups remaining** -> ask follow-up in same area
- **insufficient or max questions reached** -> mark accordingly, move on

### Cross-Criteria Intelligence

When evaluating an answer, the AI can flag that the user also addressed criteria from other areas. These are stored as `pre_addressed` so the system can:
- Skip or shorten those areas later
- Provide the AI with context that some ground is already covered

### Timeout Handling

```go
if timeRemainingS <= 0 {
    return s.finishOnTimeout(ctx, sessionCode, areas)
}
```

On timeout:
1. All unresolved areas marked `not_assessed`
2. Flow marked `done`
3. Session marked complete

### Expired Active Turn

On page reload, if the current question's answer window has expired:
```go
if currentFlow.ActiveQuestion.BufferExpired(s.nowFn()) {
    s.processTurn(ctx, sessionCode, "", questionText, currentFlow.ExpectedTurnID)
    continue // re-check state and issue next question
}
```
An empty answer is submitted automatically, and the flow advances.

---

## 10. Recovery From Browser Crashes

### localStorage Persistence

**Files**: `frontend/src/lib/storage/pendingAnswerStore.ts`, `answerDraftStore.ts`

Before any API call, the pending answer is written to localStorage:
```typescript
pendingAnswerStore.write(code, submissionPendingJob); // line 407 of useInterviewMachine.ts
```

### Boot Recovery Flow (useInterviewMachine.ts:248-374)

```
Page Load
  |
  +-- Check localStorage for pending answer
  |    |
  |    +-- Has jobId? --> Poll directly (job was already submitted to server)
  |    |
  |    +-- No jobId? --> POST /api/interview/answer-async (resubmit)
  |         +-- Server deduplicates via clientRequestId
  |
  +-- Recovery succeeded --> dispatch START_SUCCEEDED or BOOT_RECOVERY_COMPLETED
  |
  +-- Recovery failed with retryable code --> try normal /interview/start
  |
  +-- Recovery failed fatally --> show error with retry button
```

The `bootAttempt` counter drives manual retry: incrementing it re-triggers the entire boot effect. The `canRetryPendingRecovery` flag (line 462-465) only shows the retry button for specific error codes.

### Cancellation Safety

Every async effect uses the `canceled` flag pattern:
```typescript
let canceled = false;
void (async () => { ... if (canceled) return; ... })();
return () => { canceled = true; };
```

The voice recorder uses a **generation-based** guard -- incrementing `generationRef` invalidates all in-flight deferred work from previous recording attempts.

---

## 11. Dependency Injection & Layering

### Backend Pattern

**Interface-based DI**, composed in `backend/cmd/server/main.go`:

```go
type SessionStarter interface {
    StartSession(ctx context.Context, sessionCode, preferredLanguage string) (*session.Session, error)
}

type InterviewAIClient interface {
    GenerateTurn(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error)
}
```

Each domain package follows the same structure:
- `domain.go` -- types and constants
- `service.go` -- business logic
- `handler.go` -- HTTP handlers (thin: parse/validate/delegate/respond)
- `postgres.go` -- database implementation of store interfaces

This means you can test the service layer with mock stores, and the handlers with mock services.

### Frontend Pattern

Hooks compose hooks. The dependency chain:
```
page.tsx
  +-- useInterviewMachine (state machine)
  |     +-- useAsyncAnswerPolling (submission + polling)
  |     +-- pendingAnswerStore / answerDraftStore (localStorage)
  +-- useVoiceRecorder (mic management)
  |     +-- useVoiceTicker (timing)
  |     +-- useVoicePreview (playback)
  +-- useInterviewReport (report polling)
```

---

## 12. Security Model

| Layer | Mechanism |
|-------|-----------|
| Auth | JWT in HttpOnly/Secure/SameSite=Lax cookie |
| Session binding | `RequireSessionCodeMatch()` middleware |
| Brute force | Token-bucket rate limiter + failed-attempt lockout |
| CORS | Single-origin allow |
| Headers | `X-Frame-Options: DENY`, `nosniff`, HSTS |
| Input validation | JSON body size limits (64KB general, 10KB answers) |
| Secrets | `JWT_SECRET` >=32 chars, sensitive fields redacted from logs |
| Session codes | `AP-XXXX-XXXX` with curated alphabet (crypto/rand) |
| PINs | 6-digit, hashed before storage |

---

## Current state
**design wwith single-server assumption** 
**Payment is stubbed** 
**PDF generation is client-side only** 

---

The Full Answer → Next Question Pipeline

FRONTEND (browser)                          BACKEND (Go server)                         BACKGROUND
═══════════════════                         ═══════════════════                         ══════════

1. User clicks submit
       │
       ▼
2. dispatch(SUBMIT_REQUESTED)               
   state: active → submitting               
       │                                    
       ▼                                    
3. pendingAnswerStore.write()  ◄── localStorage checkpoint (crash safety)
       │
       ▼
4. POST /api/interview/answer-async  ──────► 5. SubmitAnswerAsync()
   { session_code, answer_text,                    │
     question_text, turn_id,                       ▼
     client_request_id }                     6. UpsertAnswerJob()  ◄── INSERT ... ON CONFLICT
                                                   │                    (idempotency via client_request_id)
                                                   ▼
                                             7. Validate payload matches idempotency key
                                                   │
                                                   ▼
                                             8. enqueueAsyncAnswerJob()  ──────► channel <- jobID
                                                   │                                  │
                                                   ▼                                  │
   ◄─────────────────────────────────────── 9. Return { jobId, status: "queued" }     │
       │                                                                              │
       ▼                                                           ┌──────────────────┘
10. Save jobId to pendingAnswerStore                               ▼
       │                                                    11. Worker picks up jobID
       ▼                                                           │
11. Start polling loop                                             ▼
       │                                                    12. ClaimQueuedAnswerJob()
       │                                                        UPDATE SET status='running'
       │                                                        WHERE status='queued'  ◄── atomic claim
       │                                                           │
       │                                                           ▼
       │                                                    13. processTurnCore()
       │                                                           │
       │                                                           ├── GetSessionByCode()     ◄── sequential
       │                                                           ├── GetFlowState()          ◄── sequential
       │                                                           ├── refreshAreaState()      ◄── sequential
       │                                                           ├── Check turnID match      ◄── sequential
       │                                                           ├── Check time remaining    ◄── sequential
       │                                                           │
       │                                                           ▼
       │                                                    14. callAIWithRetry()
       │                                                        ┌─ Attempt 1: GenerateTurn() ──► Claude/Vertex
       │                                                        │  (fail?) wait 3s
       │                                                        ├─ Attempt 2: GenerateTurn() ──► Claude/Vertex  
       │                                                        │  (fail?) wait 7s
       │                                                        └─ Attempt 3: GenerateTurn() ──► Claude/Vertex
       │                                                           │
       │                                                           ▼  (or fallback if all fail)
       │                                                    15. DecideCriterionTurn()
       │                                                        - sufficient → mark complete, find next area
       │                                                        - partial → follow-up in same area
       │                                                        - insufficient → mark, move on
       │                                                           │
       │                                                           ▼
       │                                                    16. [If moving to next area]
       │                                                        callAIWithRetry() AGAIN  ◄── 2nd AI call!
       │                                                        to generate the opening question
       │                                                        for the NEW area
       │                                                           │
       │                                                           ▼
       │                                                    17. ProcessCriterionTurn()  ◄── single DB transaction
       │                                                        - Save answer
       │                                                        - Update area status
       │                                                        - Store next question as ActiveQuestion
       │                                                        - Advance flow state
       │                                                           │
       │                                                           ▼
       │                                                    18. MarkAnswerJobSucceeded()
       │                                                        - Serialize next question to JSON payload
       │                                                        - UPDATE SET status='succeeded', result_payload=...
       │                                                           │
       ▼                                                           │
12. GET /api/interview/answer-jobs/{jobId} ──► GetAnswerJobResult()
    attempt 0: wait ~1s                        status still "running" → return {status: "running"}
       │
       ▼
    attempt 1: wait ~2s  ──────────────────► GetAnswerJobResult()
                                              status = "succeeded" → deserialize payload
                                              return { done, nextQuestion, timerRemainingS, ... }
       │
       ▼
13. pendingAnswerStore.clear()
       │
       ▼
14. dispatch(SUBMIT_SUCCEEDED)
    state: submitting → active (with new question)
       │
       ▼
15. User sees next question

## What's Async vs Sequential
Async (concurrent/non-blocking)
What	Why async
Worker processing vs frontend polling	The worker processes the job in a background goroutine while the frontend polls independently. They don't know about each other.
AI call	Takes 5-30s. This is THE reason the whole pipeline is async.
Recovery loop (every 10s)	Runs independently of both HTTP handlers and workers. Catches orphaned jobs.
The 2nd AI call (next area opening question)	Happens inside the same worker, but it's a second round-trip to the AI provider — this is why some polls take longer than others.
Sequential (must happen in order)
Inside processTurnCore(), everything is strictly sequential:


GetSession → GetFlowState → refreshAreaState → validate turnID
    → AI call #1 (evaluate answer)
        → DecideCriterionTurn (pure logic)
            → [conditional] AI call #2 (next area opening question)
                → ProcessCriterionTurn (DB transaction)
                    → MarkAnswerJobSucceeded
There is no parallelism inside the worker. This is deliberate — each step depends on the output of the previous step. The AI evaluation determines the decision, the decision determines whether a 2nd AI call is needed, and the DB transaction must include everything.

## The Two AI Calls — This Is the Key Insight
When the AI decides to stay in the same area (follow-up question), there is one AI call. The nextQuestion text comes directly from the evaluation response.

When the AI decides to move to a new area, there are two AI calls in sequence:

AI Call #1 — evaluate the current answer, get decision + evaluation
AI Call #2 — generateNextAreaOpeningQuestion() at service.go:690-751 — generate an opening question for the next area, with full context of all previous answers
This means area transitions take roughly 2x longer than follow-ups. The frontend polling backoff accounts for this — it starts at 1s intervals and grows to 30s.

## The Firing Order Summary

FIRE 1 (sync):   HTTP handler → UpsertAnswerJob → enqueue to channel → return jobId
FIRE 2 (async):  Worker claims job → AI call(s) → DB update → mark succeeded
FIRE 3 (async):  Frontend polls until succeeded → dispatch next question
Fire 1 returns in <100ms. Fire 2 takes 5-60s depending on AI latency and whether it's 1 or 2 AI calls. Fire 3 adds 1-5s of polling overhead on top of Fire 2.

so we made 2 ai call per answer, and separate asking for the answer evaluation or new question generation? can you point out where they are?
Not exactly 2 AI calls per answer. It depends on what happens after evaluation. Let me point you to the exact locations.

## When There's 1 AI Call (follow-up in same area)
This is the common case. At service.go:514 inside the FlowStepCriterion branch:


aiResult, err := s.callAIWithRetry(ctx, turnCtx, failureRecorder)
This single call returns both the evaluation AND the next question in one response. The AIResponse struct contains:

aiResult.Evaluation — how the answer scored (sufficient/partial/insufficient)
aiResult.NextQuestion — the follow-up question text
If DecideCriterionTurn() says "stay in this area" (CriterionTurnActionContinue), the aiResult.NextQuestion from this first call is used directly. One AI call total.

## When There's 2 AI Calls (moving to a new area)
When the decision is CriterionTurnActionNext (move to next area), a second AI call fires at service.go:572:


nextQuestionText, nextAreaSubstituted, err = s.generateNextAreaOpeningQuestion(
    ctx, sessionCode, nextArea, projectedAreas, projectedAnswers,
    sess, preferredLanguage, timeRemainingS, failureRecorder,
)
This calls into service.go:690-751, which builds a fresh AITurnContext with IsOpeningTurn: true for the new area and calls:


nextAreaAIResult, err := s.callAIWithRetry(ctx, openingTurnCtx, failureRecorder)
The reason: the first AI call evaluated the answer in the context of the current area. But the opening question for the next area needs different context — different description, different sufficiency requirements, different status. So it's a separate prompt.

## The FlowStepReadiness case also makes 1 AI call
At service.go:416:


aiResult, err := s.callAIWithRetry(ctx, turnCtx, failureRecorder)
This generates the first criterion question after the readiness step. No evaluation here — it's just question generation with IsOpeningTurn: true.

## Summary
Scenario	AI Calls	Where
Readiness → first criterion question	1	service.go:416
Criterion answer, stay in same area	1	service.go:514
Criterion answer, move to next area	2	service.go:514 + service.go:736
Disclaimer acknowledgment	0	Pure state transition, no AI
So it's 1 AI call most of the time, and 2 only on area transitions. The first call always does evaluation + suggested next question. The second call (when needed) generates a context-appropriate opening question for the new area, discarding the next-question from the first call.

===

Path A: Follow-Up in Same Area (1 AI Call)
This happens when the AI says "partial" or "insufficient but follow-ups remain."


processTurnCore() — FlowStepCriterion branch
=============================================

 1. GetSessionByCode()                          ← DB read
 2. Check session not expired
 3. GetFlowState()                               ← DB read
 4. refreshAreaState()                           ← DB read
 5. Validate turnID == flowState.ExpectedTurnID
 6. Check timeRemainingS > 0
 7. GetAnswersBySession()                        ← DB read (history for AI context)
 8. Build AITurnContext with IsOpeningTurn=false

 9. ┌─────────────────────────────────────────┐
    │  AI CALL #1: callAIWithRetry()          │  ← EVALUATE answer + suggest next question
    │  Returns: Evaluation + NextQuestion     │
    └─────────────────────────────────────────┘

10. DecideCriterionTurn(evaluation, questionsCount, maxPerArea)
    Result: { Action: "continue", MarkCurrentAs: "" }
    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
    "continue" = stay in this area, ask follow-up

11. DetermineNextAreaAfterCriterionTurn()
    Result: nextArea = currentArea (same area)

12. Use aiResult.NextQuestion as the follow-up text  ← from AI Call #1
13. Generate new turnID (crypto/rand)
14. Build NextIssuedQuestion

15. ProcessCriterionTurn()                       ← DB TRANSACTION
    - Save answer to answers table
    - Update area status
    - Store NextIssuedQuestion as ActiveQuestion
    - Advance ExpectedTurnID
    - Increment QuestionNumber

16. Return AnswerResult{
      Done: false,
      NextQuestion: follow-up question (same area),
      Substituted: false,
    }

    → processAnswerJob() sees Substituted=false
    → MarkAnswerJobSucceeded(payload with next question)
    → Frontend polls, gets "succeeded", dispatches SUBMIT_SUCCEEDED
    → User sees follow-up question in same area
Path B: Move to Next Criterion Area (2 AI Calls)
This happens when the AI says "sufficient", or "insufficient/partial but max questions reached."


processTurnCore() — FlowStepCriterion branch
=============================================

 1–8. (identical to Path A)

 9. ┌─────────────────────────────────────────┐
    │  AI CALL #1: callAIWithRetry()          │  ← EVALUATE answer + suggest next question
    │  Returns: Evaluation + NextQuestion     │
    └─────────────────────────────────────────┘

10. DecideCriterionTurn(evaluation, questionsCount, maxPerArea)
    Result: { Action: "next", MarkCurrentAs: "complete" }
    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
    "next" = done with this area, move on

11. DetermineNextAreaAfterCriterionTurn()
    Result: nextArea = "credible-fear" (different area)

12. decision.Action == "next", so DISCARD aiResult.NextQuestion
    ↓
13. ┌─────────────────────────────────────────┐
    │  AI CALL #2: generateNextAreaOpening()  │  ← GENERATE opening question for NEW area
    │  Builds fresh AITurnContext with:       │
    │    - IsOpeningTurn = true               │
    │    - CurrentAreaSlug = "credible-fear"  │
    │    - New area's description/reqs        │
    │    - Full answer history                │
    │  Returns: NextQuestion text             │
    └─────────────────────────────────────────┘

14. Generate new turnID (crypto/rand)
15. Build NextIssuedQuestion for the NEW area

16. ProcessCriterionTurn()                       ← DB TRANSACTION
    - Save answer to answers table
    - Mark CURRENT area as "complete"
    - Set NEW area to "in_progress"
    - Apply pre_addressed flags to other areas
    - Store NextIssuedQuestion as ActiveQuestion
    - Advance ExpectedTurnID
    - Increment QuestionNumber

17. Return AnswerResult{
      Done: false,
      NextQuestion: opening question (NEW area),
      Substituted: false,
    }

    → processAnswerJob() sees Substituted=false
    → MarkAnswerJobSucceeded(payload with next question)
    → Frontend polls, gets "succeeded", dispatches SUBMIT_SUCCEEDED
    → User sees opening question for the new criterion area
Path B with AI Failure (2 AI Calls, both fail)

 9. AI CALL #1: all 3 retries fail → ErrAIRetryExhausted
    → substituted = true
    → aiResult = fallbackEvaluation (partial + follow-up)
    → aiResult.NextQuestion = fallbackQuestionForArea(currentArea)

10. DecideCriterionTurn(fallback evaluation)
    Result: { Action: "continue" }  ← fallback always says "partial, follow up"
    NOTE: this means Path B BECOMES Path A when AI Call #1 fails!
    The area is NOT marked complete. A generic follow-up is used.

15. ProcessCriterionTurn()                       ← DB TRANSACTION COMMITS (with fallback data)

16. Return AnswerResult{ Substituted: true }

    → processAnswerJob() sees Substituted=true
    → MarkAnswerJobFailed(status: "canceled", code: "AI_RETRY_EXHAUSTED")
    → Frontend polls, sees "canceled", throws error
    → User sees error screen with "Reload to continue"
    → On reload: StartInterview() finds the fallback ActiveQuestion, returns it
The Key Difference at a Glance

                           AI Call #1 succeeds
                          /                    \
                   "continue"                "next"
                   (same area)             (new area)
                      |                        |
                      |                   AI Call #2
              Use NextQuestion            (opening question
              from AI Call #1              for new area)
                      |                        |
                      v                        v
              1 AI call total            2 AI calls total
              ~5-15s latency             ~10-30s latency
The branching decision point is service.go:570:


if decision.Action == CriterionTurnActionNext {
    nextQuestionText, nextAreaSubstituted, err = s.generateNextAreaOpeningQuestion(...)
}
That if is the fork. Everything before it is shared. Everything after it converges back at ProcessCriterionTurn().