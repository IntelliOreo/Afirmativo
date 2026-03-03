# Affirmative Interview Simulator — Developer Setup

## Prerequisites

- Node.js 20+ (`node --version`)
- npm 10+ (`npm --version`)
- Go 1.22+ (`go version`) — only needed for backend work
- Docker Desktop — for local containerized dev

---

## Frontend Setup (Next.js)

```bash
# 1. Install dependencies (must do this before editing — clears TypeScript errors)
cd frontend
npm install

# 2. Start development server (hot reload)
npm run dev
# → Open http://localhost:3000

# 3. Type-check without running
npx tsc --noEmit

# 4. Lint
npm run lint

# 5. Production build (static export → /frontend/out/)
npm run build
```

> **Note:** All TypeScript squiggly lines in VS Code for missing `react`, `next`, etc. will disappear
> once you run `npm install`. The errors are due to missing `node_modules/` — not code issues.

---

## Environment Variables

Create `frontend/.env` for local development:

```env
NEXT_PUBLIC_API_URL=http://localhost:8080
```

In production this is set via Cloud Run (Terraform).

---

## Database Setup (Neon Postgres)

The `database/` folder is a Go CLI that runs versioned SQL migrations against Neon. It is not a container — it runs locally and in CI/CD.

### First-time setup (already done — go.mod and .env exist)

```bash
cd database/
cp .env.example .env
# → Edit .env and fill in DATABASE_URL_DIRECT (use direct URL, no -pooler)
```

### Daily commands

```bash
cd database/

go run main.go up         # apply all pending migrations
go run main.go down       # roll back one migration
go run main.go down all   # roll back everything
go run main.go version    # show current migration version
./reset.sh                # dev only: drop everything + re-apply
```

### Adding a new migration

```bash
# Manually create a pair of files:
#   database/migrations/000002_create_coupons.up.sql
#   database/migrations/000002_create_coupons.down.sql
# Then apply:
go run main.go up
```

See `database-spec-v1.md` for the full schema and migration strategy.

---

## Backend Setup (Go)

```bash
cd backend

# 1. Start the server (loads .env automatically)
go run ./cmd/server
# → listening on http://localhost:8080

# 2. Test health check
curl http://localhost:8080/api/health

# 3. Start mock AI server (returns random interview questions)
cd mockThirdpartyAPIs
go run main.go
# → listening on http://localhost:9090
# → curl http://localhost:9090/api

# 4. Test coupon validation (requires a coupon in the DB — see below)
curl -X POST http://localhost:8080/api/coupon/validate \
  -H "Content-Type: application/json" \
  -d '{"code":"BETA-0001"}'
```

### Loading test coupons

```bash
cd database
go run main.go load_coupon
```

### Regenerating sqlc code

After editing `backend/sql/queries/*.sql`, regenerate the type-safe Go code:

```bash
cd backend
sqlc generate
```

Requires `sqlc` (`brew install sqlc`).

---

## Local Dev with Docker Compose

Spins up frontend + backend together (requires backend repo to be cloned alongside):

```bash
# From project root
docker compose up

# Frontend → http://localhost:3000
# Backend  → http://localhost:8080
```

---

## Project Structure

```
/frontend
  src/app/            ← Next.js App Router pages
    page.tsx                  → /
    disclaimer/page.tsx       → /disclaimer
    pay/page.tsx              → /pay
    session/[code]/page.tsx   → /session/AP-XXXX-XXXX
    interview/[code]/page.tsx → /interview/AP-XXXX-XXXX
    report/[code]/page.tsx    → /report/AP-XXXX-XXXX
    admin/page.tsx            → /admin
  components/         ← Shared UI primitives (Button, Card, Input, Alert, NavHeader, Footer)
  tailwind.config.ts  ← USWDS color palette + typography tokens
  next.config.ts      ← Static export config

/backend/             ← Go API server (Cloud Run container)
/database/            ← Migration tooling (NOT a container — runs locally and in CI)
  migrations/         ← Versioned SQL files (up/down pairs)
  main.go             ← Go migration CLI
  reset.sh            ← Dev helper: drop all + re-apply

/infra/               ← Terraform: Cloud Run, networking, secrets
/design/              ← Reference screenshots (USCIS.gov aesthetic)
/doc/                 ← Developer documentation
docker-compose.yml    ← Local dev only
.github/workflows/    ← CI/CD pipeline
```

---

## CI/CD

Push to `main` → GitHub Actions:
1. TypeScript lint
2. `npm run build`
3. Docker build
4. Push to GCP Artifact Registry
5. `terraform apply` → deploys to Cloud Run

Frontend and backend deploy independently.

---

## Key Conventions

- All user-facing strings must be **bilingual** (Spanish + English)
- Pages use **component abstractions** — never raw Tailwind classes in `page.tsx` files
- Keep business APIs in Go; Next.js route handlers are only for local/dev admin proxying
- Frontend env vars: `NEXT_PUBLIC_API_URL` (required). Admin proxy route is gated by `NODE_ENV=development`.
- Target device: cheap Android phones on Chrome
