# Afirmativo Developer Setup

This document is the current local development guide for this repository.

## Repo Overview

- `frontend/`: Next.js app for the bilingual interview flow
- `backend/`: Go API for sessions, interview state, reports, payments, admin cleanup, and voice token minting
- `utils/database/`: Go tooling for SQL migrations, coupon loading, and the local Postgres Studio UI
- `utils/mockThirdpartyAPIs/`: local mock AI service for development and testing
- `utils/design/`: product specs, technical notes, and refactoring docs
- `utils/terraform/`: optional local checkout of the private `afirmativo-tf` infra repo when operators need Terraform access
- `doc/`: developer documentation
- `.github/workflows/`: app-repo CI/CD that builds candidate images on `main` and dispatches release deploys to the private infra repo
- `dev-all.sh`: helper script that starts frontend, backend, the mock server, and the database UI together

## Prerequisites

- Node.js 20+ recommended
- npm 10+ recommended
- Go 1.26+
- Docker Desktop optional, only if you want a local Postgres container
- Ollama optional, only if you want local LLM inference instead of the mock or hosted AI path

## Release Automation

This public repo no longer runs Terraform directly in GitHub Actions.

- `main`
  - builds one backend candidate image and two frontend candidate images (`devcfg`, `prodcfg`)
  - pushes them to the dev Artifact Registry
- `release-dev-*`
  - dispatches the private infra repo to deploy dev with the exact candidate image refs for that commit
- `release-prod-*`
  - dispatches the private infra repo to deploy prod with the exact candidate image refs for that commit

The private infra repo owns:

- Terraform remote state
- Terraform `plan/apply`
- prod approval gates
- image promotion from dev registry to prod registry
- post-deploy smoke tests

Public build-time frontend values (`NEXT_PUBLIC_*`) come from GitHub variables, not local `.env` files.
Backend/runtime secrets stay in GCP Secret Manager and are referenced by Terraform.

## Local Development Flow

### 1. Prepare environment files

- Frontend:
  - Create `frontend/.env.local`
  - Set `API_PROXY_TARGET=http://localhost:8080`
  - `frontend/.env.example` exists if you want a template
- Backend:
  - Create `backend/.env` from `backend/.env.example`
  - The backend loads `.env` automatically in local development
- Database CLI:
  - Create `utils/database/.env`
  - It must include `DATABASE_URL_DIRECT`

Important:
- The browser-facing app now calls same-origin `/api/*`
- Locally, the frontend proxies `/api/*` to `API_PROXY_TARGET`
- In GCP, do not set `API_PROXY_TARGET`; the load balancer should own `/api/*`
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

If you are pointing `utils/database/.env` at that container, use:

```env
DATABASE_URL_DIRECT=postgres://postgres:password@localhost:5432/postgres?sslmode=disable
```

### 3. Apply migrations

```bash
cd utils/database
go run main.go up
go run main.go version
```

### 4. Start the AI dependency you want

Option A: mock AI server

```bash
cd utils/mockThirdpartyAPIs
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
- Browser health check through the frontend proxy: `http://localhost:3000/api/health`
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

### 7. Optional helper: local container verification

Use this path when you want deployment-parity validation, not day-to-day iteration.

From the project root, one-time setup if the scripts are not executable yet:

```bash
chmod +x run-backend-container.sh run-frontend-container.sh rebuild-local-containers.sh
```

Rebuild both local images:

```bash
./rebuild-local-containers.sh
```

Run backend:

```bash
./run-backend-container.sh
```

Common backend runtime overrides:

```bash
VOICE_AI_BASE_URL_OVERRIDE=http://host.docker.internal:9090 ./run-backend-container.sh
MOCK_API_URL_OVERRIDE=http://host.docker.internal:9090 ./run-backend-container.sh
OLLAMA_BASE_URL_OVERRIDE=http://host.docker.internal:11434 ./run-backend-container.sh
```

Run frontend in another terminal:

```bash
./run-frontend-container.sh
```

What these scripts do:

- write root-level log files like `container_back_<timestamp>.log`
- create the `afirmativo-local` Docker network if needed
- remove stale containers with the same names before starting
- mount local ADC automatically in the backend script if the default gcloud ADC file exists

Important:

- native dev remains the main workflow
- container verification is the pre-deploy workflow
- if you change frontend code or `frontend/next.config.ts`, rebuild the frontend image before rerunning the container
- if you change backend code, rebuild the backend image before rerunning the backend container
- if you only change runtime env overrides, restarting the affected container is often enough
- the frontend local container runner passes `PORT=3000` explicitly; the deploy image should not rely on a hardcoded Cloud Run port default

## Common Commands

### Frontend

```bash
cd frontend
npm run dev
npm run lint
npm run test
npm run build
```

Container verification helpers from the project root:

```bash
./rebuild-local-containers.sh
./run-frontend-container.sh
```

### Backend

```bash
cd backend
go run ./cmd/server
go test ./...
```

Container verification helper from the project root:

```bash
./run-backend-container.sh
```

If you edit SQL queries and need regenerated types:

```bash
cd backend
sqlc generate
```

### Database

```bash
cd utils/database
go run main.go up
go run main.go down
go run main.go down all
go run main.go version
go run main.go load_coupon
go run main.go studio
./reset.sh
```

### Local Postgres Studio

The `utils/database` module also includes a local read-only table browser for the database pointed to by `DATABASE_URL_DIRECT`.

Start it with:

```bash
cd utils/database
go run main.go studio
```

Defaults:

- Binds to `127.0.0.1:3010`
- Reads the same `utils/database/.env` file and `DATABASE_URL_DIRECT` as the migration CLI
- Shows all non-system schemas and tables, lets you browse table data, add dropdown-based filters, and inspect the generated SQL preview
- Allows deleting selected rows from the current filtered page for tables that have a primary key; tables without a primary key remain browse-only

Optional:

```bash
cd utils/database
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
cd utils/database
go run main.go load_coupon --prefix BETA --count 25 --max-uses 1 --discount-pct 100 --source beta_testers --token-length 8
```

Each inserted coupon is printed to stdout, followed by an inserted/skipped/failed summary.

## Local Docker Postgres Cleanup

This is the section to use when you want to wipe local Postgres state during development.

### Reset the schema but keep the container

If you want to clear the database and rebuild it with the current migrations:

```bash
cd utils/database
./reset.sh
```

That script does two things:
- `go run main.go down all`
- `go run main.go up`

### Drop all migrated tables and leave the database empty

If you want the database cleaned out without reapplying migrations:

```bash
cd utils/database
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

`dev-all.sh` can optionally prompt for `gcloud auth application-default login` before starting services.

## GCP Verification Shortcuts

Use these commands after a Terraform apply when the deployed app or domain does not behave as expected.

Check the backend Cloud Run service template, including plain env vars and Secret Manager references:

```bash
gcloud run services describe asilo-backend \
  --region=us-central1 \
  --project=asilo-afirmativo-dev \
  --format=export
```

Show the newest backend revision and the latest healthy revision:

```bash
gcloud run services describe asilo-backend \
  --region=us-central1 \
  --project=asilo-afirmativo-dev \
  --format='value(status.latestCreatedRevisionName,status.latestReadyRevisionName)'
```

Inspect one backend revision's logs:

```bash
gcloud logging read \
  'resource.type="cloud_run_revision"
   resource.labels.service_name="asilo-backend"
   resource.labels.revision_name="REVISION_NAME"' \
  --project=asilo-afirmativo-dev \
  --limit=100 \
  --format='table(timestamp,severity,textPayload,jsonPayload.message,jsonPayload.error)'
```

Check the custom-domain certificate state:

```bash
gcloud compute ssl-certificates describe asilo-app-managed-cert \
  --global \
  --project=asilo-afirmativo-dev \
  --format='yaml(managed.status,managed.domainStatus)'
```

Check that DNS resolves the root domain to the load balancer IP:

```bash
dig +short asilo-afirmativo.com
```

Important:

- `terraform output` is not the source of truth for loaded runtime env vars
- Cloud Run `Variables & Secrets` or `gcloud run services describe ... --format=export` is the source of truth
- Google-managed certificates can remain in `PROVISIONING` until DNS points at the load balancer and propagation finishes
