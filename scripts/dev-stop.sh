#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

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

default_name="$(default_profile_name)"
DEV_NAME="${CLIPANVIL_DEV_NAME:-$default_name}"
PID_DIR="$ROOT_DIR/.dev-pids/$DEV_NAME"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }

is_pid_running() {
  local pid=${1:-}
  [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null
}

is_process_group_running() {
  local pgid=${1:-}
  [[ "$pgid" =~ ^[0-9]+$ ]] && (( pgid > 1 )) && kill -0 "-$pgid" 2>/dev/null
}

wait_for_pid_exit() {
  local pid=$1
  for _ in $(seq 1 20); do
    if ! is_pid_running "$pid"; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

wait_for_group_exit() {
  local pgid=$1
  for _ in $(seq 1 20); do
    if ! is_process_group_running "$pgid"; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

kill_pid_tree() {
  local pid=$1 label=$2 child
  if ! is_pid_running "$pid"; then
    return 0
  fi
  while read -r child; do
    if [[ -n "$child" ]]; then
      kill_pid_tree "$child" "$label child"
    fi
  done < <(pgrep -P "$pid" 2>/dev/null || true)
  kill "$pid" 2>/dev/null || true
  if ! wait_for_pid_exit "$pid"; then
    kill -KILL "$pid" 2>/dev/null || true
  fi
}

kill_recorded_process() {
  local label=$1 pid_file=$2 pgid_file=$3
  local pid pgid
  pid="$(cat "$pid_file" 2>/dev/null || true)"
  pgid="$(cat "$pgid_file" 2>/dev/null || true)"

  if is_process_group_running "$pgid"; then
    kill "-$pgid" 2>/dev/null || true
    if ! wait_for_group_exit "$pgid"; then
      kill -KILL "-$pgid" 2>/dev/null || true
    fi
    log "${label}已停止 (PID: ${pid:-unknown}, PGID: $pgid)"
  elif is_pid_running "$pid"; then
    kill_pid_tree "$pid" "$label"
    log "${label}已停止 (PID: $pid)"
  elif [[ -f "$pid_file" ]]; then
    warn "${label}进程已不存在 (PID: ${pid:-unknown})"
  else
    warn "未找到${label} PID 文件"
  fi

  rm -f "$pid_file" "$pgid_file"
}

process_cwd() {
  local pid=$1
  lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1
}

process_is_in_repo() {
  local pid=$1 cwd
  cwd="$(process_cwd "$pid")"
  [[ "$cwd" == "$ROOT_DIR"* ]]
}

pids_for_port() {
  local port=$1
  lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true
}

wait_for_port_free() {
  local port=$1
  for _ in $(seq 1 20); do
    if [[ -z "$(pids_for_port "$port")" ]]; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

cleanup_recorded_port() {
  local label=$1 port_file=$2 port pid pids
  port="$(cat "$port_file" 2>/dev/null || true)"
  if [[ ! "$port" =~ ^[0-9]+$ ]]; then
    rm -f "$port_file"
    return 0
  fi

  pids="$(pids_for_port "$port")"
  if [[ -z "$pids" ]]; then
    log "${label}端口已释放 (port: $port)"
    rm -f "$port_file"
    return 0
  fi

  while read -r pid; do
    if [[ -z "$pid" ]]; then
      continue
    fi
    if process_is_in_repo "$pid"; then
      warn "${label}端口仍被当前 repo 进程占用，继续清理 (port: $port, PID: $pid)"
      kill_pid_tree "$pid" "$label listener"
    else
      warn "${label}端口被其他目录进程占用，未清理 (port: $port, PID: $pid, cwd: $(process_cwd "$pid"))"
    fi
  done <<< "$pids"

  if wait_for_port_free "$port"; then
    log "${label}端口已释放 (port: $port)"
  else
    warn "${label}端口仍未释放 (port: $port, PIDs: $(pids_for_port "$port" | tr '\n' ' '))"
  fi
  rm -f "$port_file"
}

echo ""
echo "========================================="
echo "  ClipAnvil 影砧 - 开发环境停止 ($DEV_NAME)"
echo "========================================="
echo ""

kill_recorded_process "后端" "$PID_DIR/server.pid" "$PID_DIR/server.pgid"
kill_recorded_process "前端" "$PID_DIR/web.pid" "$PID_DIR/web.pgid"

cleanup_recorded_port "后端" "$PID_DIR/server.port"
cleanup_recorded_port "前端" "$PID_DIR/web.port"

rm -f "$PID_DIR/server.host"

rmdir "$PID_DIR" 2>/dev/null || true
rmdir "$ROOT_DIR/.dev-pids" 2>/dev/null || true

echo ""
echo -e "${GREEN}前后端已停止。${NC}中间件容器保持运行。"
echo "如需停止中间件: docker compose -f deploy/docker-compose.yml down"
echo ""
