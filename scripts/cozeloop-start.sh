#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${CLIPANVIL_COZELOOP_ENV_FILE:-deploy/cozeloop/.env}"
COMPOSE_FILE="deploy/docker-compose.cozeloop.yml"

log() {
  echo "[cozeloop] $1"
}

copy_if_missing() {
  local source=$1 target=$2
  if [[ ! -e "$target" ]]; then
    cp "$source" "$target"
    log "created $target from $source"
  fi
}

prepare_local_config() {
  copy_if_missing "deploy/cozeloop/.env.example" "$ENV_FILE"

  while IFS= read -r example; do
    copy_if_missing "$example" "${example%.example}"
  done < <(find deploy/cozeloop/conf -maxdepth 1 -type f -name '*.yaml.example' | sort)

  if [[ ! -d deploy/cozeloop/conf/locales ]]; then
    cp -R deploy/cozeloop/conf/locales.example deploy/cozeloop/conf/locales
    log "created deploy/cozeloop/conf/locales from deploy/cozeloop/conf/locales.example"
  fi
}

require_file() {
  local file=$1
  if [[ ! -f "$file" ]]; then
    echo "[cozeloop] missing $file" >&2
    exit 1
  fi
}

env_value() {
  local key=$1 fallback=$2 value
  value="$(grep -E "^${key}=" "$ENV_FILE" 2>/dev/null | tail -1 | cut -d= -f2- || true)"
  if [[ -n "$value" ]]; then
    printf '%s' "$value"
  else
    printf '%s' "$fallback"
  fi
}

prepare_local_config
require_file "$ENV_FILE"
require_file deploy/cozeloop/conf/model_config.yaml
require_file deploy/cozeloop/conf/model_runtime_config.yaml

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" ps

log "UI: http://localhost:$(env_value COZE_LOOP_NGINX_PORT 18082)"
log "OpenAPI: http://localhost:$(env_value COZE_LOOP_APP_OPENAPI_PORT 19098)"
