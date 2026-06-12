# 部署与运维

## 容器拓扑

```
                ┌──────────┐
   浏览器  ───▶ │  nginx   │ ── 静态前端（prod）/ 代理 Vite dev server（dev）
                │  :80     │ ── /api       ──▶ server :8888
                │          │ ── /ws/chat   ──▶ server :8888 (WebSocket)
                │          │ ── /ws/canvas ──▶ server :8888 (WebSocket)
                └──────────┘

                  server (Go, 宿主机运行)
                     │
   ┌─────────────────┼──────────────────┐
   ▼                 ▼                  ▼
postgres:16       redis:7         minio:latest
:5432             :6379           :9000 (API) / :9001 (Console)
```

## 端口清单

| 服务 | 端口 | 说明 |
|---|---|---|
| Nginx | 80 | 统一入口，反代所有请求 |
| Go Server | 8888 | 后端 API + WebSocket（宿主机运行） |
| Vite Dev | 5173 | 前端开发服务器（宿主机运行，仅 dev） |
| PostgreSQL | 5432 | 数据库 |
| Redis | 6379 | 缓存 |
| MinIO API | 9000 | 对象存储 API |
| MinIO Console | 9001 | MinIO Web 管理界面 |

## 环境变量

在 `deploy/.env` 中配置（从 `deploy/.env.example` 复制）：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PG_PASSWORD` | `clipanvil_dev` | PostgreSQL 密码 |
| `MINIO_USER` | `clipanvil` | MinIO 管理员用户名 |
| `MINIO_PASSWORD` | `clipanvil_dev` | MinIO 管理员密码 |

后端连接信息在 `apps/server/config.yaml` 中配置。

## 快速启动

### 本地开发（推荐）

后端在宿主机运行，改代码后 `go build` 即可重启，无需打镜像。

```bash
./scripts/dev-start.sh     # 拉起中间件容器 → 编译启动后端 → 启动前端 → 健康检查
./scripts/dev-stop.sh      # 停止前后端进程，中间件容器保持运行
```

### 容器化部署

后端也跑在容器里，用于验证镜像构建和容器间通信。

```bash
# 启动（首次或代码变更后需 --build）
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.server.yml up -d --build

# 停止
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.server.yml down
```

容器化模式下：
- 后端使用 `deploy/config-container.yaml` 配置，通过容器名（postgres/redis/minio）连接中间件
- nginx 使用 `deploy/nginx/full.conf`，API 请求代理到 `backend:8888` 容器
- 前端仍在宿主机运行（Vite dev server）

### 手动操作

```bash
# 中间件
docker compose -f deploy/docker-compose.yml up -d     # 启动
docker compose -f deploy/docker-compose.yml down       # 停止
docker compose -f deploy/docker-compose.yml ps         # 查看状态

# 后端
make server-dev                                        # 启动（从 apps/server/ 目录 go run）

# 前端
pnpm --filter @clip-anvil/web dev                      # 启动 Vite dev server
```

## 容器管理

当前使用 **docker**（后期切换 Docker），compose 文件不使用 Docker 专有特性。

### 常用命令

```bash
# 查看容器日志
docker logs deploy_postgres_1
docker logs -f deploy_nginx_1     # 持续跟踪

# 进入容器
docker exec -it deploy_postgres_1 psql -U clipanvil
docker exec -it deploy_redis_1 redis-cli

# 健康检查
docker exec deploy_postgres_1 pg_isready -U clipanvil
docker exec deploy_redis_1 redis-cli ping
curl -s http://localhost:9000/minio/health/live

# 重建容器（配置变更后）
docker compose -f deploy/docker-compose.yml up -d --force-recreate
```

### 数据持久化

| 卷 | 用途 |
|---|---|
| `pg_data` | PostgreSQL 数据 |
| `minio_data` | MinIO 对象存储数据 |

清除数据：`docker compose -f deploy/docker-compose.yml down -v`

## Nginx 配置

| 文件 | 用途 | API 代理目标 |
|---|---|---|
| `deploy/nginx/dev.conf` | 本地开发 | `host.docker.internal:8888`（宿主机） |
| `deploy/nginx/full.conf` | 容器化部署 | `backend:8888`（容器） |
| `deploy/nginx/default.conf` | 生产模式 | `backend:8888`（容器） |

dev/full 模式下前端均代理到宿主机 Vite(:5173)。

## docker 注意事项

- 镜像拉取：如果 Docker Hub 不通，需在 docker machine 中配置镜像加速（如 `docker.m.daocloud.io`）
- 端口 80：rootless 模式下需设置 `net.ipv4.ip_unprivileged_port_start=80`
- Socket 路径：OpenSandbox（M3）需要挂载容器 socket，docker 路径为 `$XDG_RUNTIME_DIR/docker/docker.sock`
