#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"

LOG_FILE="${LOG_FILE:-$SCRIPT_DIR/container_back_${TIMESTAMP}.log}"
NETWORK_NAME="${NETWORK_NAME:-afirmativo-local}"
CONTAINER_NAME="${CONTAINER_NAME:-afirmativo-backend}"
IMAGE_NAME="${BACKEND_IMAGE:-afirmativo-backend:local}"
HOST_PORT="${HOST_PORT:-8080}"
CONTAINER_PORT="${CONTAINER_PORT:-8080}"
BACKEND_ENV_FILE="${BACKEND_ENV_FILE:-$SCRIPT_DIR/backend/.env}"
FRONTEND_URL="${FRONTEND_URL:-http://localhost:3000}"
DATABASE_URL_OVERRIDE="${DATABASE_URL_OVERRIDE:-postgres://postgres:password@host.docker.internal:5432/postgres?sslmode=disable}"
ADC_HOST_PATH="${ADC_HOST_PATH:-$HOME/.config/gcloud/application_default_credentials.json}"
ADC_CONTAINER_PATH="/var/secrets/google/application_default_credentials.json"

: > "$LOG_FILE"
exec > >(tee -a "$LOG_FILE") 2>&1

echo "Backend container log file: $LOG_FILE"

if [[ ! -f "$BACKEND_ENV_FILE" ]]; then
  echo "Backend env file not found: $BACKEND_ENV_FILE" >&2
  exit 1
fi

if ! docker image inspect "$IMAGE_NAME" >/dev/null 2>&1; then
  echo "Backend image not found: $IMAGE_NAME" >&2
  echo "Build it first with: docker build -t $IMAGE_NAME ./backend" >&2
  exit 1
fi

docker network create "$NETWORK_NAME" >/dev/null 2>&1 || true
docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

run_args=(
  docker run --rm
  --name "$CONTAINER_NAME"
  --network "$NETWORK_NAME"
  -p "${HOST_PORT}:${CONTAINER_PORT}"
  --env-file "$BACKEND_ENV_FILE"
  -e "PORT=${CONTAINER_PORT}"
  -e "FRONTEND_URL=${FRONTEND_URL}"
  -e "DATABASE_URL=${DATABASE_URL_OVERRIDE}"
)

if [[ -n "${VOICE_AI_BASE_URL_OVERRIDE:-}" ]]; then
  run_args+=(-e "VOICE_AI_BASE_URL=${VOICE_AI_BASE_URL_OVERRIDE}")
fi

if [[ -n "${MOCK_API_URL_OVERRIDE:-}" ]]; then
  run_args+=(-e "MOCK_API_URL=${MOCK_API_URL_OVERRIDE}")
fi

if [[ -n "${OLLAMA_BASE_URL_OVERRIDE:-}" ]]; then
  run_args+=(-e "OLLAMA_BASE_URL=${OLLAMA_BASE_URL_OVERRIDE}")
fi

if [[ -f "$ADC_HOST_PATH" ]]; then
  echo "Mounting ADC credentials from: $ADC_HOST_PATH"
  run_args+=(
    -v "${ADC_HOST_PATH}:${ADC_CONTAINER_PATH}:ro"
    -e "GOOGLE_APPLICATION_CREDENTIALS=${ADC_CONTAINER_PATH}"
  )
else
  echo "ADC file not found at $ADC_HOST_PATH; continuing without mounted ADC."
fi

run_args+=("$IMAGE_NAME")

echo "Starting backend container: $CONTAINER_NAME"
echo "Using image: $IMAGE_NAME"
echo "Using network: $NETWORK_NAME"

"${run_args[@]}"
