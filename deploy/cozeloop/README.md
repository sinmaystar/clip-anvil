# Coze Loop Local Sidecar

Coze Loop is an optional isolated sidecar stack. It does not share ClipAnvil PostgreSQL, Redis, MinIO, Nginx, OpenSandbox, Docker network, or Docker volumes.

The stack uses only Docker images and repository-owned Compose/bootstrap files. You do not need to clone the Coze Loop repository or build Coze Loop on the host.

## Isolation Contract

- Compose file: `deploy/docker-compose.cozeloop.yml`
- Compose project: `clipanvil-coze-loop`
- Docker network: `clipanvil-coze-loop-network`
- Docker volumes: `coze_loop_*`
- Host UI port: `18082`
- Host OpenAPI port: `19098`
- Coze Loop middleware does not publish host ports.
- ClipAnvil middleware and Coze Loop middleware are separate lifecycle domains.

## Start

```bash
./scripts/cozeloop-start.sh
```

The script creates local ignored config files from `.example` files when they are missing:

- `deploy/cozeloop/.env`
- `deploy/cozeloop/conf/*.yaml`
- `deploy/cozeloop/conf/locales/`

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

Default `./scripts/dev-start.sh` does not start Coze Loop.

## Stop With ClipAnvil

```bash
CLIPANVIL_WITH_COZELOOP=1 ./scripts/dev-stop.sh
```

Default `./scripts/dev-stop.sh` does not stop Coze Loop.

## Eino Trace Smoke

Create a Coze Loop PAT from the account settings token panel. The token is shown only once.

Set the workspace id and PAT in the ignored `deploy/cozeloop/.env` file:

```bash
CLIPANVIL_COZELOOP_WORKSPACE_ID=7654835194439925761
CLIPANVIL_COZELOOP_PAT=your-pat-token
```

Run the smoke command from the Go module:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go run ./cmd/cozeloop-trace-smoke
```

The command runs a small Eino graph and reports spans to:

```text
http://localhost:19098/v1/loop/opentelemetry/v1/traces
```

It prints the `trace_id` that you can search in Coze Loop.

## Reset Coze Loop Data Only

```bash
docker compose --env-file deploy/cozeloop/.env -f deploy/docker-compose.cozeloop.yml down -v
```

This removes only the isolated Coze Loop containers, network, and `coze_loop_*` volumes.

## Local Secrets

Local secrets stay in ignored files:

- `deploy/cozeloop/.env`
- `deploy/cozeloop/conf/model_config.yaml`
- `deploy/cozeloop/conf/model_runtime_config.yaml`

Do not put Coze Loop provider API keys in committed files.

## Upstream Attribution

The files under `deploy/cozeloop/bootstrap/` and the sample config files under `deploy/cozeloop/conf/` are vendored from `coze-dev/coze-loop` Docker Compose deployment assets and kept isolated under ClipAnvil's local deployment tree.
