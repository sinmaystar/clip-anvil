# M6.4B Agent Read-Only Canvas Detail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render Agent image attachments as thumbnails and replace the Agent text-only canvas with a Studio-derived read-only canvas plus read-only node detail.

**Architecture:** Keep persisted Agent message content stable by storing asset/node IDs only, and hydrate transient preview URLs in API responses. Extract a focused read-only canvas viewport that reuses Studio view-model projection and `MediaFlowNode`. Add a dedicated Agent node detail drawer that reads existing canvas and production-state data but exposes no mutation handlers.

**Tech Stack:** Go 1.26 + Hertz + pgx/sqlc backend, React 19 + TypeScript 6 + TanStack Query + React Flow frontend, existing `agent_message` UI protocol and Studio canvas helpers.

---

## File Map

- Modify `apps/server/internal/agent/uimessage/blocks.go`: add optional `url` and `thumbnail_url` fields to attachment structs so response hydration can render thumbnails.
- Modify `apps/server/internal/api/agent_handler.go`: include attachment URLs on upload response and hydrate message responses from asset IDs.
- Modify `apps/server/internal/api/agent_response.go`: route message response conversion through a hydrator where handlers have storage/query access.
- Modify `apps/server/internal/api/agent_handler_test.go`: add tests proving URL fields are render-only and upload attachment response carries URL.
- Modify `apps/web/src/lib/agentApi.ts`: add optional `url` and `thumbnail_url` to `AgentAttachment`.
- Modify `apps/web/src/lib/agentMessageBlocks.ts` and tests: accept hydrated attachment URL fields while keeping stable ID validation.
- Modify `apps/web/src/components/agent/AgentAttachmentBlock.tsx`: render image thumbnail tiles when URL is available.
- Create `apps/web/src/components/agent/AgentReadonlyCanvas.tsx`: React Flow read-only viewport that reuses Studio node/group/edge conversion.
- Create `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`: read-only detail surface for selected node.
- Modify `apps/web/src/pages/AgentWorkspacePage.tsx`: replace text node list with read-only canvas and wire selected-node detail.
- Modify `apps/web/src/main.css`: thumbnail, read-only canvas, and detail drawer styles.
- Add/update frontend tests under `apps/web/src/lib/*.test.mjs`: source checks for thumbnail rendering, shared canvas conversion, and absence of mutation controls in Agent detail.

## Task 1: Backend Attachment URL Hydration

**Files:**
- Modify: `apps/server/internal/agent/uimessage/blocks.go`
- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/internal/api/agent_response.go`
- Test: `apps/server/internal/api/agent_handler_test.go`

- [ ] **Step 1: Write failing tests**

Add tests that verify:

```go
func TestAgentMessageAttachmentAllowsHydratedRenderURLs(t *testing.T) {
	attachment := agentMessageAttachment{
		AssetID: "asset-1", NodeID: "node-1", Kind: "image",
		Name: "design.png", Mime: "image/png", SizeBytes: 128,
		URL: "http://localhost/image.png", ThumbnailURL: "http://localhost/thumb.png",
	}
	raw, err := json.Marshal(attachment)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"url":"http://localhost/image.png"`) {
		t.Fatalf("raw = %s", raw)
	}
	if !strings.Contains(string(raw), `"thumbnail_url":"http://localhost/thumb.png"`) {
		t.Fatalf("raw = %s", raw)
	}
}

func TestAgentMessageContentDoesNotRequireRenderURLs(t *testing.T) {
	body := agentMessageContent("hello", "client-1", []agentMessageAttachment{{
		AssetID: "asset-1", NodeID: "node-1", Kind: "image",
		Name: "design.png", Mime: "image/png", SizeBytes: 128,
	}})
	if strings.Contains(string(body), `"url"`) || strings.Contains(string(body), `"thumbnail_url"`) {
		t.Fatalf("content must not persist render URLs: %s", body)
	}
}
```

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
cd apps/server && go test ./internal/api -run 'TestAgentMessageAttachmentAllowsHydratedRenderURLs|TestAgentMessageContentDoesNotRequireRenderURLs' -count=1
```

Expected: the first test fails because `agentMessageAttachment` has no URL fields.

- [ ] **Step 3: Implement minimal backend fields and hydration**

Add optional fields:

```go
type agentMessageAttachment struct {
	AssetID      string `json:"asset_id"`
	NodeID       string `json:"node_id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Mime         string `json:"mime"`
	SizeBytes    int64  `json:"size_bytes"`
	URL          string `json:"url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}
```

Mirror these fields in `uimessage.Attachment`.

Ensure `toUIMessageAttachments` intentionally does not copy URL fields when persisting user message content.

In `PostAttachment`, set `attachment.URL = accessURL` and `attachment.ThumbnailURL = accessURL` for image/video uploads when `accessURL` is non-empty.

Add handler-level response conversion:

```go
func (h *AgentHandler) toAgentMessageResponse(ctx context.Context, msg db.AgentMessage) agentMessageResponse {
	resp := toAgentMessageResponse(msg)
	resp.Content = h.hydrateAgentMessageAttachmentURLs(ctx, msg.WorkspaceID, msg.ID, resp.Content)
	return resp
}
```

Update `ListMessages`, `PostMessage`, decision responses, and WebSocket/broadcast call sites in `AgentHandler` to use the handler method where available.

Hydration rules:

- parse `content.blocks`;
- find `type="attachment"` blocks;
- for each attachment with `asset_id`, load `GetMediaAssetByID`;
- if workspace matches and `storage_url` exists, derive object key with existing storage helper and sign URL;
- inject `url` and `thumbnail_url`;
- log failures with workspace ID, message ID, asset ID, and error.

- [ ] **Step 4: Run backend targeted tests and confirm GREEN**

Run:

```bash
cd apps/server && go test ./internal/api -run 'TestAgentMessageAttachmentAllowsHydratedRenderURLs|TestAgentMessageContentDoesNotRequireRenderURLs|TestAgentMessageContentIncludesAttachments' -count=1
```

Expected: PASS.

## Task 2: Frontend Attachment Thumbnail Rendering

**Files:**
- Modify: `apps/web/src/lib/agentApi.ts`
- Modify: `apps/web/src/lib/agentMessageBlocks.ts`
- Modify: `apps/web/src/components/agent/AgentAttachmentBlock.tsx`
- Modify: `apps/web/src/main.css`
- Test: `apps/web/src/lib/agentMessageBlocks.test.mjs`
- Test: `apps/web/src/lib/agentAttachments.test.mjs`

- [ ] **Step 1: Write failing frontend tests**

Add tests that assert the TypeScript source accepts `thumbnail_url` and renderer source contains an image preview path:

```js
test("agent attachment type includes hydrated preview urls", async () => {
  const source = await readFile(new URL("./agentApi.ts", import.meta.url), "utf8");
  assert.match(source, /thumbnail_url\?: string/);
  assert.match(source, /url\?: string/);
});

test("agent attachment block renders image thumbnails", async () => {
  const source = await readFile(
    new URL("../components/agent/AgentAttachmentBlock.tsx", import.meta.url),
    "utf8",
  );
  assert.match(source, /<img/);
  assert.match(source, /agent-attachment-thumbnail/);
});
```

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: tests fail because URL fields and `<img>` thumbnail rendering do not exist.

- [ ] **Step 3: Implement thumbnail rendering**

Update `AgentAttachment`:

```ts
export interface AgentAttachment {
  asset_id: string;
  node_id: string;
  kind: "image" | "video" | "text";
  name: string;
  mime: string;
  size_bytes: number;
  url?: string;
  thumbnail_url?: string;
}
```

Update `isAgentAttachment` to allow optional string URL fields.

Update `AgentAttachmentBlock`:

- choose `attachment.thumbnail_url ?? attachment.url`;
- render image attachments with `<img className="agent-attachment-thumbnail" />`;
- render filename and MIME/size metadata beside/below preview;
- keep the current chip fallback for missing URLs and text files.

Add CSS classes:

- `.agent-attachment-preview-card`
- `.agent-attachment-thumbnail`
- `.agent-attachment-preview-meta`

- [ ] **Step 4: Run frontend targeted tests and confirm GREEN**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

## Task 3: Agent Read-Only Canvas Viewport

**Files:**
- Create: `apps/web/src/components/agent/AgentReadonlyCanvas.tsx`
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/main.css`
- Test: `apps/web/src/lib/canvasLayering.test.mjs`

- [ ] **Step 1: Write failing source-level test**

Add assertions to an existing frontend source test or a new `agentReadonlyCanvas.test.mjs`:

```js
test("agent readonly canvas reuses studio view-model projection", async () => {
  const source = await readFile(
    new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
    "utf8",
  );
  assert.match(source, /nodeToFlowNode/);
  assert.match(source, /groupToFlowNode/);
  assert.match(source, /edgeToFlowEdge/);
  assert.match(source, /MediaFlowNode/);
  assert.match(source, /React Flow/);
});

test("agent workspace no longer renders text-only node cards", async () => {
  const source = await readFile(
    new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
    "utf8",
  );
  assert.doesNotMatch(source, /agent-node-card/);
  assert.match(source, /AgentReadonlyCanvas/);
});
```

Add the new test file to `apps/web/package.json` `test:connections`.

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: fails because `AgentReadonlyCanvas.tsx` does not exist and `agent-node-card` still exists.

- [ ] **Step 3: Implement read-only canvas**

Create `AgentReadonlyCanvas` that:

- accepts `canvas: CanvasPayload`;
- accepts `selectedNodeId` and `onSelectNode`;
- renders `React Flow` with `[GroupFlowNode, MediaFlowNode]`;
- uses `nodeToFlowNode`, `groupToFlowNode`, and `edgeToFlowEdge`;
- syncs canvas data into React Flow using `editor.store.mergeRemoteChanges`;
- dispatches active node change event for existing media node active styling;
- listens for `clip-anvil:select-node` events from `MediaFlowNode`;
- allows pan/zoom/select;
- disables toolbar keyboard shortcuts and hides React Flow UI components.

Replace the Agent page's `.agent-node-list` block with:

```tsx
<AgentReadonlyCanvas
  canvas={canvas}
  onSelectNode={setSelectedNodeId}
  selectedNodeId={selectedNodeId}
/>
```

- [ ] **Step 4: Run frontend tests and build**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 4: Agent Read-Only Node Detail Drawer

**Files:**
- Create: `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/main.css`
- Test: `apps/web/src/lib/agentReadonlyCanvas.test.mjs`

- [ ] **Step 1: Write failing source-level test**

Add tests that verify Agent detail is read-only and includes production fields:

```js
test("agent node detail drawer exposes production information without edit controls", async () => {
  const source = await readFile(
    new URL("../components/agent/AgentNodeDetailDrawer.tsx", import.meta.url),
    "utf8",
  );
  assert.match(source, /Prompt/);
  assert.match(source, /Model/);
  assert.match(source, /Versions/);
  assert.match(source, /Status/);
  assert.doesNotMatch(source, /onUpdateNode/);
  assert.doesNotMatch(source, /textarea/);
});
```

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: fails because detail drawer does not exist.

- [ ] **Step 3: Implement drawer and data loading**

Create `AgentNodeDetailDrawer` with props:

```ts
{
  node: MediaNode | null;
  productionState: NodeProductionState | null;
  isLoading: boolean;
  edges: MediaEdge[];
  nodes: MediaNode[];
  onClose: () => void;
}
```

Render read-only sections:

- identity: title, type, source, status, node ID;
- asset: asset ID, MIME, preview URL presence, size when available;
- prompt: prompt text or empty state;
- model: provider/model/params;
- relationships: upstream/downstream edge counts and names;
- versions: current version, latest version, review score, status;
- latest job: provider/model/status/progress/error;
- stale/errors: active stale reasons and node/job errors.

In `AgentWorkspacePage`, load production state only when a node is selected:

```ts
const selectedNodeProductionStateQuery = useQuery({
  queryKey: ["node", selectedNodeId, "production-state"],
  queryFn: () => fetchNodeProductionState(selectedNodeId ?? ""),
  enabled: Boolean(selectedNodeId),
});
```

Render the drawer over the canvas on the left or bottom, leaving the right floating chat free.

- [ ] **Step 4: Run tests and build**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 5: Full Verification And Browser E2E

**Files:**
- No new files unless test failures require focused fixes.

- [ ] **Step 1: Run backend verification**

Run:

```bash
make server-build
make server-test
make server-lint
```

Expected: PASS.

- [ ] **Step 2: Run frontend verification**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: PASS.

- [ ] **Step 3: Start local app**

Run:

```bash
./scripts/dev-start.sh
```

Use the Vite URL printed by the script.

- [ ] **Step 4: Browser E2E**

In browser:

1. Open the printed Vite Agent workspace URL.
2. Create or use an Agent workspace.
3. Upload a PNG image in the ClipAnvil composer.
4. Confirm the message composer/sent bubble shows a thumbnail.
5. Send a message with the image.
6. Confirm the canvas renders a visual image node instead of `filename image`.
7. Select the image node.
8. Confirm read-only detail opens.
9. Confirm prompt/model/version/status fields render as read-only or empty states.
10. Refresh the page and confirm the thumbnail still renders from a freshly hydrated URL.

- [ ] **Step 5: Inspect logs/data for hydration errors**

Check server logs from the dev profile. Confirm there are no attachment hydration errors for the successful PNG case.
