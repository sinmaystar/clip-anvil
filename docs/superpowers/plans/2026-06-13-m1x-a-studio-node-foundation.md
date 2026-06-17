# M1.x-A Studio Node Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand the current text-only Studio canvas into a four-media-type node canvas with stable backend defaults and frontend placeholder rendering.

**Architecture:** Keep the current M1 data model as the source of truth and reuse the existing `media_type` enum. The backend only relaxes node type validation and returns type-specific sizes; the frontend maps the existing node DTO into richer shape props and renders type-specific cards without introducing assets, edges, groups, or WebSocket state.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, React 19, TypeScript 6, tldraw 5, TanStack Query, TailwindCSS 4.

---

## File Structure

- Modify `apps/server/internal/api/node_handler.go`: allow four node types and return type-specific dimensions.
- Modify `apps/server/internal/api/node_handler_test.go`: cover four-type creation, dimensions, and invalid type rejection.
- Modify `packages/canvas-schema/src/index.ts`: add optional `thumbnailUrl` to `MediaShapeProps`.
- Modify `apps/web/src/lib/api.ts`: add `thumbnail_url?: string` to `MediaNode`.
- Modify `apps/web/src/lib/canvas.ts`: map `thumbnail_url` to `thumbnailUrl`.
- Modify `apps/web/src/shapes/MediaShapeUtil.tsx`: render text/image/video/audio cards.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`: make the context menu create four node types and show type-aware sidebar rows.
- Modify `apps/web/src/main.css`: add styles for type placeholders and menu rows if existing styles are insufficient.

### Task 1: Backend Multi-Type Node Defaults

**Files:**
- Modify: `apps/server/internal/api/node_handler_test.go`
- Modify: `apps/server/internal/api/node_handler.go`

- [ ] **Step 1: Add failing backend tests for image/video/audio creation**

Add table-driven coverage to `apps/server/internal/api/node_handler_test.go` near the existing create-node tests:

```go
func TestNodeHandlerCreateAllowsM1xNodeTypes(t *testing.T) {
	testCases := []struct {
		nodeType string
		width    float32
		height   float32
	}{
		{nodeType: "text", width: 200, height: 120},
		{nodeType: "image", width: 200, height: 160},
		{nodeType: "video", width: 240, height: 180},
		{nodeType: "audio", width: 200, height: 80},
	}

	for _, tc := range testCases {
		t.Run(tc.nodeType, func(t *testing.T) {
			server, token, workspaceID := newNodeHandlerTestServer(t)
			body := strings.NewReader(fmt.Sprintf(
				`{"workspace_id":%q,"node_type":%q,"title":"节点","canvas_x":12,"canvas_y":34}`,
				workspaceID,
				tc.nodeType,
			))
			req := httptest.NewRequest(http.MethodPost, "/api/nodes", body)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp := httptest.NewRecorder()
			server.ServeHTTP(resp, req)

			require.Equal(t, http.StatusOK, resp.Code)
			var node db.MediaNode
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&node))
			require.Equal(t, db.MediaType(tc.nodeType), node.NodeType)
			require.Equal(t, tc.width, node.CanvasW)
			require.Equal(t, tc.height, node.CanvasH)
		})
	}
}
```

If the current test helpers use different names, keep their existing helper style and preserve the same assertions.

- [ ] **Step 2: Run the new backend test and verify it fails**

Run:

```bash
cd apps/server && go test ./internal/api -run TestNodeHandlerCreateAllowsM1xNodeTypes -count=1
```

Expected: FAIL for `image`, `video`, and `audio` with `400 invalid node type`.

- [ ] **Step 3: Implement multi-type validation and defaults**

In `apps/server/internal/api/node_handler.go`, replace the M1-only validator and dimensions with:

```go
func defaultNodeSize(nodeType db.MediaType) (float32, float32) {
	switch nodeType {
	case db.MediaTypeText:
		return 200, 120
	case db.MediaTypeImage:
		return 200, 160
	case db.MediaTypeVideo:
		return 240, 180
	case db.MediaTypeAudio:
		return 200, 80
	default:
		return 200, 120
	}
}

func isAllowedNodeType(nodeType db.MediaType) bool {
	switch nodeType {
	case db.MediaTypeText, db.MediaTypeImage, db.MediaTypeVideo, db.MediaTypeAudio:
		return true
	default:
		return false
	}
}
```

Update the call site in `Create`:

```go
if !isAllowedNodeType(nodeType) {
	writeError(c, consts.StatusBadRequest, "invalid node type")
	return
}
```

- [ ] **Step 4: Run backend API tests**

Run:

```bash
cd apps/server && go test ./internal/api -count=1
```

Expected: PASS.

- [ ] **Step 5: Run all backend tests**

Run:

```bash
make server-test
```

Expected: PASS for all Go packages.

### Task 2: Shared Shape Contract

**Files:**
- Modify: `packages/canvas-schema/src/index.ts`
- Modify: `apps/web/src/lib/api.ts`
- Modify: `apps/web/src/lib/canvas.ts`

- [ ] **Step 1: Extend shape props with optional thumbnail URL**

In `packages/canvas-schema/src/index.ts`, change `MediaShapeProps` to:

```ts
export interface MediaShapeProps {
  nodeId: string;
  nodeType: MediaType;
  title: string;
  prompt: string;
  status: NodeStatus;
  thumbnailUrl?: string;
  w: number;
  h: number;
}
```

- [ ] **Step 2: Extend the API DTO**

In `apps/web/src/lib/api.ts`, add the optional API field:

```ts
export interface MediaNode {
  id: string;
  workspace_id: string;
  node_type: MediaType;
  title: string;
  prompt: string;
  asset_url?: string;
  thumbnail_url?: string;
  status: NodeStatus;
  canvas_x: number;
  canvas_y: number;
  canvas_w: number;
  canvas_h: number;
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 3: Map the optional field into shape props**

In `apps/web/src/lib/canvas.ts`, update `nodeToShapeProps`:

```ts
export function nodeToShapeProps(node: MediaNode): MediaShapeProps {
  return {
    nodeId: node.id,
    nodeType: node.node_type,
    title: node.title || `未命名${nodeTypeLabel(node.node_type)}`,
    prompt: node.prompt,
    status: node.status,
    thumbnailUrl: node.thumbnail_url,
    w: node.canvas_w,
    h: node.canvas_h,
  };
}

function nodeTypeLabel(nodeType: MediaNode["node_type"]) {
  switch (nodeType) {
    case "image":
      return "图片";
    case "video":
      return "视频";
    case "audio":
      return "音频";
    case "text":
    default:
      return "文本";
  }
}
```

- [ ] **Step 4: Type-check the web package**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: TypeScript succeeds or fails only on components that still need `thumbnailUrl` prop validation. If it fails on `MediaShapeUtil.props`, continue to Task 3 Step 1.

### Task 3: Type-Specific MediaShape Rendering

**Files:**
- Modify: `apps/web/src/shapes/MediaShapeUtil.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Add optional prop validation**

In `MediaShapeUtil.props`, add:

```ts
thumbnailUrl: T.optional(T.string),
```

- [ ] **Step 2: Replace text-only render metadata with type metadata**

Add this near `statusText`:

```ts
const nodeTypeMeta: Record<
  MediaShape["props"]["nodeType"],
  { icon: string; label: string; emptyTitle: string }
> = {
  text: { icon: "T", label: "Text", emptyTitle: "未命名文本" },
  image: { icon: "I", label: "Image", emptyTitle: "未命名图片" },
  video: { icon: "V", label: "Video", emptyTitle: "未命名视频" },
  audio: { icon: "A", label: "Audio", emptyTitle: "未命名音频" },
};
```

- [ ] **Step 3: Render type-specific content**

Inside `MediaNodeShape`, destructure `nodeType` and `thumbnailUrl`, then replace the fixed content with:

```tsx
const { title, prompt, status, nodeType, thumbnailUrl, w, h } = shape.props;
const typeMeta = nodeTypeMeta[nodeType];
```

Replace the content area with:

```tsx
<div className="media-node-content" data-type={nodeType}>
  {nodeType === "text" ? (
    <p>{promptValue || "等待输入 prompt"}</p>
  ) : nodeType === "image" ? (
    thumbnailUrl ? (
      <img alt={titleValue || typeMeta.emptyTitle} src={thumbnailUrl} />
    ) : (
      <div className="media-node-placeholder">图片占位</div>
    )
  ) : nodeType === "video" ? (
    <div className="media-node-placeholder">
      <span>播放预览</span>
      <span>0:00</span>
    </div>
  ) : (
    <div className="media-node-placeholder">
      <span className="media-node-waveform" />
      <span>0:00</span>
    </div>
  )}
</div>
```

Update header/footer labels:

```tsx
<span className="media-node-icon">{typeMeta.icon}</span>
<p className="media-node-title">{titleValue || typeMeta.emptyTitle}</p>
<span className="media-node-status">{statusText[status]}</span>
```

Update the footer labels:

```tsx
<span>{typeMeta.label}</span>
<span>Prompt</span>
```

Set the data attribute:

```tsx
data-type={nodeType}
```

- [ ] **Step 4: Add CSS for media placeholders**

Append or merge into `apps/web/src/main.css`:

```css
.media-node-content[data-type="image"],
.media-node-content[data-type="video"] {
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.media-node-content img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.media-node-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 100%;
  height: 100%;
  color: var(--fg-tertiary);
  font-size: 12px;
}

.media-node-waveform {
  width: 112px;
  height: 18px;
  border-radius: 999px;
  background: repeating-linear-gradient(
    90deg,
    color-mix(in_srgb, var(--fg-tertiary) 55%, transparent) 0 3px,
    transparent 3px 8px
  );
}
```

- [ ] **Step 5: Build the web package**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

### Task 4: Four-Type Context Menu and Sidebar

**Files:**
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Add node type metadata to the page**

In `WorkspaceDetailPage.tsx`, import `type MediaType` from `../lib/api`, then add:

```ts
const nodeCreateOptions: Array<{
  type: MediaType;
  title: string;
  description: string;
  icon: string;
  defaultTitle: string;
}> = [
  {
    type: "text",
    title: "文本节点",
    description: "提示词 / 文案 / 旁白",
    icon: "T",
    defaultTitle: "未命名文本",
  },
  {
    type: "image",
    title: "图片节点",
    description: "参考图 / 产品图 / 画面素材",
    icon: "I",
    defaultTitle: "未命名图片",
  },
  {
    type: "video",
    title: "视频节点",
    description: "镜头 / 片段 / 成片",
    icon: "V",
    defaultTitle: "未命名视频",
  },
  {
    type: "audio",
    title: "音频节点",
    description: "配乐 / 旁白 / 音效",
    icon: "A",
    defaultTitle: "未命名音频",
  },
];
```

- [ ] **Step 2: Make createNodeMutation accept node type**

Change the mutation input:

```ts
mutationFn: async (input?: { point?: { x: number; y: number }; nodeType?: MediaType }) => {
  if (!id || !editorRef.current) {
    throw new Error("画布尚未准备好");
  }

  const nodeType = input?.nodeType ?? "text";
  const option =
    nodeCreateOptions.find((item) => item.type === nodeType) ?? nodeCreateOptions[0];
  const center = editorRef.current.getViewportPageBounds().center;
  const position = input?.point ?? center;

  return createMediaNode({
    workspace_id: id,
    node_type: nodeType,
    title: option.defaultTitle,
    canvas_x: position.x - 100,
    canvas_y: position.y - 60,
  });
},
```

- [ ] **Step 3: Render menu options**

Replace the single context-menu button with:

```tsx
{nodeCreateOptions.map((option) => (
  <button
    key={option.type}
    onClick={() =>
      createNodeMutation.mutate({
        point: { x: contextMenu.pageX, y: contextMenu.pageY },
        nodeType: option.type,
      })
    }
    type="button"
  >
    <span className="studio-menu-icon">{option.icon}</span>
    <span>
      <span className="block text-[13px] font-semibold">{option.title}</span>
      <span className="block text-[11.5px] text-[var(--fg-tertiary)]">
        {option.description}
      </span>
    </span>
  </button>
))}
```

- [ ] **Step 4: Make sidebar rows type-aware**

Change the sidebar icon/title:

```tsx
const option =
  nodeCreateOptions.find((item) => item.type === node.node_type) ?? nodeCreateOptions[0];

return (
  <button
    className="studio-resource-item"
    data-selected={node.id === selectedNodeId}
    key={node.id}
    onClick={() => selectNode(node.id)}
    type="button"
  >
<span className="studio-resource-thumb">{option.icon}</span>
<span className="studio-resource-name">
  {node.title || option.defaultTitle}
</span>
  </button>
);
```

If the current inline JSX makes this awkward, extract a local `renderNodeRow(node)` helper in the same file.

- [ ] **Step 5: Build and manually verify**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

Manual check with dev server:

```bash
pnpm --filter @clip-anvil/web dev
```

Expected: right-click canvas shows four creation options; each creates the correct type and size after backend is running.

### Task 5: Final M1.x-A Verification and Commit

**Files:**
- All files changed in Tasks 1-4.

- [ ] **Step 1: Run backend verification**

Run:

```bash
make server-test
```

Expected: PASS.

- [ ] **Step 2: Run frontend verification**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [ ] **Step 3: Inspect the diff**

Run:

```bash
git diff -- apps/server/internal/api/node_handler.go apps/server/internal/api/node_handler_test.go packages/canvas-schema/src/index.ts apps/web/src/lib/api.ts apps/web/src/lib/canvas.ts apps/web/src/shapes/MediaShapeUtil.tsx apps/web/src/pages/WorkspaceDetailPage.tsx apps/web/src/main.css
```

Expected: only M1.x-A node foundation changes are present.

- [ ] **Step 4: Commit**

```bash
git add apps/server/internal/api/node_handler.go apps/server/internal/api/node_handler_test.go packages/canvas-schema/src/index.ts apps/web/src/lib/api.ts apps/web/src/lib/canvas.ts apps/web/src/shapes/MediaShapeUtil.tsx apps/web/src/pages/WorkspaceDetailPage.tsx apps/web/src/main.css
git commit -m "feat: support multi-type studio nodes"
```
