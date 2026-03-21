#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"

LOG_FILE="${LOG_FILE:-$SCRIPT_DIR/container_front_${TIMESTAMP}.log}"
NETWORK_NAME="${NETWORK_NAME:-afirmativo-local}"
CONTAINER_NAME="${CONTAINER_NAME:-afirmativo-frontend}"
IMAGE_NAME="${FRONTEND_IMAGE:-afirmativo-frontend:local}"
HOST_PORT="${HOST_PORT:-3000}"
CONTAINER_PORT="${CONTAINER_PORT:-3000}"
API_PROXY_TARGET="${API_PROXY_TARGET:-http://afirmativo-backend:8080}"
ENABLE_ADMIN_TOOLS="${ENABLE_ADMIN_TOOLS:-true}"

: > "$LOG_FILE"
exec > >(tee -a "$LOG_FILE") 2>&1

echo "Frontend container log file: $LOG_FILE"

if ! docker image inspect "$IMAGE_NAME" >/dev/null 2>&1; then
  echo "Frontend image not found: $IMAGE_NAME" >&2
  echo "Build it first with: ./rebuild-local-containers.sh" >&2
  exit 1
fi

docker network create "$NETWORK_NAME" >/dev/null 2>&1 || true
docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

echo "Starting frontend container: $CONTAINER_NAME"
echo "Using image: $IMAGE_NAME"
echo "Using network: $NETWORK_NAME"
echo "Using container port: $CONTAINER_PORT"
echo "Using API proxy target: $API_PROXY_TARGET"

docker run --rm \
  --name "$CONTAINER_NAME" \
  --network "$NETWORK_NAME" \
  -p "${HOST_PORT}:${CONTAINER_PORT}" \
  -e "PORT=${CONTAINER_PORT}" \
  -e "API_PROXY_TARGET=${API_PROXY_TARGET}" \
  -e "ENABLE_ADMIN_TOOLS=${ENABLE_ADMIN_TOOLS}" \
  "$IMAGE_NAME"
