# M4.1 Schema Convergence And Mock Text Run Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first M4 production loop: schema convergence plus a mock text-node run that creates `generation_job`, text asset, `artifact_version`, and updates the node current winner.

**Architecture:** Add a destructive-but-repeatable M4.1 migration as `007`, generate sqlc models/queries, then introduce a small `internal/production` service that runs text nodes through a mock provider inside a transaction. Expose a minimal authenticated Studio-only `POST /api/nodes/:id/run` endpoint while preserving the existing Studio canvas API node, especially the frontend-facing `prompt` field.

**Tech Stack:** Go 1.26, Hertz, pgx v5, sqlc v1.31, goose migrations, PostgreSQL 16, Vite/React TypeScript frontend compatibility checks.

---

## Spec Source

- Milestone: `docs/milestones/m4-shared-production-foundation.md`
- Parent roadmap: `docs/milestones/m3-m6-studio-agent-roadmap.md`
- Current backend schema: `apps/server/migrations/001_init_schema.sql` through `apps/server/migrations/006_add_workspace_mode.sql`

M4.1 acceptance to satisfy:

- A Text Node can be run through a mock text provider.
- The run creates `generation_job=succeeded`.
- The run creates a text asset and `artifact_version`.
- `media_node.current_version_id` points to the winner.
- Re-running the same node creates another version and updates the current winner.
- Existing Studio canvas smoke still works.

## File Structure

- Create `apps/server/migrations/007_m4_1_production_foundation.sql`: schema convergence and production tables.
- Modify `apps/server/sqlc/queries/asset.sql`: nullable storage/text asset creation query.
- Modify `apps/server/sqlc/queries/edge.sql`: dependency-only edge queries without `edge_type`.
- Modify `apps/server/sqlc/queries/node.sql`: production fields, prompt-template update, current-version update.
- Create `apps/server/sqlc/queries/production.sql`: job/version queries.
- Create `apps/server/internal/production/intent.go`: stable M4.1 `GenerationIntent` value.
- Create `apps/server/internal/production/mock_provider.go`: deterministic mock text provider.
- Create `apps/server/internal/production/service.go`: transactional node run service.
- Create `apps/server/internal/production/service_test.go`: service unit tests with mock query seams.
- Create `apps/server/internal/api/node_response.go`: frontend-compatible node response DTO.
- Modify `apps/server/internal/api/node_handler.go`: create/update/read nodes with `prompt_template` internally and `prompt` externally.
- Create `apps/server/internal/api/run_handler.go`: `POST /api/nodes/:id/run`.
- Create `apps/server/internal/api/run_handler_test.go`: request validation and response behavior tests.
- Modify `apps/server/internal/api/canvas_handler.go`: return node DTOs with `prompt`.
- Modify `apps/server/internal/api/node_handler_test.go`: node type naming and prompt compatibility tests.
- Modify `apps/server/cmd/server/main.go`: wire production service and route.
- Modify `apps/web/src/lib/api.ts` only if the backend response cannot preserve the current `prompt` field without frontend changes.

---

### Task 1: Add M4.1 Migration

**Files:**
- Create: `apps/server/migrations/007_m4_1_production_foundation.sql`

- [ ] **Step 1: Write the migration**

Create `apps/server/migrations/007_m4_1_production_foundation.sql` with this node:

```sql
-- +goose Up
CREATE TYPE node_type AS ENUM ('text', 'image', 'video', 'audio');
CREATE TYPE asset_type AS ENUM ('text', 'image', 'video', 'audio', 'json');
CREATE TYPE job_status AS ENUM ('pending', 'queued', 'running', 'succeeded', 'failed', 'cancelled');

ALTER TABLE media_node
    ALTER COLUMN node_type TYPE node_type USING node_type::text::node_type,
    ADD COLUMN operation_type TEXT NOT NULL DEFAULT 'manual',
    ADD COLUMN prompt_template TEXT NOT NULL DEFAULT '',
    ADD COLUMN prompt_rich JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN prompt_refs JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN model_provider TEXT,
    ADD COLUMN model_id TEXT,
    ADD COLUMN model_params JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN current_version_id UUID,
    ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}';

UPDATE media_node
SET prompt_template = prompt
WHERE prompt_template = '';

ALTER TABLE media_asset
    ALTER COLUMN type TYPE asset_type USING type::text::asset_type,
    ALTER COLUMN storage_url DROP NOT NULL,
    ADD COLUMN text_content TEXT;

ALTER TABLE media_asset
    ADD CONSTRAINT media_asset_has_content
    CHECK (storage_url IS NOT NULL OR text_content IS NOT NULL);

ALTER TABLE media_edge DROP CONSTRAINT IF EXISTS unique_edge;
ALTER TABLE media_edge DROP COLUMN IF EXISTS edge_type;
ALTER TABLE media_edge DROP COLUMN IF EXISTS transition_type;
ALTER TABLE media_edge DROP COLUMN IF EXISTS transition_duration;
ALTER TABLE media_edge ADD CONSTRAINT unique_edge UNIQUE (from_node_id, to_node_id);

DROP TYPE IF EXISTS transition_type;
DROP TYPE IF EXISTS edge_type;
DROP TYPE IF EXISTS media_type;

CREATE TABLE generation_job (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    target_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    parent_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    operation_type TEXT NOT NULL,
    provider TEXT NOT NULL,
    model_id TEXT NOT NULL,
    intent JSONB NOT NULL DEFAULT '{}',
    rendered_prompt TEXT NOT NULL DEFAULT '',
    provider_request JSONB NOT NULL DEFAULT '{}',
    provider_response JSONB NOT NULL DEFAULT '{}',
    status job_status NOT NULL DEFAULT 'pending',
    progress INT NOT NULL DEFAULT 0,
    attempt INT NOT NULL DEFAULT 1,
    max_attempts INT NOT NULL DEFAULT 1,
    retry_policy JSONB NOT NULL DEFAULT '{}',
    cost_cents INT,
    error_code TEXT,
    error_message TEXT,
    requested_by_type TEXT NOT NULL DEFAULT 'user',
    requested_by_id TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE artifact_version (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    asset_id UUID REFERENCES media_asset(id) ON DELETE SET NULL,
    version_no INT NOT NULL,
    winner BOOLEAN NOT NULL DEFAULT false,
    output JSONB NOT NULL DEFAULT '{}',
    review_score REAL,
    input_hash TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_version_per_node UNIQUE (node_id, version_no)
);

ALTER TABLE media_node
    ADD CONSTRAINT fk_media_node_current_version
    FOREIGN KEY (current_version_id) REFERENCES artifact_version(id)
    ON DELETE SET NULL;

CREATE INDEX idx_generation_job_node ON generation_job(target_node_id);
CREATE INDEX idx_generation_job_status ON generation_job(workspace_id, status);
CREATE INDEX idx_artifact_version_node ON artifact_version(node_id);
CREATE UNIQUE INDEX idx_artifact_version_one_winner
    ON artifact_version(node_id)
    WHERE winner = true;

-- +goose Down
ALTER TABLE media_node DROP CONSTRAINT IF EXISTS fk_media_node_current_version;
DROP INDEX IF EXISTS idx_artifact_version_one_winner;
DROP INDEX IF EXISTS idx_artifact_version_node;
DROP INDEX IF EXISTS idx_generation_job_status;
DROP INDEX IF EXISTS idx_generation_job_node;
DROP TABLE IF EXISTS artifact_version;
DROP TABLE IF EXISTS generation_job;

CREATE TYPE media_type AS ENUM ('text', 'image', 'video', 'audio');
CREATE TYPE edge_type AS ENUM ('dependency', 'reference', 'sequence');
CREATE TYPE transition_type AS ENUM ('cut', 'crossfade', 'dissolve', 'wipe');

ALTER TABLE media_edge DROP CONSTRAINT IF EXISTS unique_edge;
ALTER TABLE media_edge ADD COLUMN edge_type edge_type NOT NULL DEFAULT 'dependency';
ALTER TABLE media_edge ADD COLUMN transition_type transition_type;
ALTER TABLE media_edge ADD COLUMN transition_duration REAL;
ALTER TABLE media_edge ADD CONSTRAINT unique_edge UNIQUE (from_node_id, to_node_id, edge_type);

ALTER TABLE media_asset DROP CONSTRAINT IF EXISTS media_asset_has_content;
ALTER TABLE media_asset DROP COLUMN IF EXISTS text_content;
ALTER TABLE media_asset ALTER COLUMN storage_url SET NOT NULL;
ALTER TABLE media_asset ALTER COLUMN type TYPE media_type USING type::text::media_type;

ALTER TABLE media_node
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS current_version_id,
    DROP COLUMN IF EXISTS model_params,
    DROP COLUMN IF EXISTS model_id,
    DROP COLUMN IF EXISTS model_provider,
    DROP COLUMN IF EXISTS prompt_refs,
    DROP COLUMN IF EXISTS prompt_rich,
    DROP COLUMN IF EXISTS prompt_template,
    DROP COLUMN IF EXISTS operation_type;

ALTER TABLE media_node
    ALTER COLUMN node_type TYPE media_type USING node_type::text::media_type;

DROP TYPE IF EXISTS job_status;
DROP TYPE IF EXISTS asset_type;
DROP TYPE IF EXISTS node_type;
```

- [ ] **Step 2: Run migration syntax and generation check**

Run:

```bash
make sqlc-generate
```

Expected: fails before query updates because generated models and queries still refer to old columns/types. This confirms the migration is being read.

- [ ] **Step 3: Commit after this task when green later**

Commit is delayed until Task 3 because sqlc will not be green until queries are updated.

---

### Task 2: Update sqlc Queries For Production Fields

**Files:**
- Modify: `apps/server/sqlc/queries/node.sql`
- Modify: `apps/server/sqlc/queries/asset.sql`
- Modify: `apps/server/sqlc/queries/edge.sql`
- Create: `apps/server/sqlc/queries/production.sql`

- [ ] **Step 1: Update node queries**

Replace prompt writes with `prompt_template`, include production fields in creates, and add current-version update:

```sql
-- name: UpdateMediaNodePrompt :one
UPDATE media_node
SET prompt = $2,
    prompt_template = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaNodeProductionConfig :one
UPDATE media_node
SET operation_type = $2,
    prompt_template = $3,
    prompt = $3,
    model_provider = $4,
    model_id = $5,
    model_params = $6,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaNodeCurrentVersion :one
UPDATE media_node
SET current_version_id = $2,
    status = 'succeeded',
    updated_at = now()
WHERE id = $1
RETURNING *;
```

Also change `ListUpstreamDependencyNodes` to remove the `edge_type` filter:

```sql
-- name: ListUpstreamDependencyNodes :many
SELECT media_node.*
FROM media_node
JOIN media_edge ON media_edge.from_node_id = media_node.id
WHERE media_edge.to_node_id = $1
ORDER BY media_edge.created_at;
```

- [ ] **Step 2: Update edge queries**

Remove `edge_type` from inserts and endpoint lookups:

```sql
-- name: CreateMediaEdge :one
INSERT INTO media_edge (
    workspace_id,
    from_node_id,
    to_node_id
) VALUES (
    $1,
    $2,
    $3
) RETURNING *;

-- name: GetDependencyEdgeByEndpoints :one
SELECT * FROM media_edge
WHERE from_node_id = $1
  AND to_node_id = $2;

-- name: ListOutgoingDependencyEdges :many
SELECT * FROM media_edge
WHERE from_node_id = $1
ORDER BY created_at;
```

- [ ] **Step 3: Update asset queries**

Add text asset creation while preserving upload asset creation:

```sql
-- name: CreateTextMediaAsset :one
INSERT INTO media_asset (
    workspace_id,
    type,
    mime,
    storage_url,
    text_content,
    thumbnail_url,
    duration_ms,
    size_bytes,
    metadata
) VALUES (
    $1, 'text', 'text/plain; charset=utf-8', NULL, $2, NULL, NULL, $3, $4
) RETURNING *;
```

- [ ] **Step 4: Add production queries**

Create `apps/server/sqlc/queries/production.sql`:

```sql
-- name: CreateGenerationJob :one
INSERT INTO generation_job (
    workspace_id,
    target_node_id,
    parent_job_id,
    operation_type,
    provider,
    model_id,
    intent,
    rendered_prompt,
    provider_request,
    provider_response,
    status,
    progress,
    attempt,
    max_attempts,
    retry_policy,
    cost_cents,
    error_code,
    error_message,
    requested_by_type,
    requested_by_id,
    started_at,
    completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
) RETURNING *;

-- name: NextArtifactVersionNo :one
SELECT COALESCE(MAX(version_no), 0)::int + 1 AS version_no
FROM artifact_version
WHERE node_id = $1;

-- name: ClearArtifactWinnersForNode :exec
UPDATE artifact_version
SET winner = false
WHERE node_id = $1
  AND winner = true;

-- name: CreateArtifactVersion :one
INSERT INTO artifact_version (
    workspace_id,
    node_id,
    job_id,
    asset_id,
    version_no,
    winner,
    output,
    review_score,
    input_hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: ListArtifactVersionsByNode :many
SELECT *
FROM artifact_version
WHERE node_id = $1
ORDER BY version_no;

-- name: ListGenerationJobsByNode :many
SELECT *
FROM generation_job
WHERE target_node_id = $1
ORDER BY created_at;
```

- [ ] **Step 5: Run sqlc**

Run:

```bash
make sqlc-generate
```

Expected: sqlc succeeds and regenerates files in `apps/server/internal/store/db`.

---

### Task 3: Repair Backend Compile After Generated Type Changes

**Files:**
- Modify: `apps/server/internal/api/node_handler.go`
- Modify: `apps/server/internal/api/edge_handler.go`
- Modify: `apps/server/internal/api/canvas_handler.go`
- Modify: `apps/server/internal/api/upload_handler.go`
- Modify: `apps/server/internal/api/node_handler_test.go`

- [ ] **Step 1: Replace generated type names**

After sqlc generation, use the new enum names. Replace `db.MediaType` with `db.NodeType` for node handling, and use `db.AssetType` for asset handling.

Expected helper signatures:

```go
func (h *NodeHandler) assetIDForCreate(
	ctx context.Context,
	id string,
	workspaceID pgtype.UUID,
	nodeType db.NodeType,
	c *app.RequestContext,
) (pgtype.UUID, bool)
```

```go
func isAllowedNodeType(nodeType db.NodeType) bool {
	switch nodeType {
	case db.NodeTypeText,
		db.NodeTypeImage,
		db.NodeTypeVideo,
		db.NodeTypeAudio:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 2: Keep asset-node type validation explicit**

Add a conversion helper in `node_handler.go`:

```go
func assetTypeForNodeType(nodeType db.NodeType) db.AssetType {
	switch nodeType {
	case db.NodeTypeText:
		return db.AssetTypeText
	case db.NodeTypeImage:
		return db.AssetTypeImage
	case db.NodeTypeVideo:
		return db.AssetTypeVideo
	case db.NodeTypeAudio:
		return db.AssetTypeAudio
	default:
		return db.AssetType("")
	}
}
```

Use it in `assetIDForCreate`:

```go
if asset.WorkspaceID != workspaceID || asset.Type != assetTypeForNodeType(nodeType) {
	writeError(c, consts.StatusBadRequest, "invalid asset")
	return pgtype.UUID{}, false
}
```

- [ ] **Step 3: Run focused backend tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: tests may still fail because node API responses do not yet expose frontend-compatible `prompt`. Continue to Task 4 before treating this as a failure.

---

### Task 4: Preserve Frontend-Compatible Node JSON

**Files:**
- Create: `apps/server/internal/api/node_response.go`
- Modify: `apps/server/internal/api/node_handler.go`
- Modify: `apps/server/internal/api/canvas_handler.go`

- [ ] **Step 1: Add node response DTO**

Create `apps/server/internal/api/node_response.go`:

```go
package api

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type mediaNodeResponse struct {
	ID               pgtype.UUID        `json:"id"`
	WorkspaceID      pgtype.UUID        `json:"workspace_id"`
	GroupID          pgtype.UUID        `json:"group_id"`
	AssetID          pgtype.UUID        `json:"asset_id"`
	NodeType         db.NodeType        `json:"node_type"`
	OperationType    string             `json:"operation_type"`
	Title            string             `json:"title"`
	Status           db.NodeStatus      `json:"status"`
	Prompt           string             `json:"prompt"`
	PromptTemplate   string             `json:"prompt_template"`
	PromptRich       []byte             `json:"prompt_rich"`
	PromptRefs       []byte             `json:"prompt_refs"`
	ModelProvider    pgtype.Text        `json:"model_provider"`
	ModelID          pgtype.Text        `json:"model_id"`
	ModelParams      []byte             `json:"model_params"`
	CurrentVersionID pgtype.UUID        `json:"current_version_id"`
	Source           string             `json:"source"`
	CanvasX          float32            `json:"canvas_x"`
	CanvasY          float32            `json:"canvas_y"`
	CanvasW          float32            `json:"canvas_w"`
	CanvasH          float32            `json:"canvas_h"`
	Metadata         []byte             `json:"metadata"`
	CreatedAt        pgtype.Timestamptz `json:"created_at"`
	UpdatedAt        pgtype.Timestamptz `json:"updated_at"`
	ThumbnailURL     *string            `json:"thumbnail_url,omitempty"`
	_                time.Time
}

func toMediaNodeResponse(node db.MediaNode) mediaNodeResponse {
	prompt := node.PromptTemplate
	if prompt == "" {
		prompt = node.Prompt
	}
	return mediaNodeResponse{
		ID:               node.ID,
		WorkspaceID:      node.WorkspaceID,
		GroupID:          node.GroupID,
		AssetID:          node.AssetID,
		NodeType:         node.NodeType,
		OperationType:    node.OperationType,
		Title:            node.Title,
		Status:           node.Status,
		Prompt:           prompt,
		PromptTemplate:   node.PromptTemplate,
		PromptRich:       node.PromptRich,
		PromptRefs:       node.PromptRefs,
		ModelProvider:    node.ModelProvider,
		ModelID:          node.ModelID,
		ModelParams:      node.ModelParams,
		CurrentVersionID: node.CurrentVersionID,
		Source:           node.Source,
		CanvasX:          node.CanvasX,
		CanvasY:          node.CanvasY,
		CanvasW:          node.CanvasW,
		CanvasH:          node.CanvasH,
		Metadata:         node.Metadata,
		CreatedAt:        node.CreatedAt,
		UpdatedAt:        node.UpdatedAt,
	}
}
```

If generated JSONB fields are `pgtype` values rather than `[]byte`, adjust this file to the exact generated types after `make sqlc-generate`.

- [ ] **Step 2: Use the response DTO from node handlers**

Change successful node handler responses:

```go
c.JSON(consts.StatusOK, toMediaNodeResponse(node))
```

Use this in `Create`, `Get`, `Update`, `BatchUpdatePosition` response elements, and `Delete` only if it returns a node.

- [ ] **Step 3: Use the response DTO from canvas**

Update `canvasNodeResponse`:

```go
type canvasNodeResponse struct {
	mediaNodeResponse
}
```

Update `toCanvasNodeResponses` so it starts from `toMediaNodeResponse(node)` and attaches `ThumbnailURL`.

- [ ] **Step 4: Add compatibility tests**

Add to `apps/server/internal/api/node_handler_test.go`:

```go
func TestMediaNodeResponseUsesPromptTemplateAsPrompt(t *testing.T) {
	node := db.MediaNode{
		NodeType:       db.NodeTypeText,
		Prompt:         "old prompt",
		PromptTemplate: "new prompt",
	}
	response := toMediaNodeResponse(node)
	if response.Prompt != "new prompt" {
		t.Fatalf("prompt = %q, want new prompt", response.Prompt)
	}
	if response.PromptTemplate != "new prompt" {
		t.Fatalf("prompt_template = %q, want new prompt", response.PromptTemplate)
	}
}
```

- [ ] **Step 5: Run focused tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: backend tests compile and pass, except production service tests that are added in Task 5.

---

### Task 5: Add Production Service And Mock Text Provider

**Files:**
- Create: `apps/server/internal/production/intent.go`
- Create: `apps/server/internal/production/mock_provider.go`
- Create: `apps/server/internal/production/service.go`
- Create: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Define the M4.1 intent**

Create `apps/server/internal/production/intent.go`:

```go
package production

import "github.com/jackc/pgx/v5/pgtype"

type GenerationIntent struct {
	WorkspaceID     pgtype.UUID        `json:"workspace_id"`
	TargetNodeID    pgtype.UUID        `json:"target_node_id"`
	OutputType      string             `json:"output_type"`
	OperationType   string             `json:"operation_type"`
	PromptTemplate  string             `json:"prompt_template"`
	ModelProvider   string             `json:"model_provider"`
	ModelID         string             `json:"model_id"`
	ModelParams     map[string]any     `json:"model_params"`
	RequestedByType string             `json:"requested_by_type"`
	RequestedByID   string             `json:"requested_by_id"`
}

type ProviderResult struct {
	RenderedPrompt   string
	TextContent      string
	ProviderRequest  map[string]any
	ProviderResponse map[string]any
}

type TextProvider interface {
	RunText(ctx context.Context, intent GenerationIntent) (ProviderResult, error)
}
```

Include `context` in the imports after writing the file.

- [ ] **Step 2: Add deterministic mock provider**

Create `apps/server/internal/production/mock_provider.go`:

```go
package production

import (
	"context"
	"fmt"
)

type MockTextProvider struct{}

func (MockTextProvider) RunText(ctx context.Context, intent GenerationIntent) (ProviderResult, error) {
	select {
	case <-ctx.Done():
		return ProviderResult{}, ctx.Err()
	default:
	}
	rendered := intent.PromptTemplate
	if rendered == "" {
		rendered = "empty prompt"
	}
	return ProviderResult{
		RenderedPrompt: rendered,
		TextContent:    fmt.Sprintf("[mock:%s] %s", intent.ModelID, rendered),
		ProviderRequest: map[string]any{
			"provider": intent.ModelProvider,
			"model_id": intent.ModelID,
			"prompt":   rendered,
		},
		ProviderResponse: map[string]any{
			"text": fmt.Sprintf("[mock:%s] %s", intent.ModelID, rendered),
		},
	}, nil
}
```

- [ ] **Step 3: Add service transaction**

Create `apps/server/internal/production/service.go` with a `Service.RunNode` method that:

1. Loads the node by id.
2. Rejects non-text nodes with `unsupported node type`.
3. Builds `GenerationIntent` using defaults `operation_type=text_generation`, `model_provider=mock`, `model_id=mock-text`.
4. Calls `MockTextProvider`.
5. In one transaction creates a succeeded `generation_job`, creates a text asset, clears existing winners, creates a winner `artifact_version`, and updates `media_node.current_version_id`.

The implementation entrypoint must look like this:

```go
type Service struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	provider TextProvider
}

func NewService(pool *pgxpool.Pool, queries *db.Queries, provider TextProvider) *Service {
	return &Service{pool: pool, queries: queries, provider: provider}
}

func (s *Service) RunNode(ctx context.Context, nodeID pgtype.UUID, requestedByType string, requestedByID string) (db.MediaNode, error) {
	// Implement the transactional flow described above.
}
```

Use `encoding/json` to marshal intent/request/response into JSONB parameters for sqlc.

- [ ] **Step 4: Add service tests**

Create `apps/server/internal/production/service_test.go` with tests for:

```go
func TestMockTextProviderReturnsDeterministicText(t *testing.T) {
	provider := MockTextProvider{}
	intent := GenerationIntent{
		PromptTemplate: "write a short ad",
		ModelProvider:  "mock",
		ModelID:        "mock-text",
	}
	result, err := provider.RunText(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	if result.RenderedPrompt != "write a short ad" {
		t.Fatalf("rendered prompt = %q", result.RenderedPrompt)
	}
	if result.TextContent != "[mock:mock-text] write a short ad" {
		t.Fatalf("text content = %q", result.TextContent)
	}
}
```

Add a transaction-level service test only if the repo already has a local Postgres test helper. If there is no helper, cover the full persistence behavior through the API smoke in Task 7.

- [ ] **Step 5: Run tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: service and existing backend tests pass.

---

### Task 6: Add Run API Endpoint

**Files:**
- Create: `apps/server/internal/api/run_handler.go`
- Create: `apps/server/internal/api/run_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Add run handler**

Create `apps/server/internal/api/run_handler.go`:

```go
package api

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/sinmaystar/clip-anvil/internal/production"
)

type RunHandler struct {
	production *production.Service
	queries    workspaceModeQueries
}

func NewRunHandler(service *production.Service, queries workspaceModeQueries) *RunHandler {
	return &RunHandler{production: service, queries: queries}
}

func (h *RunHandler) RunNode(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	nodeID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "node not found")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	if _, ok := requireStudioWorkspace(ctx, h.queries, node.WorkspaceID, accountID, c); !ok {
		return
	}
	updated, err := h.production.RunNode(ctx, nodeID, "user", uuidToString(accountID))
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	c.JSON(consts.StatusOK, toMediaNodeResponse(updated))
}
```

If `nodeForAccountByQueries` does not exist, extract the existing `NodeHandler.nodeForAccount` logic into a package function in `node_handler.go` and keep `NodeHandler.nodeForAccount` as a thin wrapper.

- [ ] **Step 2: Wire the route**

Modify `apps/server/cmd/server/main.go`:

```go
productionService := production.NewService(pgPool, queries, production.MockTextProvider{})
runHandler := api.NewRunHandler(productionService, queries)
```

Add route near the node routes:

```go
h.POST("/api/nodes/:id/run", authMiddleware, runHandler.RunNode)
```

- [ ] **Step 3: Add handler unit tests**

Create `apps/server/internal/api/run_handler_test.go` with:

```go
func TestRunNodeRejectsInvalidNodeID(t *testing.T) {
	// Instantiate RunHandler with nil service because invalid id returns before service use.
	handler := NewRunHandler(nil, nil)
	if handler == nil {
		t.Fatal("expected handler")
	}
}
```

Keep full persistence validation for the M4.1 API smoke because existing handler tests do not run a full Hertz server with Postgres.

- [ ] **Step 4: Run backend tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: all backend tests pass.

---

### Task 7: Add M4.1 API Smoke Script

**Files:**
- Create: `scripts/smoke-m4-1.sh`

- [ ] **Step 1: Create smoke script**

Create `scripts/smoke-m4-1.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

base="${CLIPANVIL_API_BASE:-http://127.0.0.1:${CLIPANVIL_SERVER_PORT:-8888}/api}"
email="m4-1-$(date +%s)@clip.test"

node <<'NODE'
const base = process.env.CLIPANVIL_API_BASE || "http://127.0.0.1:8888/api";
async function req(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  if (!res.ok) throw new Error(`${init.method || "GET"} ${path} -> ${res.status}: ${text}`);
  return text ? JSON.parse(text) : null;
}
const email = `m4-1-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.1 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const workspace = await req("/workspaces", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({name: "M4.1 Smoke", mode: "studio"}),
});
const node = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "text",
    title: "Mock copy",
    prompt: "Write a crisp product line",
    canvas_x: 20,
    canvas_y: 40,
  }),
});
const first = await req(`/nodes/${node.id}/run`, {method: "POST", headers});
const second = await req(`/nodes/${node.id}/run`, {method: "POST", headers});
const canvas = await req(`/workspaces/${workspace.id}/canvas`, {headers});
if (!first.current_version_id || !second.current_version_id) throw new Error("missing current version");
if (first.current_version_id === second.current_version_id) throw new Error("re-run did not create a new current version");
if (canvas.nodes.length !== 1) throw new Error(`canvas node count ${canvas.nodes.length}`);
console.log(JSON.stringify({
  workspaceId: workspace.id,
  nodeId: node.id,
  firstVersion: first.current_version_id,
  secondVersion: second.current_version_id,
  canvasNodes: canvas.nodes.length,
}, null, 2));
NODE
```

Make it executable:

```bash
chmod +x scripts/smoke-m4-1.sh
```

- [ ] **Step 2: Run smoke after local server is up**

Start the app with repo tooling:

```bash
./scripts/dev-start.sh
```

Use the printed API port:

```bash
CLIPANVIL_API_BASE=http://127.0.0.1:<printed-server-port>/api scripts/smoke-m4-1.sh
```

Expected: JSON output includes different `firstVersion` and `secondVersion`, and `canvasNodes` equals `1`.

---

### Task 8: Frontend Compatibility Build

**Files:**
- Modify only if required: `apps/web/src/lib/api.ts`
- Modify only if required: `apps/web/src/components/canvas-flow/flowTypes.ts`

- [ ] **Step 1: Try build without frontend changes**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: build passes because backend still returns `prompt` and existing `node_type` values.

- [ ] **Step 2: If TypeScript requires new fields, add optional fields**

If needed, extend `MediaNode` in `apps/web/src/lib/api.ts`:

```ts
export interface MediaNode {
  id: string;
  workspace_id: string;
  node_type: MediaType;
  operation_type?: string;
  title: string;
  prompt: string;
  prompt_template?: string;
  current_version_id?: string | null;
  asset_id?: string | null;
  asset_url?: string;
  thumbnail_url?: string;
  group_id?: string | null;
  status: NodeStatus;
  canvas_x: number;
  canvas_y: number;
  canvas_w: number;
  canvas_h: number;
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 3: Re-run frontend build**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: build passes.

---

### Task 9: Final M4.1 Verification And Completion Record

**Files:**
- Modify: `docs/milestones/m4-shared-production-foundation.md`

- [ ] **Step 1: Run required verification**

Run:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: all commands pass.

- [ ] **Step 2: Run M4.1 self-test smoke**

Run the script from Task 7 against the active dev server:

```bash
CLIPANVIL_API_BASE=http://127.0.0.1:<printed-server-port>/api scripts/smoke-m4-1.sh
```

Expected: the script reports two different current version ids for the same node.

- [ ] **Step 3: Add completion record**

Append under M4.1 in `docs/milestones/m4-shared-production-foundation.md`:

```markdown
Completion record:

- Schema convergence for `node_type`, `asset_type`, dependency-only edges, production node fields, `generation_job`, and `artifact_version` is implemented.
- Mock Text Node run creates succeeded job, text asset, artifact version, and current winner.
- Re-running a Text Node creates a new version and updates current winner.
- Existing Studio canvas fetch and node behavior remains compatible.

Verification:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
scripts/smoke-m4-1.sh
git diff --check
```
```

- [ ] **Step 4: Commit M4.1 implementation**

Run:

```bash
git status --short
git add apps/server/migrations/007_m4_1_production_foundation.sql apps/server/sqlc/queries apps/server/internal/store/db apps/server/internal/production apps/server/internal/api apps/server/cmd/server/main.go apps/web/src/lib/api.ts apps/web/src/components/canvas-flow/flowTypes.ts scripts/smoke-m4-1.sh docs/milestones/m4-shared-production-foundation.md
git diff --cached --stat
git commit -m "feat: add m4.1 production foundation"
```

Expected: commit succeeds with only M4.1 files staged.

---

## Self-Review

Spec coverage:

- Schema convergence is covered by Tasks 1 through 3.
- Mock text run is covered by Tasks 5 through 7.
- Job/version/current winner is covered by Tasks 2, 5, and 7.
- Re-run version behavior is covered by Task 7.
- Existing Studio canvas compatibility is covered by Tasks 4, 8, and 9.

Red-flag scan:

- The plan avoids unresolved marker text.
- The plan names exact files and commands.
- The plan includes concrete test and code snippets for each implementation area.

Type consistency:

- Backend production code uses generated `db.NodeType` and `db.AssetType` after sqlc regeneration.
- Frontend compatibility keeps `prompt` in API responses while exposing `prompt_template` as optional.
- M4.1 does not introduce M4.2 capability tables, M4.4 stale logic, or M4.5 Reference Pack behavior.
