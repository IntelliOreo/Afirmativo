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

Create `frontend/.env.local` for local development:

```env
NEXT_PUBLIC_API_URL=http://localhost:8080
```

In production this is set via Cloud Run (Terraform).

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
- **No Next.js API routes** — the Go backend is the sole API layer
- `NEXT_PUBLIC_API_URL` is the only environment variable the frontend uses
- Target device: cheap Android phones on Chrome
