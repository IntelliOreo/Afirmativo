# Afirmativo

Bilingual AI-assisted practice tool for asylum interview preparation. Session-based, supports typed and voice answers, async AI evaluation, bilingual reports.

## Architecture

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

```text
frontend/                Next.js app, landing/session pages, mobile-first interview composer, browser-side interview state machine
backend/                 Go API: sessions, interview, reports, payments, voice
```

## Design Choices and Trade-offs

### Async answer processing via Postgres job queue

AI evaluation takes 5-15s per turn. Synchronous would block the browser and risk HTTP timeouts. Instead, answers are submitted as async jobs backed by Postgres rows. Workers claim jobs with `SKIP LOCKED`, which gives us a distributed job queue without adding Redis or SQS. A periodic recovery loop requeues stale jobs; idle workers also poll Postgres directly as a fallback.

**Trade-off**: queue hints are process-local channels. A worker on instance B won't be hinted when instance A enqueues — it falls back to DB polling via recovery. Acceptable at current scale.

### Async request correlation is DB-backed

Interview answer jobs and reports persist the latest triggering HTTP `request_id` as `last_request_id`. Worker logs keep that correlation across restart and cross-instance claim handoff.

### Rate limiting is process-local

Token-bucket throttles and failed-attempt lockouts use in-memory maps. State resets on restart and doesn't span instances.

**Trade-off accepted**: traffic is low. Distributed rate limiting would require Redis or equivalent — not warranted yet. First scaling step when needed.

### Payment config is typed, fulfillment stays local

`config.go` now validates the Stripe secrets plus the two checkout amounts used by the payment flow. `main.go` still owns the product names and fulfillment wiring for `direct_session` and `coupon_pack_10`.

**Decision**: pricing lives in config, but the small product catalog and fulfillment semantics stay in code.
**Trade-off**: startup validation stays strong without turning payment setup into a wider abstraction, but future product shapes require an explicit code change instead of a config flip.

`coupon_pack_10` is intentionally a fixed 10-use product. Only pricing is configurable.
**Decision**: `POST /api/payment/checkout` currently treats a missing `product` as `direct_session` during rollout.
**Trade-off**: older callers keep working while frontend and backend can roll independently, but malformed new callers can still under-specify the request until that fallback is removed. Once all callers send an explicit product, missing `product` should become a `400`.

### No Stripe SDK

Zero-dependency Stripe integration: form-encoded POST for hosted checkout, HMAC-SHA256 for webhook signature verification (timing-safe via `hmac.Equal`). We only use a single Stripe surface (checkout sessions), so the SDK's weight isn't justified. The frontend treats checkout start failures as generic retryable runtime errors, not as a feature-unavailable rollout state.

**Trade-off**: we own signature verification code and any Stripe API drift. Acceptable for one endpoint.

### Install metadata is truthful but intentionally minimal

The frontend serves a real app manifest and generated app icons so browsers are not pointed at missing install assets.

**Trade-off**: this is install metadata only. There is still no service worker, offline cache, or special install flow. The app behaves like a normal networked web app after installation.

### Payment state machine with pessimistic locking

Both the Stripe webhook and the browser poll can resolve fulfillment under the same locked payment row. `FOR UPDATE` prevents double-provisioning. Direct-session reveal PINs still have a 10-minute TTL and are consumed on first read; coupon-pack reveals are repeatable because the issued coupon is not treated like secret PIN material.

The frontend session handoff bootstrap waits for language initialization and runs once per session page load. This keeps the one-time PIN handoff resilient to client rerenders while preserving immediate PIN consumption from browser storage.
Coupon redemptions no longer rely on browser-only coupon handoff state. The redeemed coupon snapshot is stored on the session and returned by `POST /api/session/verify`, so `/session/[code]` can show the same coupon summary again after reload or manual PIN verification.
Hosted Stripe Checkout now supports two products through the same webhook endpoint: a one-time direct session and a 10-use coupon pack. `/pay/success` either redirects into `/session/{code}` for direct-session fulfillment or shows the issued coupon for the coupon-pack path.
That same ready state now offers both `Copy all` and a user-controlled `mailto:` handoff, with the email body reusing the exact same session/coupon reveal text.

**Trade-off**: row-level locks under contention. A single payment rarely has concurrent mutations.

### JWT in HttpOnly cookies

Tokens can't be read by XSS. The auth cookie is `SameSite=Lax`. Same-origin `/api/*` means no cross-subdomain auth needed.

### Inline i18n catalogs

Two languages (en/es), ~30 strings per page. Type-safe `Record<Lang, T>` with `satisfies`. No react-intl/next-intl.

**Trade-off**: won't scale past 5+ languages or translator workflows. Appropriate for current scope; migration path is straightforward.

### sqlc, not an ORM

SQL queries live in `.sql` files, Go code is generated. No runtime reflection, no query builder. Queries are auditable and type-safe.

### stdlib net/http, no framework

Composable middleware chain (`Chain` function). No gin/echo/chi. Minimal dependency tree.

### OpenTelemetry wired but noop by default

OTel tracer is always in the middleware chain. Locally it runs as a noop provider (`OTEL_ENABLED=false`) with zero overhead. In GCP it exports to Cloud Trace. The code path is identical either way — no conditional wiring.

### Interview package is large and stays that way

The backend `interview/` package is ~30 files (~2400 lines of production code, ~3200 lines of tests). The frontend interview page + state machine + hooks is another ~700 lines. These are the largest modules in the codebase.

They were already split by responsibility seam over multiple passes: async runtime, turn policy, criterion helpers, AI retry, timing, snapshots, and control flow each have their own file. The core control flow was deliberately left in place each time. The state machine reducer on the frontend was similarly preserved — timeout, draft, voice, and polling each became their own hook, but the reducer stays as the lifecycle backbone.

The voice composer keeps the same review/edit/submit flow, but the primary action now stops and opens transcript review in one step. Replay lives in the secondary control row instead of beside the timer.

**Why not refactor further**: the interview flow is the protected area of the app. Every speculative rewrite attempt (new layers, package reshuffles, shared worker abstractions) was evaluated and rejected because the risk-to-payoff ratio was wrong. The domain is genuinely complex — multi-step branching, AI retry with degraded fallback, async processing with recovery, timeout with draft persistence, voice transcription lifecycle. The files are large because the domain is large, not because the structure is lazy.

**Trade-off accepted**: new contributors will need to read more context before touching the interview path. That's preferable to a "clean" abstraction that hides the actual control flow and makes failure modes invisible. The current shape optimizes for debuggability and safe modification over visual elegance.

## What's Not Built Yet

- `GET /api/report/{code}/pdf` returns `501`
- Distributed rate limiting (needs Redis)
- Languages beyond en/es
- Offline/PWA support beyond install metadata

## If We Need Multiple Instances Later

The current system is intentionally optimized for low cost and low operational complexity. We do not need these changes yet, but scaling to multiple backend instances would require revisiting:

- distributed rate limiting
- async worker topology (embedded in app instances vs dedicated worker instances)
- cluster-wide metrics plus shared cache metadata where cost or observability warrants it

Detailed scaling notes live in `utils/design/codebase-deep-dive.md`.

## Routes

```text
GET  /api/health                            instance-local health + pool stats + async queue depths
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
GET  /api/payment/checkout-sessions/{id}
POST /api/payment/webhook
POST /api/admin/cleanup-db
```

## Local Dev

```bash
./dev-all.sh                    # starts frontend + backend + mock AI + DB studio
go run ./cmd/server             # backend only
npm run dev                     # frontend only
```

## Local E2E

Browser E2E lives in `e2e/` and is intentionally narrow: smoke coverage plus the access handoff journeys.

Requirements:
- frontend dependencies installed in `frontend/`
- e2e dependencies installed in `e2e/`
- local or CI Postgres admin URL exposed as `AFIRMATIVO_TEST_DATABASE_URL`

Run locally:

```bash
cd e2e
npx playwright test
```

The E2E setup creates a throwaway database, applies the repo-local `e2e/migrations` snapshot, seeds `E2E-TEST-COUPON`, starts the backend with a test Stripe mock, starts the frontend with `API_PROXY_TARGET=http://127.0.0.1:8080`, and tears the database down after the run.

## Deploy

Fully automated via GitHub Actions + Terraform. No manual `gcloud` or `terraform apply` in the deploy path.

```text
pull_request -> main
  -> GitHub Actions runs backend/frontend verification only

main push
  -> GitHub Actions builds immutable candidate images (backend:sha, frontend:sha-devcfg, frontend:sha-prodcfg)
  -> pushes to Artifact Registry via WIF (no static GCP keys)

release-dev-* / release-prod-* push
  -> dispatches the private Terraform repo
  -> private repo runs terraform plan/apply + post-deploy smoke tests
```

Same-origin `/api/*` contract. GCP load balancer splits `/api/*` to backend, everything else to frontend. Secrets stay in GCP Secret Manager — GitHub Actions never reads `.env` files.

The repos are split by trust boundary: the public app repo can only push images (Artifact Registry write). The private infra repo owns deploy authority (Cloud Run, Terraform state, IAM).

The workflow is deliberately minimal to control costs. Heavy Docker builds run on the public repo (free GitHub-hosted runners). The private infra repo only runs promotion, Terraform, and smoke tests — keeping billed private minutes low.

**Trade-off on prod permissions**: the deploy identity currently has broader GCP access than ideal. The target state separates bootstrap/admin Terraform (APIs, IAM, networking) from deploy/runtime Terraform (image selection, Cloud Run revisions). Prod should eventually use private-repo environment approval + least-privilege IAM scoped to image rollout only. Not yet implemented because the team is small and the blast radius is contained.
