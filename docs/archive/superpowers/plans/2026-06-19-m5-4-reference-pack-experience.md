# M5.4 Reference Pack Experience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn Reference Pack from a creatable placeholder node into a usable Studio semantic asset pack: users can see pack summaries, add/remove direct members, use packs as dependency inputs, and observe downstream stale when membership changes.

**Architecture:** Reuse the M4 Reference Pack backend primitives and M5.1-M5.3 Studio foundation. Add a lightweight canvas pack preview so pack cards can show member counts without per-pack frontend fetches, then wire the selected pack property panel to the existing `fetchReferencePackItems` / `replaceReferencePackItems` API helpers. Keep membership separate from dependency edges and keep Prompt `@` references deferred to M5.5.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, React 19, TypeScript 6, TanStack Query, React Flow, Vite 8, Node test runner, in-app browser smoke.

---

## Current-State Notes

- M5.1-M5.3 are present in the working tree but not committed.
- `reference_pack` is already a valid `media_node.node_type`.
- The backend already has `reference_pack_item`, `ReferencePackHandler.ListItems`, and `ReferencePackHandler.ReplaceItems`.
- Backend validation already rejects duplicate members, self-membership, nested packs, and cross-workspace members.
- `replaceReferencePackItems` already calls `production.MarkDownstreamStale` with reason `reference_pack_membership_changed`.
- Frontend API helpers already exist:
  - `fetchReferencePackItems(id)`
  - `replaceReferencePackItems(id, member_node_ids)`
- Frontend can already create Reference Pack nodes and render a placeholder card.
- M5.3 added `production_preview` and `active_stale_reason_count`, but Reference Pack cards still show `等待成员` because canvas does not expose member summaries.
- `PropertyPanel.tsx` currently treats Reference Pack mostly like any other node; it does not show member candidates or membership editing controls.

## Scope Boundaries

M5.4 includes:

- Reference Pack canvas card member summary.
- Reference Pack property panel membership list.
- Add/remove existing non-pack workspace nodes as direct members.
- Preserve member display order returned by `reference_pack_item.position`.
- Refresh canvas, selected pack membership, selected production state, and downstream stale UI after membership changes.
- Dependency edges to a Reference Pack remain supported through the existing edge UI/API.
- E2E smoke confirms a dependent node intent expands pack members and membership changes mark downstream stale.

M5.4 does not include:

- Prompt `@` candidate menu or explicit `prompt_refs`. That is M5.5.
- Pack nesting.
- Adding raw assets directly to packs.
- Drag-and-drop pack member sorting.
- Manual version snapshotting for pack membership history.
- Multi-pack bulk operations.
- Agent mode behavior.

---

## File Structure

- Modify `apps/server/internal/api/canvas_handler.go`
  - Add `reference_pack_preview` to canvas node responses.
  - Populate member count and first member summaries for pack nodes.
- Modify `apps/server/internal/api/canvas_handler_test.go`
  - Add conversion tests for Reference Pack preview.
- Modify `apps/web/src/lib/api.ts`
  - Add `ReferencePackPreview` and `ReferencePackPreviewMember` to `MediaNode`.
- Create `apps/web/src/lib/referencePack.ts`
  - Pure helpers for member rows, candidate rows, toggle payloads, and canvas summary text.
- Create `apps/web/src/lib/referencePack.test.mjs`
  - Unit tests for helper behavior.
- Modify `apps/web/src/lib/productionPreview.ts`
  - Prefer Reference Pack preview summary for `reference_pack` node text.
- Modify `apps/web/src/lib/canvas.ts`
  - Pass Reference Pack preview summary into media node data through `winnerPreviewText`.
- Modify `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
  - Keep current node renderer; no new node data expected if `previewText` carries the pack summary.
- Modify `apps/web/src/components/PropertyPanel.tsx`
  - Add Reference Pack-specific member management UI.
  - Keep group, dependency, stale, and version/job sections visible where useful.
  - Hide model params/run semantics that do not apply to packs.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`
  - Fetch selected pack items when a Reference Pack is selected.
  - Add `replaceReferencePackItems` mutation.
  - Invalidate canvas and production-state queries after membership changes.
- Modify `apps/web/src/main.css`
  - Add compact member list, candidate list, and pack action styling.
- Modify `apps/web/tsconfig.test.json`
  - Include `src/lib/referencePack.ts`.
- Modify `apps/web/package.json`
  - Add `src/lib/referencePack.test.mjs` to `test:connections`.

---

## Deliverable Standards

- Reference Pack card shows member count and first few member labels, not only `等待成员`.
- Reference Pack card still shows stale badge from M5.3 when downstream/source state marks it stale or degraded.
- Selecting a Reference Pack opens a property panel with:
  - current members in position order,
  - available non-pack candidate nodes,
  - add/remove controls,
  - membership save state,
  - dependency/stale sections.
- Candidate list excludes:
  - the pack itself,
  - other Reference Pack nodes,
  - already selected members.
- Removing a member preserves the relative order of remaining members.
- Adding a member appends it after existing members.
- Membership changes do not alter media groups.
- Dependency edges to a pack still behave like ordinary dependency edges.
- A node depending on a pack sees expanded direct member refs in latest job intent.
- Changing pack membership marks downstream dependents stale with `reference_pack_membership_changed`.

## Acceptance Standards

- User can create Pack P and add Image A and Image B.
- Refreshing the page preserves Pack P membership and card summary.
- User can remove Image B from Pack P.
- Pack P cannot contain itself or another pack.
- User can connect P -> Image C.
- Running Image C produces a latest job intent containing one `reference_pack` input and direct `reference_pack_member` refs.
- Changing P membership marks Image C stale with reason `reference_pack_membership_changed`.
- Existing M5.1-M5.3 flows still pass: normal node creation, manual run, version display, stale recovery.

## E2E Smoke Case

1. Start the app with `./scripts/dev-start.sh` and use the printed Vite URL.
2. Register or log in.
3. Create a Studio Workspace named `M5.4 Reference Pack Smoke`.
4. Create Image Node A and Image Node B.
5. Configure A and B as `text_to_image` / `mock-image-only` and run them.
6. Create Reference Pack P.
7. Select P and add A and B as members from the property panel.
8. Refresh the page and confirm P still shows two members.
9. Create Image Node C.
10. Create dependency P -> C.
11. Configure C as `text_to_image` / `mock-image-only` and run it.
12. Confirm C latest job intent includes P as `reference_pack` and A/B as `reference_pack_member`.
13. Remove B from P in the property panel.
14. Confirm C canvas card and property panel show stale reason `reference_pack_membership_changed`.
15. Check browser console for new application errors.

---

## Task 1: Add Backend Canvas Reference Pack Preview

**Files:**
- Modify: `apps/server/internal/api/canvas_handler.go`
- Modify: `apps/server/internal/api/canvas_handler_test.go`

- [ ] **Step 1: Add failing backend preview tests**

Append these tests in `apps/server/internal/api/canvas_handler_test.go`:

```go
func TestCanvasNodeResponsesIncludeReferencePackPreview(t *testing.T) {
	packID := pgtype.UUID{Bytes: [16]byte{0x21}, Valid: true}
	memberAID := pgtype.UUID{Bytes: [16]byte{0x22}, Valid: true}
	memberBID := pgtype.UUID{Bytes: [16]byte{0x23}, Valid: true}
	pack := db.MediaNode{
		ID:       packID,
		NodeType: db.NodeTypeReferencePack,
		Title:    "商品参考包",
	}
	packMembers := map[pgtype.UUID][]db.MediaNode{
		packID: {
			{ID: memberAID, NodeType: db.NodeTypeImage, Title: "主图"},
			{ID: memberBID, NodeType: db.NodeTypeVideo, Title: "动作参考"},
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{pack}, nil, nil, nil, packMembers)

	if nodes[0].ReferencePackPreview == nil {
		t.Fatal("reference pack preview should be set")
	}
	if nodes[0].ReferencePackPreview.MemberCount != 2 {
		t.Fatalf("member count = %d, want 2", nodes[0].ReferencePackPreview.MemberCount)
	}
	if len(nodes[0].ReferencePackPreview.Members) != 2 {
		t.Fatalf("members len = %d, want 2", len(nodes[0].ReferencePackPreview.Members))
	}
	if nodes[0].ReferencePackPreview.Members[0].Title != "主图" {
		t.Fatalf("first member = %#v", nodes[0].ReferencePackPreview.Members[0])
	}
}

func TestCanvasNodeResponsesDoNotAttachReferencePackPreviewToNormalNodes(t *testing.T) {
	node := db.MediaNode{
		ID:       pgtype.UUID{Bytes: [16]byte{0x24}, Valid: true},
		NodeType: db.NodeTypeImage,
		Title:    "普通图片",
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, nil, nil, nil, nil)

	if nodes[0].ReferencePackPreview != nil {
		t.Fatalf("normal node preview = %#v, want nil", nodes[0].ReferencePackPreview)
	}
}
```

Update existing `toCanvasNodeResponses` test calls in the same file to pass the new fifth argument `nil`.

- [ ] **Step 2: Run focused backend tests and verify failure**

Run from `apps/server`:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestCanvasNodeResponses(IncludeReferencePackPreview|DoNotAttachReferencePackPreviewToNormalNodes)'
```

Expected: FAIL because `ReferencePackPreview` and the fifth `toCanvasNodeResponses` argument do not exist.

- [ ] **Step 3: Add response structs**

In `apps/server/internal/api/canvas_handler.go`, extend `canvasNodeResponse`:

```go
type canvasNodeResponse struct {
	db.MediaNode
	ThumbnailURL           *string                     `json:"thumbnail_url,omitempty"`
	ProductionPreview      *canvasProductionPreview    `json:"production_preview,omitempty"`
	ReferencePackPreview   *canvasReferencePackPreview `json:"reference_pack_preview,omitempty"`
	ActiveStaleReasonCount int                         `json:"active_stale_reason_count"`
}

type canvasReferencePackPreview struct {
	MemberCount int                                `json:"member_count"`
	Members     []canvasReferencePackPreviewMember `json:"members"`
}

type canvasReferencePackPreviewMember struct {
	ID       string `json:"id"`
	NodeType string `json:"node_type"`
	Title    string `json:"title"`
	Status   string `json:"status"`
}
```

- [ ] **Step 4: Load pack members for canvas**

In `GetCanvas`, after loading stale counts, load pack members:

```go
packMembers, err := h.referencePackMembersByPack(ctx, nodes)
if err != nil {
	writeError(c, consts.StatusInternalServerError, "failed to load reference pack members")
	return
}
```

Pass `packMembers` into `toCanvasNodeResponses`.

Add helper:

```go
func (h *CanvasHandler) referencePackMembersByPack(ctx context.Context, nodes []db.MediaNode) (map[pgtype.UUID][]db.MediaNode, error) {
	out := map[pgtype.UUID][]db.MediaNode{}
	for _, node := range nodes {
		if node.NodeType != db.NodeTypeReferencePack {
			continue
		}
		members, err := h.queries.ListReferencePackItemNodes(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		out[node.ID] = members
	}
	return out, nil
}
```

- [ ] **Step 5: Convert pack preview**

Change `toCanvasNodeResponses` signature:

```go
func toCanvasNodeResponses(
	nodes []db.MediaNode,
	assets map[pgtype.UUID]db.MediaAsset,
	versions map[pgtype.UUID]db.ArtifactVersion,
	staleCounts map[pgtype.UUID]int,
	packMembers map[pgtype.UUID][]db.MediaNode,
) []canvasNodeResponse
```

Inside the loop:

```go
if node.NodeType == db.NodeTypeReferencePack {
	response.ReferencePackPreview = toCanvasReferencePackPreview(packMembers[node.ID])
}
```

Add helper:

```go
func toCanvasReferencePackPreview(members []db.MediaNode) *canvasReferencePackPreview {
	preview := &canvasReferencePackPreview{
		MemberCount: len(members),
		Members:     make([]canvasReferencePackPreviewMember, 0, min(len(members), 3)),
	}
	for i, member := range members {
		if i >= 3 {
			break
		}
		preview.Members = append(preview.Members, canvasReferencePackPreviewMember{
			ID:       uuidToString(member.ID),
			NodeType: string(member.NodeType),
			Title:    member.Title,
			Status:   string(member.Status),
		})
	}
	return preview
}
```

If `min` is unavailable in the current Go toolchain, use a simple capacity helper:

```go
func previewMemberCapacity(count int) int {
	if count > 3 {
		return 3
	}
	return count
}
```

- [ ] **Step 6: Run backend API tests**

Run from `apps/server`:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: PASS.

---

## Task 2: Add Frontend Reference Pack Helpers And Types

**Files:**
- Modify: `apps/web/src/lib/api.ts`
- Modify: `apps/web/src/lib/productionPreview.ts`
- Create: `apps/web/src/lib/referencePack.ts`
- Create: `apps/web/src/lib/referencePack.test.mjs`
- Modify: `apps/web/tsconfig.test.json`
- Modify: `apps/web/package.json`

- [ ] **Step 1: Add failing helper tests**

Create `apps/web/src/lib/referencePack.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  candidateReferencePackMembers,
  memberIdsAfterToggle,
  memberNodesForPack,
  referencePackSummaryText,
} from "../../dist-test/lib/referencePack.js";
import { winnerPreviewText } from "../../dist-test/lib/productionPreview.js";

const pack = {
  id: "pack",
  node_type: "reference_pack",
  title: "商品参考包",
  status: "draft",
};

const nodes = [
  pack,
  { id: "image-a", node_type: "image", title: "主图", status: "succeeded" },
  { id: "image-b", node_type: "image", title: "细节图", status: "succeeded" },
  { id: "text-a", node_type: "text", title: "卖点", status: "succeeded" },
  { id: "pack-2", node_type: "reference_pack", title: "另一个包", status: "draft" },
];

const items = [
  { id: "item-2", pack_node_id: "pack", member_node_id: "image-b", position: 1 },
  { id: "item-1", pack_node_id: "pack", member_node_id: "image-a", position: 0 },
];

describe("reference pack helpers", () => {
  it("returns member nodes in position order", () => {
    assert.deepEqual(
      memberNodesForPack(items, nodes).map((node) => node.id),
      ["image-a", "image-b"],
    );
  });

  it("filters candidates to non-pack non-member nodes", () => {
    assert.deepEqual(
      candidateReferencePackMembers(pack, nodes, items).map((node) => node.id),
      ["text-a"],
    );
  });

  it("builds toggle payloads while preserving order", () => {
    assert.deepEqual(memberIdsAfterToggle(items, "image-a", false), ["image-b"]);
    assert.deepEqual(memberIdsAfterToggle(items, "text-a", true), [
      "image-a",
      "image-b",
      "text-a",
    ]);
  });

  it("summarizes pack preview members", () => {
    assert.equal(
      referencePackSummaryText({
        member_count: 2,
        members: [
          { id: "image-a", node_type: "image", title: "主图", status: "succeeded" },
          { id: "image-b", node_type: "image", title: "细节图", status: "succeeded" },
        ],
      }),
      "2 members · image 主图, image 细节图",
    );
  });

  it("uses reference pack preview in winner preview text", () => {
    assert.equal(
      winnerPreviewText({
        node_type: "reference_pack",
        prompt: "",
        reference_pack_preview: {
          member_count: 1,
          members: [{ id: "image-a", node_type: "image", title: "主图", status: "succeeded" }],
        },
      }),
      "1 member · image 主图",
    );
  });
});
```

- [ ] **Step 2: Register tests and verify failure**

Add `src/lib/referencePack.ts` to `apps/web/tsconfig.test.json`.

Append `src/lib/referencePack.test.mjs` to the `test:connections` script in `apps/web/package.json`.

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL because `referencePack.ts` does not exist and `winnerPreviewText` does not read `reference_pack_preview`.

- [ ] **Step 3: Add frontend API types**

In `apps/web/src/lib/api.ts`, add:

```ts
export interface ReferencePackPreviewMember {
  id: string;
  node_type: MediaType;
  title: string;
  status: NodeStatus;
}

export interface ReferencePackPreview {
  member_count: number;
  members: ReferencePackPreviewMember[];
}
```

Add to `MediaNode`:

```ts
  reference_pack_preview?: ReferencePackPreview;
```

- [ ] **Step 4: Create helper module**

Create `apps/web/src/lib/referencePack.ts`:

```ts
import type {
  MediaNode,
  ReferencePackItem,
  ReferencePackPreview,
} from "./api";

export function orderedReferencePackItems(items: ReferencePackItem[]) {
  return [...items].sort((a, b) => a.position - b.position);
}

export function memberNodesForPack(
  items: ReferencePackItem[],
  nodes: MediaNode[],
) {
  const nodesById = new Map(nodes.map((node) => [node.id, node]));
  return orderedReferencePackItems(items)
    .map((item) => nodesById.get(item.member_node_id))
    .filter((node): node is MediaNode => Boolean(node));
}

export function candidateReferencePackMembers(
  pack: Pick<MediaNode, "id">,
  nodes: MediaNode[],
  items: ReferencePackItem[],
) {
  const memberIds = new Set(items.map((item) => item.member_node_id));
  return nodes.filter(
    (node) =>
      node.id !== pack.id &&
      node.node_type !== "reference_pack" &&
      !memberIds.has(node.id),
  );
}

export function memberIdsAfterToggle(
  items: ReferencePackItem[],
  memberNodeId: string,
  checked: boolean,
) {
  const orderedIds = orderedReferencePackItems(items).map(
    (item) => item.member_node_id,
  );
  if (checked) {
    return orderedIds.includes(memberNodeId)
      ? orderedIds
      : [...orderedIds, memberNodeId];
  }
  return orderedIds.filter((id) => id !== memberNodeId);
}

export function referencePackSummaryText(preview?: ReferencePackPreview) {
  if (!preview || preview.member_count === 0) {
    return "等待成员";
  }
  const unit = preview.member_count === 1 ? "member" : "members";
  const memberText = preview.members
    .map((member) => `${member.node_type} ${member.title || "未命名"}`)
    .join(", ");
  return memberText
    ? `${preview.member_count} ${unit} · ${memberText}`
    : `${preview.member_count} ${unit}`;
}
```

- [ ] **Step 5: Use pack summary in preview text**

In `apps/web/src/lib/productionPreview.ts`, update the import and function signature:

```ts
import { referencePackSummaryText } from "./referencePack";

export function winnerPreviewText(
  node: Pick<
    MediaNode,
    "node_type" | "prompt" | "production_preview" | "reference_pack_preview"
  >,
) {
```

Then replace the Reference Pack branch:

```ts
  if (node.node_type === "reference_pack") {
    return referencePackSummaryText(node.reference_pack_preview);
  }
```

If Node test ESM resolution fails for extensionless imports, keep `referencePackSummaryText` logic local to `productionPreview.ts` or make `referencePack.ts` avoid importing from `productionPreview.ts`; do not introduce circular imports.

- [ ] **Step 6: Run frontend tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

---

## Task 3: Add Reference Pack Property Panel UI

**Files:**
- Modify: `apps/web/src/components/PropertyPanel.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Extend `PropertyPanelProps`**

Add imports:

```ts
import type { ReferencePackItem } from "../lib/api";
import {
  candidateReferencePackMembers,
  memberIdsAfterToggle,
  memberNodesForPack,
} from "../lib/referencePack";
```

Add props:

```ts
  isReferencePackItemsLoading: boolean;
  isUpdatingReferencePackItems: boolean;
  referencePackItems: ReferencePackItem[];
  onReplaceReferencePackItems: (packNodeId: string, memberNodeIds: string[]) => void;
```

- [ ] **Step 2: Pass props through `PropertyPanel` to `NodePropertyPanel`**

When rendering `NodePropertyPanel`, pass the four new props unchanged.

- [ ] **Step 3: Add Reference Pack-specific section**

Inside `NodePropertyPanel`, compute:

```ts
const isReferencePack = node.node_type === "reference_pack";
```

Before the normal `Operation` field, render:

```tsx
{isReferencePack ? (
  <ReferencePackMembersSection
    candidates={candidateReferencePackMembers(node, nodes, referencePackItems)}
    isLoading={isReferencePackItemsLoading}
    isUpdating={isUpdatingReferencePackItems}
    members={memberNodesForPack(referencePackItems, nodes)}
    onToggleMember={(memberNodeId, checked) =>
      onReplaceReferencePackItems(
        node.id,
        memberIdsAfterToggle(referencePackItems, memberNodeId, checked),
      )
    }
  />
) : null}
```

- [ ] **Step 4: Hide model/params/run sections for Reference Packs**

Wrap the Operation, Model, Params, Prompt, Run, Versions, and Latest Job production sections so they render only when `!isReferencePack`, except:

- Keep title, type, group, dependency, stale reasons.
- Keep prompt/title inline editor behavior outside the right panel unchanged.
- Keep the M5.2 "Reference Pack 在 M5.4 管理成员" copy out of the Run section because the member UI is now the primary action.

For Reference Pack nodes, show a small informational section:

```tsx
{isReferencePack ? (
  <div className="property-section">
    <p className="studio-section-label">Reference Pack</p>
    <p className="property-empty">
      Reference Pack 管理已有节点的直接成员；成员关系不会改变分组，也不会自动包含成员的上游依赖。
    </p>
  </div>
) : null}
```

- [ ] **Step 5: Add member section component**

Add below `DependencyRows`:

```tsx
function ReferencePackMembersSection({
  candidates,
  isLoading,
  isUpdating,
  members,
  onToggleMember,
}: {
  candidates: MediaNode[];
  isLoading: boolean;
  isUpdating: boolean;
  members: MediaNode[];
  onToggleMember: (memberNodeId: string, checked: boolean) => void;
}) {
  return (
    <div className="property-section">
      <p className="studio-section-label">Members</p>
      {isLoading ? (
        <p className="property-empty">正在读取成员。</p>
      ) : members.length ? (
        <div className="reference-pack-member-list">
          {members.map((member, index) => (
            <div className="reference-pack-member-row" key={member.id}>
              <span>{index + 1}</span>
              <NodeChip node={member} />
              <button
                className="studio-secondary-button"
                disabled={isUpdating}
                onClick={() => onToggleMember(member.id, false)}
                type="button"
              >
                移除
              </button>
            </div>
          ))}
        </div>
      ) : (
        <p className="property-empty">暂无成员。</p>
      )}
      <p className="studio-section-label">Candidates</p>
      {candidates.length ? (
        <div className="reference-pack-candidate-list">
          {candidates.map((candidate) => (
            <button
              className="reference-pack-candidate-row"
              disabled={isUpdating}
              key={candidate.id}
              onClick={() => onToggleMember(candidate.id, true)}
              type="button"
            >
              <NodeChip node={candidate} />
              <span>添加</span>
            </button>
          ))}
        </div>
      ) : (
        <p className="property-empty">没有可添加的非 Pack 节点。</p>
      )}
    </div>
  );
}
```

- [ ] **Step 6: Add styles**

Add to `apps/web/src/main.css` near property panel styles:

```css
.reference-pack-member-list,
.reference-pack-candidate-list {
  display: grid;
  gap: 6px;
}

.reference-pack-member-row,
.reference-pack-candidate-row {
  align-items: center;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--fg-primary) 4%, transparent);
  display: grid;
  gap: 8px;
  min-width: 0;
  padding: 8px;
}

.reference-pack-member-row {
  grid-template-columns: auto minmax(0, 1fr) auto;
}

.reference-pack-candidate-row {
  color: var(--fg-secondary);
  cursor: pointer;
  font: inherit;
  grid-template-columns: minmax(0, 1fr) auto;
  text-align: left;
}

.reference-pack-candidate-row:disabled,
.reference-pack-member-row button:disabled {
  cursor: wait;
  opacity: 0.6;
}
```

- [ ] **Step 7: Run frontend build**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

---

## Task 4: Wire Reference Pack Query And Mutation In Studio Page

**Files:**
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`

- [ ] **Step 1: Import API helpers**

Add imports:

```ts
  fetchReferencePackItems,
  replaceReferencePackItems,
```

from `../lib/api`.

- [ ] **Step 2: Add selected pack items query**

After `selectedNodeProductionStateQuery`, add:

```ts
const selectedReferencePackItemsQuery = useQuery({
  queryKey: ["reference-pack", selectedNodeId, "items"],
  queryFn: () => fetchReferencePackItems(selectedNodeId ?? ""),
  enabled: selectedNode?.node_type === "reference_pack",
});
```

- [ ] **Step 3: Add replace mutation**

Add:

```ts
const replaceReferencePackItemsMutation = useMutation({
  mutationFn: async (input: { packNodeId: string; memberNodeIds: string[] }) =>
    replaceReferencePackItems(input.packNodeId, input.memberNodeIds),
  onSuccess: (_items, input) => {
    void queryClient.invalidateQueries({
      queryKey: ["reference-pack", input.packNodeId, "items"],
    });
    void queryClient.invalidateQueries({
      queryKey: ["workspace", id, "canvas"],
    });
    void queryClient.invalidateQueries({
      queryKey: ["node", input.packNodeId, "production-state"],
    });
  },
  onError: (_error, input) => {
    void queryClient.invalidateQueries({
      queryKey: ["reference-pack", input.packNodeId, "items"],
    });
    void queryClient.invalidateQueries({
      queryKey: ["workspace", id, "canvas"],
    });
  },
});
```

- [ ] **Step 4: Pass new props to `PropertyPanel`**

Add:

```tsx
isReferencePackItemsLoading={selectedReferencePackItemsQuery.isFetching}
isUpdatingReferencePackItems={replaceReferencePackItemsMutation.isPending}
referencePackItems={selectedReferencePackItemsQuery.data ?? []}
onReplaceReferencePackItems={(packNodeId, memberNodeIds) =>
  replaceReferencePackItemsMutation.mutate({ packNodeId, memberNodeIds })
}
```

- [ ] **Step 5: Invalidate downstream selected state after membership update**

If browser smoke shows downstream stale cards update but selected downstream property panel remains stale-free, add a broad production state invalidation:

```ts
void queryClient.invalidateQueries({
  predicate: (query) =>
    Array.isArray(query.queryKey) &&
    query.queryKey[0] === "node" &&
    query.queryKey[2] === "production-state",
});
```

Use this only if necessary; start with targeted invalidation plus canvas invalidation.

- [ ] **Step 6: Run frontend tests and build**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

---

## Task 5: Reference Pack E2E Smoke And Final Verification

**Files:**
- No new source file expected.
- Optional: add a short smoke note under this plan after execution if the browser flow reveals any stable manual steps worth preserving.

- [ ] **Step 1: Backend API smoke for pack intent expansion**

Use a short local API script against the printed backend port to:

1. Register a user.
2. Create Studio workspace.
3. Create and run Image A and Image B as `text_to_image` / `mock-image-only`.
4. Create Reference Pack P.
5. Replace P members with A and B.
6. Create Image C.
7. Create dependency P -> C.
8. Configure and run C.
9. Fetch C production state.
10. Assert latest job intent has:
    - one `InputRef` with `kind === "reference_pack"` and `node_id === P`,
    - two `InputRef` entries with `kind === "reference_pack_member"`.

Run with the actual backend port from `CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh`. If sandbox blocks localhost, rerun with escalation.

- [ ] **Step 2: Browser smoke Reference Pack UI**

Use the same workspace or create a fresh one:

1. Open the printed Vite URL.
2. Log in as the smoke user.
3. Open the Studio route.
4. Select Pack P.
5. Confirm property panel shows A and B in Members.
6. Confirm candidate list excludes P and other packs.
7. Remove B from P.
8. Confirm P canvas card summary updates from 2 members to 1 member.
9. Confirm C shows stale badge and property panel stale reason `reference_pack_membership_changed`.
10. Check browser console for application errors.

- [ ] **Step 3: Stop runtime**

Run:

```bash
CLIPANVIL_DEV_NAME=<printed-profile> ./scripts/dev-stop.sh
```

- [ ] **Step 4: Final verification**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: all commands pass.

---

## Self-Review

- Spec coverage: creation already exists from M5.1, member management and pack card summary are covered by Tasks 1-4, dependency-to-pack and stale behavior are covered by Task 5.
- Scope: Prompt `@` candidate behavior is explicitly deferred to M5.5; M5.4 only keeps packs usable as dependency inputs.
- Test coverage: backend response conversion tests, frontend pure helper tests, TypeScript/build checks, API smoke, browser smoke, and final verification are all included.
- Current-state fit: no migration or sqlc generation is planned because the M4.5 Reference Pack schema/API already exists.
