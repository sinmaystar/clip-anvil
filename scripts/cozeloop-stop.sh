#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${CLIPANVIL_COZELOOP_ENV_FILE:-deploy/cozeloop/.env}"
COMPOSE_FILE="deploy/docker-compose.cozeloop.yml"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "[cozeloop] $ENV_FILE is missing; nothing to stop"
  exit 0
fi

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" down
