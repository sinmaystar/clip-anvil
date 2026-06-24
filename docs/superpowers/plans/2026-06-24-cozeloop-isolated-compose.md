# Coze Loop Isolated Compose Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Coze Loop as an optional Docker Compose-managed sidecar stack with complete runtime isolation from ClipAnvil middleware, ports, volumes, and process lifecycle.

**Architecture:** Coze Loop runs as a separate Compose project with only image-based containers and repository-owned configuration files. ClipAnvil does not share its PostgreSQL, Redis, MinIO, Nginx, OpenSandbox, Docker volumes, or Docker network with Coze Loop; ClipAnvil only talks to Coze Loop through a configured localhost OpenAPI endpoint in the future SDK integration phase.

**Tech Stack:** Docker Compose profiles, `cozedev/coze-loop` stable image, Coze Loop auxiliary images, Bash startup scripts, ClipAnvil `deploy/` conventions, optional future Go SDK integration.

---

## Isolation Requirements

- Coze Loop must never reuse ClipAnvil's PostgreSQL, Redis, MinIO, Nginx, OpenSandbox, Docker volumes, or Docker network.
- Coze Loop service names, container names, volume names, network names, and environment variables must use the `coze-loop` or `COZE_LOOP` namespace.
- Coze Loop internal middleware ports must not be published to the host.
- Only Coze Loop UI and OpenAPI are allowed to publish host ports.
- Published host ports must not overlap ClipAnvil defaults or worktree port pools:
  - ClipAnvil Nginx: `80`
  - ClipAnvil Redis: `6379`
  - ClipAnvil MinIO API/Console: `9000` / `9001`
  - ClipAnvil OpenSandbox: `8080`
  - ClipAnvil server pool: `8888-8999`
  - ClipAnvil web pool: `5173-5299`
- Coze Loop defaults:
  - UI: `18082:80`
  - OpenAPI: `19098:8888`
- Coze Loop must be opt-in. `./scripts/dev-start.sh` must keep current behavior unless `CLIPANVIL_WITH_COZELOOP=1` is set.
- Secrets must not be committed. Commit only examples and local-safe sample config.

## File Structure

- Create: `deploy/docker-compose.cozeloop.yml`
  - Defines the fully isolated Coze Loop stack.
- Create: `deploy/cozeloop/.env.example`
  - Documents image tags, isolated ports, credentials, and generated secret values.
- Create: `deploy/cozeloop/conf/model_config.yaml.example`
  - Local-safe sample model config copied by the user or script before first real model use.
- Create: `deploy/cozeloop/conf/model_runtime_config.yaml.example`
  - Local-safe sample runtime model config for providers that need separate runtime secrets.
- Create: `deploy/cozeloop/README.md`
  - Operator notes: isolation contract, startup, shutdown, reset, and troubleshooting.
- Modify: `.gitignore`
  - Ignore `deploy/cozeloop/.env`, real model config files, and any local Coze Loop data artifacts.
- Create: `scripts/cozeloop-start.sh`
  - Starts only the isolated Coze Loop Compose stack.
- Create: `scripts/cozeloop-stop.sh`
  - Stops only the isolated Coze Loop Compose stack.
- Modify: `scripts/dev-start.sh`
  - Optionally invokes `scripts/cozeloop-start.sh` when `CLIPANVIL_WITH_COZELOOP=1`.
- Modify: `scripts/dev-stop.sh`
  - Optionally invokes `scripts/cozeloop-stop.sh` when `CLIPANVIL_WITH_COZELOOP=1`; default stop behavior remains unchanged.
- Modify: `docs/engineering/deployment.md`
  - Adds the isolated Coze Loop topology, port list, and operational commands.

## Task 1: Add Isolated Compose Assets

**Files:**
- Create: `deploy/docker-compose.cozeloop.yml`
- Create: `deploy/cozeloop/.env.example`
- Create: `deploy/cozeloop/conf/model_config.yaml.example`
- Create: `deploy/cozeloop/conf/model_runtime_config.yaml.example`
- Create: `deploy/cozeloop/README.md`
- Modify: `.gitignore`

- [ ] **Step 1: Create Coze Loop env example**

Create `deploy/cozeloop/.env.example` with these defaults:

```dotenv
COMPOSE_PROJECT_NAME=clipanvil-coze-loop

COZE_LOOP_APP_IMAGE_REGISTRY=docker.io
COZE_LOOP_APP_IMAGE_REPOSITORY=cozedev
COZE_LOOP_APP_IMAGE_NAME=coze-loop
COZE_LOOP_APP_IMAGE_TAG=1.5.1
COZE_LOOP_APP_OPENAPI_PORT=19098

COZE_LOOP_NGINX_IMAGE_REGISTRY=docker.io
COZE_LOOP_NGINX_IMAGE_REPOSITORY=library
COZE_LOOP_NGINX_IMAGE_NAME=nginx
COZE_LOOP_NGINX_IMAGE_TAG=1.28.0
COZE_LOOP_NGINX_PORT=18082

COZE_LOOP_REDIS_IMAGE_REGISTRY=docker.io
COZE_LOOP_REDIS_IMAGE_REPOSITORY=library
COZE_LOOP_REDIS_IMAGE_NAME=redis
COZE_LOOP_REDIS_IMAGE_TAG=8.2.0
COZE_LOOP_REDIS_DOMAIN=coze-loop-redis
COZE_LOOP_REDIS_PORT=6379
COZE_LOOP_REDIS_PASSWORD=replace-with-local-redis-password

COZE_LOOP_MYSQL_IMAGE_REGISTRY=docker.io
COZE_LOOP_MYSQL_IMAGE_REPOSITORY=library
COZE_LOOP_MYSQL_IMAGE_NAME=mysql
COZE_LOOP_MYSQL_IMAGE_TAG=9.4.0
COZE_LOOP_MYSQL_DOMAIN=coze-loop-mysql
COZE_LOOP_MYSQL_PORT=3306
COZE_LOOP_MYSQL_USER=root
COZE_LOOP_MYSQL_PASSWORD=replace-with-local-mysql-password
COZE_LOOP_MYSQL_DATABASE=cozeloop

COZE_LOOP_CLICKHOUSE_IMAGE_REGISTRY=docker.io
COZE_LOOP_CLICKHOUSE_IMAGE_REPOSITORY=clickhouse
COZE_LOOP_CLICKHOUSE_IMAGE_NAME=clickhouse-server
COZE_LOOP_CLICKHOUSE_IMAGE_TAG=latest
COZE_LOOP_CLICKHOUSE_DOMAIN=coze-loop-clickhouse
COZE_LOOP_CLICKHOUSE_PORT=9000
COZE_LOOP_CLICKHOUSE_USER=default
COZE_LOOP_CLICKHOUSE_PASSWORD=replace-with-local-clickhouse-password
COZE_LOOP_CLICKHOUSE_DATABASE=cozeloop

COZE_LOOP_OSS_IMAGE_REGISTRY=docker.io
COZE_LOOP_OSS_IMAGE_REPOSITORY=minio
COZE_LOOP_OSS_IMAGE_NAME=minio
COZE_LOOP_OSS_IMAGE_TAG=RELEASE.2025-06-13T11-33-47Z
COZE_LOOP_OSS_PROTOCOL=http
COZE_LOOP_OSS_DOMAIN=coze-loop-minio
COZE_LOOP_OSS_PORT=9000
COZE_LOOP_OSS_REGION=us-east-1
COZE_LOOP_OSS_USER=replace-with-local-minio-user
COZE_LOOP_OSS_PASSWORD=replace-with-local-minio-password
COZE_LOOP_OSS_BUCKET=cozeloop

COZE_LOOP_RMQ_IMAGE_REGISTRY=docker.io
COZE_LOOP_RMQ_IMAGE_REPOSITORY=apache
COZE_LOOP_RMQ_IMAGE_NAME=rocketmq
COZE_LOOP_RMQ_IMAGE_TAG=5.3.3
COZE_LOOP_RMQ_NAMESRV_DOMAIN=coze-loop-rmq-namesrv
COZE_LOOP_RMQ_NAMESRV_PORT=9876

COZE_LOOP_PYTHON_FAAS_IMAGE_REGISTRY=docker.io
COZE_LOOP_PYTHON_FAAS_IMAGE_REPOSITORY=cozedev
COZE_LOOP_PYTHON_FAAS_IMAGE_NAME=coze-loop-python-faas
COZE_LOOP_PYTHON_FAAS_IMAGE_TAG=1.0.0
COZE_LOOP_PYTHON_FAAS_DOMAIN=coze-loop-python-faas
COZE_LOOP_PYTHON_FAAS_PORT=8000
COZE_LOOP_JS_FAAS_DOMAIN=coze-loop-js-faas
COZE_LOOP_JS_FAAS_PORT=8000

DENO_DIR=/tmp/.deno
DENO_NO_UPDATE_CHECK=1
DENO_V8_FLAGS=--max-old-space-size=2048
FAAS_WORKSPACE=/tmp/faas-workspace
FAAS_PORT=8000
FAAS_TIMEOUT=30000
FAAS_LANGUAGE=python
NUMPY_VERSION=>=1.24.0
PANDAS_VERSION=>=1.5.0
JSONSCHEMA_VERSION=>=4.0.0
SCIPY_VERSION=>=1.10.0
SKLEARN_VERSION=>=1.3.0
```

- [ ] **Step 2: Create local-safe sample model configs**

Create `deploy/cozeloop/conf/model_config.yaml.example`:

```yaml
models:
  - id: 1
    name: "local-sample"
    frame: "eino"
    protocol: "openai"
    protocol_config:
      base_url: ""
      api_key: "replace-with-provider-api-key"
      model: "replace-with-model-id"
    param_config:
      param_schemas:
        - name: "temperature"
          label: "temperature"
          desc: "Controls output randomness."
          type: "float"
          min: "0"
          max: "1.0"
          default_val: "0.7"
        - name: "max_tokens"
          label: "max_tokens"
          desc: "Controls maximum output tokens."
          type: "int"
          min: "1"
          max: "4096"
          default_val: "2048"
```

Create `deploy/cozeloop/conf/model_runtime_config.yaml.example`:

```yaml
need_cvt_url_to_base_64: true
qianfan_ak: "replace-if-qianfan-is-used"
qianfan_sk: "replace-if-qianfan-is-used"
```

- [ ] **Step 3: Add compose file**

Create `deploy/docker-compose.cozeloop.yml` with all Coze Loop service names prefixed and no host ports on middleware services. The compose file must mount `deploy/cozeloop/conf` into the app and use dedicated named volumes:

```yaml
name: clipanvil-coze-loop

services:
  coze-loop-app:
    image: "${COZE_LOOP_APP_IMAGE_REGISTRY}/${COZE_LOOP_APP_IMAGE_REPOSITORY}/${COZE_LOOP_APP_IMAGE_NAME}:${COZE_LOOP_APP_IMAGE_TAG}"
    restart: unless-stopped
    networks: [coze-loop-network]
    ports:
      - "${COZE_LOOP_APP_OPENAPI_PORT}:8888"
    volumes:
      - coze_loop_nginx_data:/coze-loop/resources
      - ./cozeloop/conf:/coze-loop/conf:ro
    depends_on:
      coze-loop-redis:
        condition: service_healthy
      coze-loop-mysql:
        condition: service_healthy
      coze-loop-clickhouse:
        condition: service_healthy
      coze-loop-minio:
        condition: service_healthy
      coze-loop-rmq-namesrv:
        condition: service_healthy
      coze-loop-rmq-broker:
        condition: service_healthy
      coze-loop-python-faas:
        condition: service_healthy
      coze-loop-js-faas:
        condition: service_healthy
    environment:
      COZE_LOOP_REDIS_DOMAIN: "${COZE_LOOP_REDIS_DOMAIN}"
      COZE_LOOP_REDIS_PORT: "${COZE_LOOP_REDIS_PORT}"
      COZE_LOOP_REDIS_PASSWORD: "${COZE_LOOP_REDIS_PASSWORD}"
      COZE_LOOP_MYSQL_DOMAIN: "${COZE_LOOP_MYSQL_DOMAIN}"
      COZE_LOOP_MYSQL_PORT: "${COZE_LOOP_MYSQL_PORT}"
      COZE_LOOP_MYSQL_USER: "${COZE_LOOP_MYSQL_USER}"
      COZE_LOOP_MYSQL_PASSWORD: "${COZE_LOOP_MYSQL_PASSWORD}"
      COZE_LOOP_MYSQL_DATABASE: "${COZE_LOOP_MYSQL_DATABASE}"
      COZE_LOOP_CLICKHOUSE_DOMAIN: "${COZE_LOOP_CLICKHOUSE_DOMAIN}"
      COZE_LOOP_CLICKHOUSE_PORT: "${COZE_LOOP_CLICKHOUSE_PORT}"
      COZE_LOOP_CLICKHOUSE_USER: "${COZE_LOOP_CLICKHOUSE_USER}"
      COZE_LOOP_CLICKHOUSE_PASSWORD: "${COZE_LOOP_CLICKHOUSE_PASSWORD}"
      COZE_LOOP_CLICKHOUSE_DATABASE: "${COZE_LOOP_CLICKHOUSE_DATABASE}"
      COZE_LOOP_OSS_PROTOCOL: "${COZE_LOOP_OSS_PROTOCOL}"
      COZE_LOOP_OSS_DOMAIN: "${COZE_LOOP_OSS_DOMAIN}"
      COZE_LOOP_OSS_PORT: "${COZE_LOOP_OSS_PORT}"
      COZE_LOOP_OSS_USER: "${COZE_LOOP_OSS_USER}"
      COZE_LOOP_OSS_PASSWORD: "${COZE_LOOP_OSS_PASSWORD}"
      COZE_LOOP_OSS_REGION: "${COZE_LOOP_OSS_REGION}"
      COZE_LOOP_OSS_BUCKET: "${COZE_LOOP_OSS_BUCKET}"
      COZE_LOOP_RMQ_NAMESRV_DOMAIN: "${COZE_LOOP_RMQ_NAMESRV_DOMAIN}"
      COZE_LOOP_RMQ_NAMESRV_PORT: "${COZE_LOOP_RMQ_NAMESRV_PORT}"
      COZE_LOOP_PYTHON_FAAS_DOMAIN: "${COZE_LOOP_PYTHON_FAAS_DOMAIN}"
      COZE_LOOP_PYTHON_FAAS_PORT: "${COZE_LOOP_PYTHON_FAAS_PORT}"
      COZE_LOOP_JS_FAAS_DOMAIN: "${COZE_LOOP_JS_FAAS_DOMAIN}"
      COZE_LOOP_JS_FAAS_PORT: "${COZE_LOOP_JS_FAAS_PORT}"

  coze-loop-nginx:
    image: "${COZE_LOOP_NGINX_IMAGE_REGISTRY}/${COZE_LOOP_NGINX_IMAGE_REPOSITORY}/${COZE_LOOP_NGINX_IMAGE_NAME}:${COZE_LOOP_NGINX_IMAGE_TAG}"
    restart: unless-stopped
    networks: [coze-loop-network]
    ports:
      - "${COZE_LOOP_NGINX_PORT}:80"
    volumes:
      - coze_loop_nginx_data:/usr/share/nginx/html:ro
    depends_on:
      coze-loop-app:
        condition: service_started

  coze-loop-redis:
    image: "${COZE_LOOP_REDIS_IMAGE_REGISTRY}/${COZE_LOOP_REDIS_IMAGE_REPOSITORY}/${COZE_LOOP_REDIS_IMAGE_NAME}:${COZE_LOOP_REDIS_IMAGE_TAG}"
    restart: unless-stopped
    networks: [coze-loop-network]
    volumes:
      - coze_loop_redis_data:/data
    command: ["redis-server", "--requirepass", "${COZE_LOOP_REDIS_PASSWORD}"]
    healthcheck:
      test: ["CMD-SHELL", "redis-cli -a \"$${COZE_LOOP_REDIS_PASSWORD}\" ping | grep PONG"]
      interval: 10s
      timeout: 5s
      retries: 10

  coze-loop-mysql:
    image: "${COZE_LOOP_MYSQL_IMAGE_REGISTRY}/${COZE_LOOP_MYSQL_IMAGE_REPOSITORY}/${COZE_LOOP_MYSQL_IMAGE_NAME}:${COZE_LOOP_MYSQL_IMAGE_TAG}"
    restart: unless-stopped
    networks: [coze-loop-network]
    volumes:
      - coze_loop_mysql_data:/var/lib/mysql
    environment:
      MYSQL_ROOT_PASSWORD: "${COZE_LOOP_MYSQL_PASSWORD}"
      MYSQL_DATABASE: "${COZE_LOOP_MYSQL_DATABASE}"
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h 127.0.0.1 -uroot -p\"$${MYSQL_ROOT_PASSWORD}\""]
      interval: 10s
      timeout: 5s
      retries: 20

  coze-loop-clickhouse:
    image: "${COZE_LOOP_CLICKHOUSE_IMAGE_REGISTRY}/${COZE_LOOP_CLICKHOUSE_IMAGE_REPOSITORY}/${COZE_LOOP_CLICKHOUSE_IMAGE_NAME}:${COZE_LOOP_CLICKHOUSE_IMAGE_TAG}"
    restart: unless-stopped
    networks: [coze-loop-network]
    volumes:
      - coze_loop_clickhouse_data:/var/lib/clickhouse
    environment:
      CLICKHOUSE_USER: "${COZE_LOOP_CLICKHOUSE_USER}"
      CLICKHOUSE_PASSWORD: "${COZE_LOOP_CLICKHOUSE_PASSWORD}"
      CLICKHOUSE_DB: "${COZE_LOOP_CLICKHOUSE_DATABASE}"
    healthcheck:
      test: ["CMD-SHELL", "clickhouse-client --query 'SELECT 1' --password \"$${CLICKHOUSE_PASSWORD}\""]
      interval: 10s
      timeout: 5s
      retries: 20

  coze-loop-minio:
    image: "${COZE_LOOP_OSS_IMAGE_REGISTRY}/${COZE_LOOP_OSS_IMAGE_REPOSITORY}/${COZE_LOOP_OSS_IMAGE_NAME}:${COZE_LOOP_OSS_IMAGE_TAG}"
    restart: unless-stopped
    networks: [coze-loop-network]
    volumes:
      - coze_loop_minio_data:/data
      - coze_loop_minio_config:/root/.minio
    command: server /data
    environment:
      MINIO_ROOT_USER: "${COZE_LOOP_OSS_USER}"
      MINIO_ROOT_PASSWORD: "${COZE_LOOP_OSS_PASSWORD}"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 15s
      timeout: 5s
      retries: 10

  coze-loop-rmq-namesrv:
    image: "${COZE_LOOP_RMQ_IMAGE_REGISTRY}/${COZE_LOOP_RMQ_IMAGE_REPOSITORY}/${COZE_LOOP_RMQ_IMAGE_NAME}:${COZE_LOOP_RMQ_IMAGE_TAG}"
    restart: unless-stopped
    networks: [coze-loop-network]
    volumes:
      - coze_loop_rmq_namesrv_data:/store
    command: ["sh", "mqnamesrv"]
    healthcheck:
      test: ["CMD-SHELL", "sh mqadmin clusterList -n 127.0.0.1:9876 >/dev/null 2>&1"]
      interval: 15s
      timeout: 10s
      retries: 20

  coze-loop-rmq-broker:
    image: "${COZE_LOOP_RMQ_IMAGE_REGISTRY}/${COZE_LOOP_RMQ_IMAGE_REPOSITORY}/${COZE_LOOP_RMQ_IMAGE_NAME}:${COZE_LOOP_RMQ_IMAGE_TAG}"
    restart: unless-stopped
    networks: [coze-loop-network]
    volumes:
      - coze_loop_rmq_broker_data:/store
    depends_on:
      coze-loop-rmq-namesrv:
        condition: service_healthy
    command: ["sh", "mqbroker", "-n", "coze-loop-rmq-namesrv:9876"]

  coze-loop-python-faas:
    image: "${COZE_LOOP_PYTHON_FAAS_IMAGE_REGISTRY}/${COZE_LOOP_PYTHON_FAAS_IMAGE_REPOSITORY}/${COZE_LOOP_PYTHON_FAAS_IMAGE_NAME}:${COZE_LOOP_PYTHON_FAAS_IMAGE_TAG}"
    restart: unless-stopped
    networks: [coze-loop-network]
    volumes:
      - coze_loop_python_faas_workspace:/tmp/faas-workspace
    environment:
      DENO_DIR: "${DENO_DIR}"
      DENO_NO_UPDATE_CHECK: "${DENO_NO_UPDATE_CHECK}"
      DENO_V8_FLAGS: "${DENO_V8_FLAGS}"
      FAAS_WORKSPACE: "${FAAS_WORKSPACE}"
      FAAS_PORT: "${FAAS_PORT}"
      FAAS_TIMEOUT: "${FAAS_TIMEOUT}"
      FAAS_LANGUAGE: "${FAAS_LANGUAGE}"
      NUMPY_VERSION: "${NUMPY_VERSION}"
      PANDAS_VERSION: "${PANDAS_VERSION}"
      JSONSCHEMA_VERSION: "${JSONSCHEMA_VERSION}"
      SCIPY_VERSION: "${SCIPY_VERSION}"
      SKLEARN_VERSION: "${SKLEARN_VERSION}"

  coze-loop-js-faas:
    image: "denoland/deno:1.45.5"
    restart: unless-stopped
    networks: [coze-loop-network]
    volumes:
      - coze_loop_js_faas_workspace:/tmp/faas-workspace
    environment:
      DENO_DIR: "/tmp/.deno"
      DENO_NO_UPDATE_CHECK: "1"
      FAAS_WORKSPACE: "/tmp/faas-workspace"
      FAAS_PORT: "8000"
      FAAS_TIMEOUT: "30000"
      FAAS_LANGUAGE: "javascript"

volumes:
  coze_loop_nginx_data:
  coze_loop_redis_data:
  coze_loop_mysql_data:
  coze_loop_clickhouse_data:
  coze_loop_minio_data:
  coze_loop_minio_config:
  coze_loop_rmq_namesrv_data:
  coze_loop_rmq_broker_data:
  coze_loop_python_faas_workspace:
  coze_loop_js_faas_workspace:

networks:
  coze-loop-network:
    name: clipanvil-coze-loop-network
```

During implementation, compare this file against the upstream Coze Loop stable compose for required bootstrap entrypoints and init jobs. If the upstream image still requires mounted bootstrap scripts for startup, vendor only the deployment bootstrap files under `deploy/cozeloop/bootstrap/` and keep them isolated under the same namespace. Do not ask the user to clone the Coze Loop repository on the host.

- [ ] **Step 4: Ignore real local Coze Loop config**

Add these entries to `.gitignore`:

```gitignore
deploy/cozeloop/.env
deploy/cozeloop/conf/model_config.yaml
deploy/cozeloop/conf/model_runtime_config.yaml
deploy/cozeloop/data/
```

- [ ] **Step 5: Verify compose syntax**

Run:

```bash
cp deploy/cozeloop/.env.example deploy/cozeloop/.env
cp deploy/cozeloop/conf/model_config.yaml.example deploy/cozeloop/conf/model_config.yaml
cp deploy/cozeloop/conf/model_runtime_config.yaml.example deploy/cozeloop/conf/model_runtime_config.yaml
docker compose --env-file deploy/cozeloop/.env -f deploy/docker-compose.cozeloop.yml config >/tmp/clipanvil-cozeloop-compose.yml
```

Expected: command succeeds and rendered config contains `clipanvil-coze-loop-network`, `coze_loop_minio_data`, `19098:8888`, and `18082:80`.

## Task 2: Add Isolated Startup And Stop Scripts

**Files:**
- Create: `scripts/cozeloop-start.sh`
- Create: `scripts/cozeloop-stop.sh`

- [ ] **Step 1: Create startup script**

Create `scripts/cozeloop-start.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${CLIPANVIL_COZELOOP_ENV_FILE:-deploy/cozeloop/.env}"
COMPOSE_FILE="deploy/docker-compose.cozeloop.yml"

fail() {
  echo "[cozeloop] $1" >&2
  exit 1
}

if [[ ! -f "$ENV_FILE" ]]; then
  fail "missing $ENV_FILE; copy deploy/cozeloop/.env.example and replace local secrets"
fi

if [[ ! -f deploy/cozeloop/conf/model_config.yaml ]]; then
  fail "missing deploy/cozeloop/conf/model_config.yaml; copy the .example file and set provider config"
fi

if [[ ! -f deploy/cozeloop/conf/model_runtime_config.yaml ]]; then
  fail "missing deploy/cozeloop/conf/model_runtime_config.yaml; copy the .example file"
fi

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" ps

echo "[cozeloop] UI: http://localhost:${COZE_LOOP_NGINX_PORT:-18082}"
echo "[cozeloop] OpenAPI: http://localhost:${COZE_LOOP_APP_OPENAPI_PORT:-19098}"
```

- [ ] **Step 2: Create stop script**

Create `scripts/cozeloop-stop.sh`:

```bash
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
```

- [ ] **Step 3: Make scripts executable**

Run:

```bash
chmod +x scripts/cozeloop-start.sh scripts/cozeloop-stop.sh
```

- [ ] **Step 4: Verify scripts parse**

Run:

```bash
bash -n scripts/cozeloop-start.sh
bash -n scripts/cozeloop-stop.sh
```

Expected: both commands succeed.

## Task 3: Make dev-start Optional Without Changing Defaults

**Files:**
- Modify: `scripts/dev-start.sh`
- Modify: `scripts/dev-stop.sh`

- [ ] **Step 1: Add optional start hook**

In `scripts/dev-start.sh`, after the existing middleware startup block and before backend build, add:

```bash
if [[ "${CLIPANVIL_WITH_COZELOOP:-}" == "1" ]]; then
  warn "正在启动隔离 Coze Loop stack..."
  ./scripts/cozeloop-start.sh
  log "隔离 Coze Loop stack 已启动"
fi
```

Do not make Coze Loop part of the default middleware check. Default `./scripts/dev-start.sh` must continue to start only ClipAnvil middleware, Go server, and Vite.

- [ ] **Step 2: Add optional stop hook**

In `scripts/dev-stop.sh`, after existing ClipAnvil frontend/backend process cleanup succeeds, add:

```bash
if [[ "${CLIPANVIL_WITH_COZELOOP:-}" == "1" ]]; then
  echo "--- Coze Loop ---"
  ./scripts/cozeloop-stop.sh
fi
```

Default `./scripts/dev-stop.sh` must not stop Coze Loop because Coze Loop is a shared optional sidecar stack.

- [ ] **Step 3: Verify shell syntax**

Run:

```bash
bash -n scripts/dev-start.sh
bash -n scripts/dev-stop.sh
bash -n scripts/cozeloop-start.sh
bash -n scripts/cozeloop-stop.sh
```

Expected: all commands succeed.

## Task 4: Document Operator Workflow

**Files:**
- Create: `deploy/cozeloop/README.md`
- Modify: `docs/engineering/deployment.md`

- [ ] **Step 1: Add Coze Loop local README**

Create `deploy/cozeloop/README.md`:

```markdown
# Coze Loop Local Sidecar

Coze Loop is an optional isolated sidecar stack. It does not share ClipAnvil PostgreSQL, Redis, MinIO, Nginx, OpenSandbox, Docker network, or Docker volumes.

## First Run

```bash
cp deploy/cozeloop/.env.example deploy/cozeloop/.env
cp deploy/cozeloop/conf/model_config.yaml.example deploy/cozeloop/conf/model_config.yaml
cp deploy/cozeloop/conf/model_runtime_config.yaml.example deploy/cozeloop/conf/model_runtime_config.yaml
```

Replace local secrets in `deploy/cozeloop/.env` and provider settings in `deploy/cozeloop/conf/model_config.yaml`.

## Start

```bash
./scripts/cozeloop-start.sh
```

UI: `http://localhost:18082`
OpenAPI: `http://localhost:19098`

## Stop

```bash
./scripts/cozeloop-stop.sh
```

## Start With ClipAnvil

```bash
CLIPANVIL_WITH_COZELOOP=1 ./scripts/dev-start.sh
```

## Reset Coze Loop Data Only

```bash
docker compose --env-file deploy/cozeloop/.env -f deploy/docker-compose.cozeloop.yml down -v
```

This removes only `coze_loop_*` volumes.
```

- [ ] **Step 2: Update deployment doc**

In `docs/engineering/deployment.md`, add a "Coze Loop Sidecar" section after the current container topology. Include:

- It is optional and disabled by default.
- It is fully isolated from ClipAnvil middleware.
- Ports are `18082` for UI and `19098` for OpenAPI.
- Middleware services do not publish host ports.
- Start commands:

```bash
./scripts/cozeloop-start.sh
CLIPANVIL_WITH_COZELOOP=1 ./scripts/dev-start.sh
```

- Stop command:

```bash
./scripts/cozeloop-stop.sh
```

## Task 5: Isolation Verification

**Files:**
- All files from previous tasks.

- [ ] **Step 1: Render compose config**

Run:

```bash
docker compose --env-file deploy/cozeloop/.env -f deploy/docker-compose.cozeloop.yml config >/tmp/clipanvil-cozeloop-compose.yml
```

Expected: succeeds.

- [ ] **Step 2: Check no forbidden host ports**

Run:

```bash
rg -n '"?(80|6379|8080|8888|9000|9001):(80|6379|8080|8888|9000|9001)"?' /tmp/clipanvil-cozeloop-compose.yml
```

Expected: no matches. The rendered config may contain container-internal ports, but must not publish ClipAnvil host ports.

- [ ] **Step 3: Check namespacing**

Run:

```bash
rg -n '(^  [a-z].*:|name:|container_name:|source:|target:)' /tmp/clipanvil-cozeloop-compose.yml
```

Expected: service names use `coze-loop-*`, volumes use `coze_loop_*`, and network uses `clipanvil-coze-loop-network`.

- [ ] **Step 4: Confirm ClipAnvil default env is unchanged**

Run:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Expected: output contains only existing ClipAnvil exports. It must not require `deploy/cozeloop/.env` and must not start or inspect Coze Loop.

- [ ] **Step 5: Run static checks**

Run:

```bash
git diff --check
```

Expected: no whitespace errors.

## Future Integration Boundary

Coze Loop SDK integration is intentionally out of this isolated compose plan. A separate SDK integration plan may add optional Go tracing behind:

```dotenv
COZELOOP_ENABLED=1
COZELOOP_API_BASE_URL=http://localhost:19098/
COZELOOP_WORKSPACE_ID=replace-with-coze-loop-workspace-id
COZELOOP_API_TOKEN=replace-with-coze-loop-pat
```

The first trace points should be the shared production boundary and Agent worker boundary, not frontend UI code:

- `apps/server/internal/production/service.go`
- `apps/server/internal/production/runner.go`
- `apps/server/internal/agent/worker/executor.go`
- `apps/server/internal/agent/producer/executor.go`

ClipAnvil remains the source of truth for `generation_job`, `artifact_version`, workspace state, media assets, and Agent runtime state. Coze Loop stores only its own observability/evaluation data inside its isolated stack.
