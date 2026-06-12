#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PID_DIR="$ROOT_DIR/.dev-pids"
cd "$ROOT_DIR"

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

# 检查是否已在运行
if [[ -d "$PID_DIR" ]] && kill -0 "$(cat "$PID_DIR/server.pid" 2>/dev/null)" 2>/dev/null; then
  fail "开发环境已在运行，先执行 ./scripts/dev-stop.sh"
fi

mkdir -p "$PID_DIR"

echo ""
echo "========================================="
echo "  ClipAnvil 影砧 - 开发环境启动"
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
./bin/server > /tmp/clipanvil-server.log 2>&1 &
echo $! > "$PID_DIR/server.pid"

wait_for "http://localhost:8888/api/health" 20 "Go server"
log "后端已启动 (PID: $(cat "$PID_DIR/server.pid"))"

# 3. 前端
echo ""
echo "--- 前端 ---"
pnpm --filter @clip-anvil/web dev > /tmp/clipanvil-web.log 2>&1 &
echo $! > "$PID_DIR/web.pid"

wait_for "http://localhost:5173" 15 "Vite dev server"
log "前端已启动 (PID: $(cat "$PID_DIR/web.pid"))"

# 4. 健康检查
echo ""
echo "--- 健康检查 ---"
HEALTH=$(curl -s http://localhost/api/health)
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
echo "  浏览器打开: http://localhost"
echo "  API 地址:   http://localhost/api/health"
echo ""
echo "  后端日志:   tail -f /tmp/clipanvil-server.log"
echo "  前端日志:   tail -f /tmp/clipanvil-web.log"
echo ""
echo "  停止:       ./scripts/dev-stop.sh"
echo "========================================="
