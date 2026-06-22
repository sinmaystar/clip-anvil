#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

sanitize() {
  tr -cs '[:alnum:]_.-' '-' | sed 's/^-//; s/-$//'
}

default_profile_name() {
  local base branch checksum suffix
  base="$(basename "$ROOT_DIR" | sanitize)"
  branch="$(git -C "$ROOT_DIR" branch --show-current 2>/dev/null | sanitize)"
  if [[ -z "$branch" ]]; then
    branch="detached"
  fi
  checksum="$(printf '%s' "$ROOT_DIR" | cksum)"
  checksum="${checksum%% *}"
  suffix="$(printf '%04d' "$((checksum % 10000))")"
  echo "$base-$branch-$suffix"
}

port_in_use() {
  local port=$1
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi
  (echo >"/dev/tcp/127.0.0.1/$port") >/dev/null 2>&1
}

find_free_port() {
  local port=$1 max_port=$2
  while (( port <= max_port )); do
    if ! port_in_use "$port"; then
      echo "$port"
      return 0
    fi
    port=$((port + 1))
  done
  return 1
}

shell_quote() {
  printf '%q' "$1"
}

is_pid_running() {
  local pid=${1:-}
  [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null
}

default_name="$(default_profile_name)"
DEV_NAME="${CLIPANVIL_DEV_NAME:-$default_name}"
SERVER_HOST="${CLIPANVIL_SERVER_HOST:-localhost}"

if [[ -n "${CLIPANVIL_SERVER_PORT:-}" ]]; then
  SERVER_PORT="$CLIPANVIL_SERVER_PORT"
  if port_in_use "$SERVER_PORT"; then
    echo "CLIPANVIL_SERVER_PORT=$SERVER_PORT 已被占用，请换一个端口或先停止对应 profile。" >&2
    exit 1
  fi
else
  SERVER_PORT="$(find_free_port 8888 8999)" || {
    echo "未找到可用后端端口（8888-8999）。" >&2
    exit 1
  }
fi

if [[ -n "${CLIPANVIL_WEB_PORT:-}" ]]; then
  WEB_PORT="$CLIPANVIL_WEB_PORT"
  if port_in_use "$WEB_PORT"; then
    echo "CLIPANVIL_WEB_PORT=$WEB_PORT 已被占用，请换一个端口或先停止对应 profile。" >&2
    exit 1
  fi
else
  WEB_PORT="$(find_free_port 5173 5299)" || {
    echo "未找到可用前端端口（5173-5299）。" >&2
    exit 1
  }
fi

PUBLIC_BASE_URL="${CLIPANVIL_PUBLIC_BASE_URL:-http://localhost:$WEB_PORT}"
PID_DIR="$ROOT_DIR/.dev-pids/$DEV_NAME"
SERVER_LOG="${CLIPANVIL_SERVER_LOG:-/tmp/clipanvil-$DEV_NAME-server.log}"
WEB_LOG="${CLIPANVIL_WEB_LOG:-/tmp/clipanvil-$DEV_NAME-web.log}"

if [[ "${CLIPANVIL_PRINT_DEV_ENV:-}" == "1" ]]; then
  echo "export CLIPANVIL_DEV_NAME=$(shell_quote "$DEV_NAME")"
  echo "export CLIPANVIL_SERVER_PORT=$(shell_quote "$SERVER_PORT")"
  echo "export CLIPANVIL_WEB_PORT=$(shell_quote "$WEB_PORT")"
  echo "export CLIPANVIL_SERVER_HOST=$(shell_quote "$SERVER_HOST")"
  echo "export CLIPANVIL_PUBLIC_BASE_URL=$(shell_quote "$PUBLIC_BASE_URL")"
  echo "export CLIPANVIL_SERVER_LOG=$(shell_quote "$SERVER_LOG")"
  echo "export CLIPANVIL_WEB_LOG=$(shell_quote "$WEB_LOG")"
  exit 0
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
fail() { echo -e "${RED}[✗]${NC} $1"; exit 1; }

wait_for() {
  local url=$1 max=$2 desc=$3
  for i in $(seq 1 "$max"); do
    if curl -s -o /dev/null -w '' "$url" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  fail "$desc 启动超时（${max}s）"
}

start_detached() {
  local pid_file=$1 pgid_file=$2 log_file=$3
  shift 3
  python3 - "$pid_file" "$pgid_file" "$log_file" "$@" <<'PY'
import os
import subprocess
import sys

pid_file, pgid_file, log_file, *cmd = sys.argv[1:]
with open(log_file, "wb") as log:
    proc = subprocess.Popen(
        cmd,
        stdin=subprocess.DEVNULL,
        stdout=log,
        stderr=subprocess.STDOUT,
        close_fds=True,
        start_new_session=True,
    )
with open(pid_file, "w", encoding="utf-8") as file:
    file.write(f"{proc.pid}\n")
with open(pgid_file, "w", encoding="utf-8") as file:
    file.write(f"{os.getpgid(proc.pid)}\n")
PY
}

cleanup_stale_pid_dir() {
  local server_pid web_pid
  server_pid="$(cat "$PID_DIR/server.pid" 2>/dev/null || true)"
  web_pid="$(cat "$PID_DIR/web.pid" 2>/dev/null || true)"
  if is_pid_running "$server_pid" || is_pid_running "$web_pid"; then
    fail "开发环境 profile '$DEV_NAME' 已在运行，先执行 CLIPANVIL_DEV_NAME=$DEV_NAME ./scripts/dev-stop.sh"
  fi
  rm -rf "$PID_DIR"
}

# 检查是否已在运行
if [[ -d "$PID_DIR" ]]; then
  cleanup_stale_pid_dir
fi

mkdir -p "$PID_DIR"
echo "$SERVER_PORT" > "$PID_DIR/server.port"
echo "$WEB_PORT" > "$PID_DIR/web.port"
echo "$SERVER_HOST" > "$PID_DIR/server.host"

echo ""
echo "========================================="
echo "  ClipAnvil 影砧 - 开发环境启动 ($DEV_NAME)"
echo "========================================="
echo ""

# 1. 中间件
echo "--- 中间件 ---"
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'deploy_postgres_1'; then
  log "中间件容器已在运行"
else
  warn "正在启动中间件容器..."
  docker compose -f deploy/docker-compose.yml up -d
  sleep 3
  log "中间件容器已启动"
fi

wait_for "http://localhost:9000/minio/health/live" 15 "MinIO"

# 2. 后端
echo ""
echo "--- 后端 ---"
cd "$ROOT_DIR/apps/server"
go build -o "$ROOT_DIR/bin/server" ./cmd/server
cd "$ROOT_DIR"
CLIPANVIL_SERVER_PORT="$SERVER_PORT" \
  start_detached "$PID_DIR/server.pid" "$PID_DIR/server.pgid" "$SERVER_LOG" ./bin/server

wait_for "http://$SERVER_HOST:$SERVER_PORT/api/health" 20 "Go server"
log "后端已启动 (PID: $(cat "$PID_DIR/server.pid"), PGID: $(cat "$PID_DIR/server.pgid"), port: $SERVER_PORT)"

# 3. 前端
echo ""
echo "--- 前端 ---"
CLIPANVIL_WEB_PORT="$WEB_PORT" \
CLIPANVIL_SERVER_PORT="$SERVER_PORT" \
CLIPANVIL_SERVER_HOST="$SERVER_HOST" \
  start_detached "$PID_DIR/web.pid" "$PID_DIR/web.pgid" "$WEB_LOG" pnpm --filter @clip-anvil/web dev

wait_for "http://localhost:$WEB_PORT" 15 "Vite dev server"
log "前端已启动 (PID: $(cat "$PID_DIR/web.pid"), PGID: $(cat "$PID_DIR/web.pgid"), port: $WEB_PORT)"

# 4. 健康检查
echo ""
echo "--- 健康检查 ---"
HEALTH=$(curl -s "http://$SERVER_HOST:$SERVER_PORT/api/health")
STATUS=$(echo "$HEALTH" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])" 2>/dev/null || echo "fail")

if [[ "$STATUS" == "ok" ]]; then
  log "健康检查通过: $HEALTH"
else
  fail "健康检查失败: $HEALTH"
fi

echo ""
echo "========================================="
echo -e "  ${GREEN}全部启动成功！${NC}"
echo ""
echo "  浏览器打开: $PUBLIC_BASE_URL"
echo "  API 地址:   http://$SERVER_HOST:$SERVER_PORT/api/health"
echo "  Profile:    $DEV_NAME"
echo ""
echo "  后端日志:   tail -f $SERVER_LOG"
echo "  前端日志:   tail -f $WEB_LOG"
echo ""
echo "  停止:       CLIPANVIL_DEV_NAME=$DEV_NAME ./scripts/dev-stop.sh"
echo "========================================="
