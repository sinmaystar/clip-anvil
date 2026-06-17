# M1.x-D1 Studio Upload Assets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add MediaAsset persistence, MinIO upload, and drag-to-canvas creation of asset-backed image/video/audio nodes.

**Architecture:** Uploaded files become `media_asset` records linked to `media_node.asset_id`. The server stores files in MinIO and returns short-lived access URLs for previews; the frontend uses `FormData` and creates a matching media node only after upload succeeds.

**Tech Stack:** Go 1.26, Hertz multipart handling, minio-go v7, pgx/sqlc, React 19, TypeScript 6.

---

## File Structure

- Create `apps/server/migrations/004_add_assets.sql`: asset table and node FK.
- Create `apps/server/sqlc/queries/asset.sql`: asset queries.
- Modify `apps/server/sqlc/queries/node.sql`: asset-aware create/update.
- Generate `apps/server/internal/store/db/*`.
- Create `apps/server/internal/api/upload_handler.go`: multipart upload API.
- Create `apps/server/internal/api/upload_handler_test.go`: MIME, size, ownership tests.
- Modify `apps/server/internal/api/node_handler.go`: accept `asset_id` on create.
- Modify `apps/server/internal/api/canvas_handler.go`: include preview fields in node DTO.
- Modify `apps/server/cmd/server/main.go`: register upload route.
- Modify `apps/web/src/lib/api.ts`: upload API and multipart-safe fetch.
- Create `apps/web/src/components/FileDropZone.tsx`: drag overlay and upload orchestration.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`: attach drop zone to canvas.
- Modify `apps/web/src/main.css`: drop overlay styles.

### Task 1: Asset Schema and Queries

**Files:**
- Create: `apps/server/migrations/004_add_assets.sql`
- Create: `apps/server/sqlc/queries/asset.sql`
- Modify: `apps/server/sqlc/queries/node.sql`

- [ ] **Step 1: Create migration**

Create `apps/server/migrations/004_add_assets.sql`:

```sql
-- +goose Up
CREATE TABLE media_asset (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    type media_type NOT NULL,
    mime TEXT NOT NULL,
    storage_url TEXT NOT NULL,
    thumbnail_url TEXT,
    duration_ms INT,
    size_bytes BIGINT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_asset_workspace ON media_asset(workspace_id);

ALTER TABLE media_node
    ADD COLUMN asset_id UUID REFERENCES media_asset(id) ON DELETE SET NULL;

CREATE INDEX idx_media_node_asset ON media_node(asset_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_node_asset;
ALTER TABLE media_node DROP COLUMN IF EXISTS asset_id;
DROP INDEX IF EXISTS idx_media_asset_workspace;
DROP TABLE IF EXISTS media_asset;
```

- [ ] **Step 2: Add asset queries**

Create `apps/server/sqlc/queries/asset.sql`:

```sql
-- name: CreateMediaAsset :one
INSERT INTO media_asset (
    workspace_id,
    type,
    mime,
    storage_url,
    thumbnail_url,
    duration_ms,
    size_bytes,
    metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetMediaAssetByID :one
SELECT * FROM media_asset
WHERE id = $1;

-- name: ListMediaAssetsByWorkspace :many
SELECT * FROM media_asset
WHERE workspace_id = $1
ORDER BY created_at;
```

- [ ] **Step 3: Extend node queries for asset_id**

Update `CreateMediaNode` and `CreateMediaNodeWithID` queries to include nullable `asset_id`. Add:

```sql
-- name: UpdateMediaNodeAsset :one
UPDATE media_node
SET asset_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
```

- [ ] **Step 4: Generate code**

Run:

```bash
make sqlc-generate
```

Expected: generated asset model/query code exists and node create params include `AssetID`.

### Task 2: Backend Upload API

**Files:**
- Create: `apps/server/internal/api/upload_handler.go`
- Create: `apps/server/internal/api/upload_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Add failing upload tests**

Create upload tests with multipart helper coverage:

```go
func multipartUploadRequest(t *testing.T, workspaceID string, filename string, content []byte) (*http.Request, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("workspace_id", workspaceID))
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return httptest.NewRequest(http.MethodPost, "/api/upload", &body), writer.FormDataContentType()
}

func TestUploadHandlerAcceptsImage(t *testing.T) {
	server, token, workspaceID := newUploadHandlerTestServer(t)
	req, contentType := multipartUploadRequest(t, workspaceID, "image.png", tinyPNG)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var asset db.MediaAsset
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&asset))
	require.Equal(t, db.MediaTypeImage, asset.Type)
}

func TestUploadHandlerRejectsTextFile(t *testing.T) {
	server, token, workspaceID := newUploadHandlerTestServer(t)
	req, contentType := multipartUploadRequest(t, workspaceID, "note.txt", []byte("plain text"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusBadRequest, resp.Code)
}
```

Use multipart test requests with a small in-memory PNG byte slice:

```go
var tinyPNG = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
```

For MinIO, use the real configured development MinIO when running integration tests. Mark upload tests with a helper that skips if MinIO is unreachable:

```go
if testing.Short() {
	t.Skip("upload tests require MinIO")
}
```

- [ ] **Step 2: Implement handler structure**

Create `upload_handler.go`:

```go
type UploadHandler struct {
	queries *db.Queries
	minio   *minio.Client
}

func NewUploadHandler(queries *db.Queries, minioClient *minio.Client) *UploadHandler {
	return &UploadHandler{queries: queries, minio: minioClient}
}
```

- [ ] **Step 3: Implement MIME detection**

Add:

```go
func mediaTypeForMIME(mime string) (db.MediaType, bool) {
	switch mime {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return db.MediaTypeImage, true
	case "video/mp4", "video/quicktime", "video/webm":
		return db.MediaTypeVideo, true
	case "audio/mpeg", "audio/wav", "audio/aac", "audio/ogg":
		return db.MediaTypeAudio, true
	default:
		return "", false
	}
}
```

- [ ] **Step 4: Implement upload flow**

`Upload` method:

1. Authenticate account.
2. Read `workspace_id`.
3. Validate workspace ownership.
4. Read multipart file under key `file`.
5. Reject files over `100 << 20`.
6. Detect MIME from file header using `http.DetectContentType`.
7. Map MIME to media type.
8. Ensure bucket `workspace-{workspaceID}` exists.
9. Put object at `assets/{assetID}/{filename}`.
10. Insert `media_asset`.
11. Return asset response with `access_url`.

Use:

```go
objectName := fmt.Sprintf("assets/%s/%s", uuidString(assetID), safeFilename(header.Filename))
_, err = h.minio.PutObject(ctx, bucketName, objectName, file, header.Size, minio.PutObjectOptions{ContentType: mime})
```

- [ ] **Step 5: Register route**

In `main.go`:

```go
uploadHandler := api.NewUploadHandler(queries, minioClient)
h.POST("/api/upload", authMiddleware, uploadHandler.Upload)
```

- [ ] **Step 6: Run backend tests**

Run:

```bash
docker compose -f deploy/docker-compose.yml up -d
make server-test
```

Expected: PASS when MinIO is available.

### Task 3: Node Asset Binding

**Files:**
- Modify: `apps/server/internal/api/node_handler.go`
- Modify: `apps/server/internal/api/node_handler_test.go`
- Modify: `apps/server/internal/api/canvas_handler.go`

- [ ] **Step 1: Add failing node asset tests**

Add asset binding tests with concrete assertions:

```go
func TestNodeHandlerCreateWithAssetID(t *testing.T) {
	server, token, workspaceID := newNodeHandlerTestServer(t)
	asset := createTestAsset(t, workspaceID, db.MediaTypeImage)
	body := strings.NewReader(fmt.Sprintf(
		`{"workspace_id":%q,"node_type":"image","asset_id":%q,"title":"产品图","canvas_x":0,"canvas_y":0}`,
		workspaceID,
		asset.ID.String(),
	))
	req := httptest.NewRequest(http.MethodPost, "/api/nodes", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	var node db.MediaNode
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&node))
	require.Equal(t, asset.ID, node.AssetID)
}

func TestNodeHandlerRejectsAssetTypeMismatch(t *testing.T) {
	server, token, workspaceID := newNodeHandlerTestServer(t)
	asset := createTestAsset(t, workspaceID, db.MediaTypeImage)
	body := strings.NewReader(fmt.Sprintf(
		`{"workspace_id":%q,"node_type":"video","asset_id":%q,"title":"错误类型","canvas_x":0,"canvas_y":0}`,
		workspaceID,
		asset.ID.String(),
	))
	req := httptest.NewRequest(http.MethodPost, "/api/nodes", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusBadRequest, resp.Code)
}
```

- [ ] **Step 2: Extend create request**

In `createNodeRequest`:

```go
AssetID string `json:"asset_id"`
```

- [ ] **Step 3: Validate asset binding**

When `AssetID` is present:

- Parse UUID.
- Load asset.
- Ensure asset workspace equals request workspace.
- Ensure `asset.Type == nodeType`.
- Pass nullable asset id to sqlc create params.

- [ ] **Step 4: Add asset preview to canvas node DTO**

If current canvas returns `db.MediaNode` directly, introduce a response DTO:

```go
type canvasNodeResponse struct {
	db.MediaNode
	ThumbnailURL *string `json:"thumbnail_url,omitempty"`
}
```

For each node with `asset_id`, load asset once from a map keyed by id and set `ThumbnailURL` to asset `ThumbnailUrl` if present.

- [ ] **Step 5: Run tests**

```bash
make server-test
```

Expected: PASS.

### Task 4: Frontend Multipart API and Drop Zone

**Files:**
- Modify: `apps/web/src/lib/api.ts`
- Create: `apps/web/src/components/FileDropZone.tsx`
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Make `apiFetch` multipart-safe**

In `apiFetch`, change the content-type behavior:

```ts
if (!headers.has("Content-Type") && options.body && !(options.body instanceof FormData)) {
  headers.set("Content-Type", "application/json");
}
```

- [ ] **Step 2: Add asset DTO and upload function**

```ts
export interface MediaAsset {
  id: string;
  workspace_id: string;
  type: Exclude<MediaType, "text">;
  mime: string;
  storage_url: string;
  access_url?: string;
  thumbnail_url?: string;
  size_bytes: number;
  created_at: string;
}

export function uploadMediaAsset(workspaceId: string, file: File) {
  const form = new FormData();
  form.append("workspace_id", workspaceId);
  form.append("file", file);
  return apiFetch<MediaAsset>("/upload", {
    method: "POST",
    body: form,
  });
}
```

- [ ] **Step 3: Create FileDropZone**

Create component that accepts:

```ts
interface FileDropZoneProps {
  workspaceId: string;
  editor: Editor | null;
  onAssetNodeCreated: (node: MediaNode) => void;
}
```

On drop:

```ts
const point = editor.screenToPage({ x: event.clientX, y: event.clientY });
const assets = await Promise.allSettled(files.map((file) => uploadMediaAsset(workspaceId, file)));
for (const [index, result] of assets.entries()) {
  if (result.status !== "fulfilled") {
    continue;
  }
  const asset = result.value;
  const node = await createMediaNode({
    workspace_id: workspaceId,
    node_type: asset.type,
    asset_id: asset.id,
    title: fileNameWithoutExtension(files[index].name),
    canvas_x: point.x + index * 260,
    canvas_y: point.y,
  });
  onAssetNodeCreated(node);
}
```

- [ ] **Step 4: Mount drop zone on canvas**

In `WorkspaceDetailPage.tsx`, render inside the canvas frame:

```tsx
{id ? (
  <FileDropZone
    editor={editorRef.current}
    onAssetNodeCreated={(node) => {
      queryClient.setQueryData<CanvasPayload>(["workspace", id, "canvas"], (current) =>
        appendCanvasNode(current, node),
      );
      editorRef.current?.createShapes([nodeToShape(node)]);
    }}
    workspaceId={id}
  />
) : null}
```

- [ ] **Step 5: Add drop overlay CSS**

```css
.file-drop-zone[data-active="true"] {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  background: color-mix(in_srgb, var(--color-accent) 16%, transparent);
  color: var(--fg-primary);
  pointer-events: none;
  z-index: 20;
}
```

- [ ] **Step 6: Build**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

### Task 5: Final D1 Verification and Commit

**Files:**
- All files changed in Tasks 1-4.

- [ ] **Step 1: Run backend verification**

```bash
docker compose -f deploy/docker-compose.yml up -d
make migrate-up
make sqlc-generate
make server-test
```

Expected: PASS.

- [ ] **Step 2: Run frontend build**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/server/migrations/004_add_assets.sql apps/server/sqlc/queries/asset.sql apps/server/sqlc/queries/node.sql apps/server/internal/store/db apps/server/internal/api apps/server/cmd/server/main.go apps/web/src
git commit -m "feat: add studio media uploads"
```
