#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PID_DIR="$ROOT_DIR/.dev-pids"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }

echo ""
echo "========================================="
echo "  ClipAnvil 影砧 - 开发环境停止"
echo "========================================="
echo ""

# 停止后端
if [[ -f "$PID_DIR/server.pid" ]]; then
  PID=$(cat "$PID_DIR/server.pid")
  if kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    log "后端已停止 (PID: $PID)"
  else
    warn "后端进程已不存在 (PID: $PID)"
  fi
  rm -f "$PID_DIR/server.pid"
else
  warn "未找到后端 PID 文件"
fi

# 停止前端
if [[ -f "$PID_DIR/web.pid" ]]; then
  PID=$(cat "$PID_DIR/web.pid")
  if kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    log "前端已停止 (PID: $PID)"
  else
    warn "前端进程已不存在 (PID: $PID)"
  fi
  rm -f "$PID_DIR/web.pid"
else
  warn "未找到前端 PID 文件"
fi

rmdir "$PID_DIR" 2>/dev/null || true

echo ""
echo -e "${GREEN}前后端已停止。${NC}中间件容器保持运行。"
echo "如需停止中间件: docker compose -f deploy/docker-compose.yml down"
echo ""
