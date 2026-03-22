#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
LOG_FILE="$SCRIPT_DIR/dev_${TIMESTAMP}.log"

# Clear log file on each start
: > "$LOG_FILE"

# Send all stdout/stderr to terminal + log file
exec > >(tee -a "$LOG_FILE") 2>&1

if [[ ! -t 0 ]]; then
  echo "This script needs an interactive terminal so you can choose which services to start."
  exit 1
fi

SERVICE_NAMES=("frontend" "backend" "mock third-party API" "database UI")
SERVICE_PORTS=(3000 8080 9090 3010)
SERVICE_PREFIXES=("frontend" "backend" "mock" "database")
SERVICE_DIRS=(
  "$SCRIPT_DIR/frontend"
  "$SCRIPT_DIR/backend"
  "$SCRIPT_DIR/utils/mockThirdpartyAPIs"
  "$SCRIPT_DIR/utils/database"
)
SERVICE_COMMANDS=(
  "npm run dev"
  "go run ./cmd/server"
  "go run ."
  "go run . studio"
)
SELECTED_SERVICES=()

prompt_yes_no() {
  local prompt="$1"
  local answer

  while true; do
    read -r -p "$prompt [Y/n] " answer
    case "${answer:-Y}" in
      [Yy]) return 0 ;;
      [Nn]) return 1 ;;
      *) echo "Please enter y or n." ;;
    esac
  done
}

if prompt_yes_no "Run gcloud auth application-default login?"; then
  gcloud auth application-default login
fi

echo "Choose which services to start:"
for i in "${!SERVICE_NAMES[@]}"; do
  if prompt_yes_no "Start ${SERVICE_NAMES[$i]}?"; then
    SELECTED_SERVICES+=("$i")
  fi
done

if [[ ${#SELECTED_SERVICES[@]} -eq 0 ]]; then
  echo "No services selected. Exiting."
  exit 0
fi

for index in "${SELECTED_SERVICES[@]}"; do
  port="${SERVICE_PORTS[$index]}"
  pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN || true)"
  if [[ -n "$pids" ]]; then
    echo "Killing process(es) on port $port: $pids"
    kill $pids 2>/dev/null || true
    sleep 1

    still_listening="$(lsof -tiTCP:"$port" -sTCP:LISTEN || true)"
    if [[ -n "$still_listening" ]]; then
      echo "Force killing process(es) still on port $port: $still_listening"
      kill -9 $still_listening 2>/dev/null || true
    fi
  fi
done

cleanup() {
  trap - INT TERM EXIT
  kill 0 2>/dev/null || true
}

trap cleanup INT TERM EXIT

for index in "${SELECTED_SERVICES[@]}"; do
  service_dir="${SERVICE_DIRS[$index]}"
  service_command="${SERVICE_COMMANDS[$index]}"
  service_prefix="${SERVICE_PREFIXES[$index]}"

  (
    cd "$service_dir"
    export PORT="${SERVICE_PORTS[$index]}"
    eval "$service_command" 2>&1 | sed -u "s/^/[$service_prefix] /"
  ) &
done

wait
