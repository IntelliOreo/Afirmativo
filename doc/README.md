# Afirmativo Developer Setup

This document is the current local development guide for this repository.

## Repo Overview

- `frontend/`: Next.js app for the bilingual interview flow
- `backend/`: Go API for sessions, interview state, reports, payments, admin cleanup, and voice token minting
- `database/`: Go tooling for SQL migrations, coupon loading, and the local Postgres Studio UI
- `mockThirdpartyAPIs/`: local mock AI service for development and testing
- `design/`: product specs, technical notes, and refactoring docs
- `infra/`: Terraform for Cloud Run, networking, and secrets (future/planned)
- `doc/`: developer documentation
- `docker-compose.yml`: local dev only (future/planned)
- `.github/workflows/`: CI/CD pipeline (future/planned)
- `dev-all.sh`: helper script that starts frontend, backend, the mock server, and the database UI together

## Prerequisites

- Node.js 20+ recommended
- npm 10+ recommended
- Go 1.26+
- Docker Desktop optional, only if you want a local Postgres container
- Ollama optional, only if you want local LLM inference instead of the mock or hosted AI path

## Local Development Flow

### 1. Prepare environment files

- Frontend:
  - Create `frontend/.env.local`
  - Set `NEXT_PUBLIC_API_URL=http://localhost:8080`
  - `frontend/.env.example` exists if you want a template
- Backend:
  - Create `backend/.env` from `backend/.env.example`
  - The backend loads `.env` automatically in local development
- Database CLI:
  - Create `database/.env`
  - It must include `DATABASE_URL_DIRECT`

Important:
- The backend reads `DATABASE_URL`
- The database migration CLI reads `DATABASE_URL_DIRECT`
- They are not interchangeable

### 2. Start Postgres

You can use either Neon or a local Docker container.

If you want local Docker Postgres, there is currently no checked-in Docker Compose file in this repo. Use a standalone container:

```bash
docker run --name test-postgres \
  -e POSTGRES_PASSWORD=password \
  -p 5432:5432 \
  -d postgres:latest
```

If that container already exists:

```bash
docker start test-postgres
```

If you are pointing `database/.env` at that container, use:

```env
DATABASE_URL_DIRECT=postgres://postgres:password@localhost:5432/postgres?sslmode=disable
```

### 3. Apply migrations

```bash
cd database
go run main.go up
go run main.go version
```

### 4. Start the AI dependency you want

Option A: mock AI server

```bash
cd mockThirdpartyAPIs
go run .
```

Option B: Ollama

```bash
ollama serve
```

If you use Ollama, make sure your backend config also sets:

- `AI_PROVIDER=ollama`
- `OLLAMA_BASE_URL=http://localhost:11434` unless you changed the default
- `AI_MODEL` to a model you already pulled into Ollama

If the model is not on your machine yet, pull it before starting the backend:

```bash
ollama pull <your-model-name>
```

### 5. Start backend and frontend

Backend API:

```bash
cd backend
go run ./cmd/server
```

Frontend:

```bash
cd frontend
npm install
npm run dev
```

Once everything is running:

- Frontend: `http://localhost:3000`
- Backend health check: `http://localhost:8080/api/health`
- Postgres Studio: `http://localhost:3010`
- Mock AI server: `http://localhost:9090/api`
- Ollama default base URL: `http://localhost:11434`

### 6. Optional helper: run the app stack together

From the repo root:

```bash
./dev-all.sh
```

Notes:
- This script starts the frontend, backend, and mock AI server
- It also clears any processes already listening on ports `3000`, `8080`, and `9090`
- Run it only after your env files are ready
- It does not start Docker Postgres or Ollama for you

## Common Commands

### Frontend

```bash
cd frontend
npm run dev
npm run lint
npm run test
npm run build
```

### Backend

```bash
cd backend
go run ./cmd/server
go test ./...
```

If you edit SQL queries and need regenerated types:

```bash
cd backend
sqlc generate
```

### Database

```bash
cd database
go run main.go up
go run main.go down
go run main.go down all
go run main.go version
go run main.go load_coupon
go run main.go studio
./reset.sh
```

### Local Postgres Studio

The `database` module also includes a local read-only table browser for the database pointed to by `DATABASE_URL_DIRECT`.

Start it with:

```bash
cd database
go run main.go studio
```

Defaults:

- Binds to `127.0.0.1:3010`
- Reads the same `database/.env` file and `DATABASE_URL_DIRECT` as the migration CLI
- Shows all non-system schemas and tables, lets you browse table data, add dropdown-based filters, and inspect the generated SQL preview
- Allows deleting selected rows from the current filtered page for tables that have a primary key; tables without a primary key remain browse-only

Optional:

```bash
cd database
STUDIO_ADDR=127.0.0.1:3010 go run main.go studio
go run main.go studio --addr 127.0.0.1:3010
```

### Coupon creation with all supported flags

The coupon loader generates codes and inserts them into the `coupons` table.

Supported flags:

- `--prefix`: coupon prefix, default `COUPON`
- `--count`: number of coupons to generate, default `10`
- `--max-uses`: max uses per coupon, default `1`
- `--discount-pct`: discount percent, default `100`
- `--source`: source tag for auditability, default empty
- `--token-length`: random suffix length, default `8`

Original example:

```bash
go run main.go load_coupon --prefix TEST --count 5 --max-uses 100 --discount-pct 100 --source manual_test
```

Full example with every supported flag:

```bash
cd database
go run main.go load_coupon --prefix BETA --count 25 --max-uses 1 --discount-pct 100 --source beta_testers --token-length 8
```

Each inserted coupon is printed to stdout, followed by an inserted/skipped/failed summary.

## Local Docker Postgres Cleanup

This is the section to use when you want to wipe local Postgres state during development.

### Reset the schema but keep the container

If you want to clear the database and rebuild it with the current migrations:

```bash
cd database
./reset.sh
```

That script does two things:
- `go run main.go down all`
- `go run main.go up`

### Drop all migrated tables and leave the database empty

If you want the database cleaned out without reapplying migrations:

```bash
cd database
go run main.go down all
```

### Stop and remove the Docker Postgres container

If you want to fully remove the local Postgres container used in this doc:

```bash
docker stop test-postgres
docker rm test-postgres
```

If you want to force-remove it in one command:

```bash
docker rm -f test-postgres
```

### Remove the Postgres image too

Optional:

```bash
docker image rm postgres:latest
```

### Do you need to remove a Docker volume?

Not for the `docker run` command shown above.

That command does not attach a named Docker volume, so removing `test-postgres` is usually enough. If you later switch to a setup that uses a named volume, remove that volume separately with `docker volume rm <volume-name>`.

## Testing Notes

- Frontend tests run with `npm run test`
- Backend tests run with `go test ./...`
- The backend async Postgres integration tests are opt-in and can run against a local Docker Postgres instance when `AFIRMATIVO_TEST_DATABASE_URL` is set
- Those integration tests create and clean up their own temporary databases

gcloud auth application-default login
