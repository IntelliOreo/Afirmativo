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

## 13. Production Readiness Assessment

### Strengths

1. **Async job processing with recovery** -- the recovery loop catches orphaned jobs every 10s. This is production-grade resilience.
2. **Idempotency on both sides** -- `clientRequestId` + server-side upsert means double-submits are safe, even after browser crashes.
3. **Turn-based conflict detection** -- prevents stale answer corruption with optimistic concurrency. No two requests can advance the same turn.
4. **Graceful AI degradation** -- fallback evaluations and fallback questions mean the interview never hard-fails on AI issues.
5. **localStorage persistence** -- users survive browser crashes without losing their in-progress answer.
6. **Circuit breakers on the frontend** -- prevent cascade failures when the backend is struggling.
7. **Clean domain separation** -- each package owns its data, logic, and storage interface. No cross-package imports of concrete types.
8. **Structured logging** -- consistent `slog` usage with correlation fields (session_code, job_id, client_request_id).
9. **DB migration discipline** -- 28 incremental migrations, all with up/down scripts.
10. **Context timeouts everywhere** -- every DB call gets a 5s timeout. Every async job gets a 3min timeout. No unbounded waits.

### Weaknesses

#### High Severity

1. **In-memory channel as job queue** -- `asyncAnswerQueue chan string` lives in process memory. If the Go server restarts, all jobs in the channel are lost. The recovery loop re-picks from DB, but there is a **10s window** where jobs sit in limbo. Under load during deploys, this causes noticeable delays.

2. **Single-server assumption** -- Rate limiters (`ratelimit.go`), lockout state (`lockout.go`), and the async job channel are all in-memory. Scaling to multiple backend instances requires moving these to Redis or similar shared state.

3. **No dead-letter queue or alerting** -- Failed jobs are marked terminal but there is no mechanism to re-process them or alert on failure rates. During an extended AI provider outage, failed jobs accumulate silently with no operator visibility.

#### Medium Severity

4. **Timer drift on frontend** -- The `TICK` uses `setInterval(1000)`. JavaScript timers are imprecise: tab backgrounding, heavy rendering, or GC pauses cause drift. The server is the source of truth for expiry, but the user sees a misleading countdown. A `Date.now()` comparison approach would be more accurate.

5. **No distributed tracing** -- There is logging with session codes, but no distributed trace IDs (OpenTelemetry, X-Request-ID propagation). Debugging a specific user's flow across frontend -> backend -> AI provider requires correlating logs manually.

6. **Fallback substitution is invisible to users** -- When AI retries exhaust and a fallback question/evaluation is used, the user does not know quality degraded. The interview continues with generic questions, potentially producing a less useful report.

7. **Error messages are hardcoded in two languages** -- Some error messages in the frontend are inline bilingual strings (e.g., useInterviewMachine.ts:427-429). This does not scale and is easy to get out of sync.

#### Low Severity / Not-Yet-Wired

8. **Payment is stubbed** -- Stripe checkout and webhook handlers return 501. The payment flow is not yet functional.

9. **PDF generation is client-side only** -- `GET /api/report/{code}/pdf` returns 501. Server-generated PDFs (for email delivery or archival) are not available.

10. **Voice transcription token lifecycle** -- Tokens are minted per-request with a TTL, but there is no client-side token caching. Rapid consecutive recordings each trigger a new token mint, which under rate limiting could fail.

11. **No integration tests for the async pipeline** -- The codebase has unit test infrastructure (Vitest for frontend) but end-to-end tests that exercise submit -> process -> poll -> succeed are not visible. This is the riskiest code path.

---

## 14. Patterns Cheat Sheet

| Pattern | Where | Why |
|---------|-------|-----|
| `useReducer` state machine | `useInterviewMachine.ts` | Predictable state transitions, no impossible states |
| Brand types | `models.ts` | Compile-time prevention of ID mixups |
| Optimistic concurrency (turnID) | `service.go` | Prevent stale answer processing |
| Idempotency key | `clientRequestId` | Safe retries after browser crashes |
| Exponential backoff + jitter | `useAsyncAnswerPolling.ts` | Prevent thundering herd on retries |
| Circuit breaker | Polling hooks | Stop hammering a failing server |
| Bounded worker pool | `service_async.go` | Prevent unbounded goroutine growth |
| Periodic recovery loop | `service_async.go` | At-least-once processing guarantee |
| localStorage checkpointing | `pendingAnswerStore.ts` | Survive browser crashes |
| Interface-based DI | All backend services | Testability, swappable implementations |
| Fallback substitution | `service.go` | Graceful degradation when AI fails |
| Generation-based cancellation | `useVoiceRecorder.ts` | Invalidate stale async callbacks |
| `canceled` flag in effects | `useInterviewMachine.ts` | Prevent state updates after unmount |
| `sync.Once` for runtime start | `service_async.go` | Ensure worker pool starts exactly once |
| Context timeouts on every DB call | All service methods | Prevent unbounded database waits |

---

## 15. If You Are Touching This Code

- **Read the state machine reducer first** (useInterviewMachine.ts:122-187) -- it is 65 lines and tells you every possible state the interview can be in.
- **The async pipeline is the critical path** -- submit -> enqueue -> claim -> process -> succeed. Trace it end to end before changing anything.
- **Turn IDs are your concurrency safety** -- never skip the `expectedTurnID` check.
- **localStorage is your crash safety** -- if you add new persistent state, follow the same read/write/clear pattern in `lib/storage/`.
- **AI providers are interchangeable** -- the `InterviewAIClient` interface abstracts everything. Adding a new provider means implementing one method.
- **The recovery loop is your safety net** -- if something goes wrong with the in-memory queue, the recovery loop will fix it within 10 seconds.
- **Every DB call has a 5s timeout** -- follow the same pattern: `dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout); defer dbCancel()`.
