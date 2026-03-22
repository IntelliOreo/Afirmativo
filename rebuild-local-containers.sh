#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

BACKEND_IMAGE="${BACKEND_IMAGE:-afirmativo-backend:local}"
FRONTEND_IMAGE="${FRONTEND_IMAGE:-afirmativo-frontend:local}"
FRONTEND_ENV_FILE="${FRONTEND_ENV_FILE:-$SCRIPT_DIR/frontend/.env.local}"

cd "$SCRIPT_DIR"

echo "Rebuilding backend image: $BACKEND_IMAGE"
docker build -t "$BACKEND_IMAGE" ./backend

if [[ -f "$FRONTEND_ENV_FILE" ]]; then
  echo "Loading frontend build vars from: $FRONTEND_ENV_FILE"
  set -a
  # shellcheck disable=SC1090
  source "$FRONTEND_ENV_FILE"
  set +a
else
  echo "Frontend env file not found at $FRONTEND_ENV_FILE; continuing with current shell env."
fi

echo "Rebuilding frontend image: $FRONTEND_IMAGE"
docker build \
  --build-arg NEXT_PUBLIC_VOICE_API_URL="${NEXT_PUBLIC_VOICE_API_URL:-}" \
  --build-arg NEXT_PUBLIC_LOG_LEVEL="${NEXT_PUBLIC_LOG_LEVEL:-info}" \
  --build-arg NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY="${NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY:-}" \
  --build-arg NEXT_PUBLIC_ALLOW_SENSITIVE_DEBUG_LOGS="${NEXT_PUBLIC_ALLOW_SENSITIVE_DEBUG_LOGS:-}" \
  --build-arg NEXT_PUBLIC_ASYNC_POLL_TIMEOUT_SECONDS="${NEXT_PUBLIC_ASYNC_POLL_TIMEOUT_SECONDS:-}" \
  --build-arg NEXT_PUBLIC_ASYNC_POLL_CIRCUIT_BREAKER_FAILURES="${NEXT_PUBLIC_ASYNC_POLL_CIRCUIT_BREAKER_FAILURES:-}" \
  --build-arg NEXT_PUBLIC_ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_SECONDS="${NEXT_PUBLIC_ASYNC_POLL_CIRCUIT_BREAKER_COOLDOWN_SECONDS:-}" \
  --build-arg NEXT_PUBLIC_VOICE_MAX_SECONDS="${NEXT_PUBLIC_VOICE_MAX_SECONDS:-}" \
  -t "$FRONTEND_IMAGE" \
  ./frontend

echo
echo "Local images rebuilt:"
echo "  backend  -> $BACKEND_IMAGE"
echo "  frontend -> $FRONTEND_IMAGE"
echo
echo "Next:"
echo "  ./run-backend-container.sh"
echo "  ./run-frontend-container.sh"
