# M5.2 Production Property Panel And Manual Run Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the Studio property panel into the first usable single-node production control surface: operation/model/params configuration, production state display, run, failure, and retry.

**Architecture:** Keep M5.2 focused on one selected node. Reuse M4 run/state/model endpoints and the M5.1 frontend API contracts, add a small backend validation fix for `supported_input_node_types`, and isolate frontend production-panel decisions in pure helpers before wiring React UI. Leave current winner canvas previews, version history polish, stale UX, Prompt `@`, and Reference Pack membership to M5.3-M5.5.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, React 19, TypeScript 6, TanStack Query, Vite 8, Node test runner.

---

## Current-State Notes

- M5.1 is present in the working tree: `MediaType` includes `reference_pack`, frontend API helpers exist, and Studio can create a Reference Pack node.
- `PropertyPanel.tsx` is still an M1-style inspector. It only shows status, group, dependencies, and prompt text.
- `WorkspaceDetailPage.tsx` owns selected node state, canvas query cache, and node update mutations. `PropertyPanel.tsx` exists but is not mounted in the Studio page yet; M5.2 must add a right-side dock for it.
- `fetchNodeProductionState`, `fetchModelCapabilities`, `runNode`, and `retryJob` exist in `apps/web/src/lib/api.ts`.
- Backend `RunHandler` already exposes production state, run, retry, versions, jobs, stale reasons, and sandbox job responses.
- Backend capability validation currently runs before `loadInputContext`, so it cannot use `supported_input_node_types`. M5.2 must move input context loading before validation and reject unsupported concrete input node types.

## Scope Boundaries

M5.2 includes:

- Production sections in the selected-node property panel.
- Operation/model/params editing from the property panel.
- Single-node run and retry controls.
- Latest job, current version, and concise error display in the property panel.
- Frontend warning for operation/model incompatibility.
- Backend validation for `supported_input_node_types`.

M5.2 does not include:

- Canvas preview driven by current winner. That is M5.3.
- Full version list UI. M5.2 only shows current version and latest job summary.
- Stale badges and stale reason workflow. That is M5.3.
- Reference Pack membership management. That is M5.4.
- Prompt `@` editor, `prompt_refs`, and `prompt_rich`. That is M5.5.
- Multi-node batch run or automatic cascade rerun.

---

## File Structure

- Modify `apps/server/internal/production/capability.go`
  - Validate concrete input node types against `Capability.SupportedInputNodeTypes`.
- Modify `apps/server/internal/production/service.go`
  - Load `InputRefs` before `ValidateCapability`.
- Modify `apps/server/internal/production/service_test.go`
  - Add a failing test for unsupported input node type validation.
- Create `apps/web/src/lib/productionPanel.ts`
  - Pure helpers for operation labels, model filtering, selected capability, params defaults, run disabled reasons, and job summaries.
- Create `apps/web/src/lib/productionPanel.test.mjs`
  - Node tests for helper behavior.
- Modify `apps/web/tsconfig.test.json`
  - Include `src/lib/productionPanel.ts`.
- Modify `apps/web/package.json`
  - Add `src/lib/productionPanel.test.mjs` to `test:connections`.
- Modify `apps/web/src/lib/api.ts`
  - Preserve failed run response payload in `ApiError`, so the UI can show latest job errors immediately when the backend returns a failed `runNodeResponse`.
- Modify `apps/web/src/components/PropertyPanel.tsx`
  - Turn the existing unused inspector into a mounted production inspector.
  - Add production state, model capabilities, operation/model/params controls, run/retry buttons, latest job display, and current version summary.
  - Keep group/edge sections read-only when optional editing callbacks are not supplied.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`
  - Fetch model capabilities and selected-node production state.
  - Add node production config update, run, and retry mutations.
  - Invalidate canvas and production state after config/run/retry.
- Modify `apps/web/src/main.css`
  - Add compact production-panel styles that fit the existing inspector visual system.

---

## Task 1: Enforce Supported Input Node Types In Backend

**Files:**
- Modify: `apps/server/internal/production/capability.go`
- Modify: `apps/server/internal/production/service.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Add a failing capability validator test**

Append this test near the other capability validator tests in `apps/server/internal/production/service_test.go`:

```go
func TestCapabilityValidatorRejectsUnsupportedInputNodeType(t *testing.T) {
	capability := Capability{
		ProviderID:              "mock",
		ModelID:                 "mock-image-only",
		OutputTypes:             []string{"image"},
		SupportedOperations:     []string{"text_to_image"},
		SupportedInputNodeTypes: []string{"text"},
		Limits:                  CapabilityLimits{MaxAttempts: 3},
	}
	intent := GenerationIntent{
		OutputType:     "image",
		OperationType:  "text_to_image",
		PromptTemplate: "make an image",
		InputRefs: []InputRef{
			{Kind: "dependency", NodeType: "video"},
		},
		Model:  ModelSpec{Provider: "mock", ModelID: "mock-image-only"},
		Params: map[string]any{},
	}

	err := ValidateCapability(intent, capability)
	if !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("error = %v, want ErrCapabilityMismatch", err)
	}
	if !strings.Contains(err.Error(), "does not support input node type video") {
		t.Fatalf("error = %q, want unsupported input node type message", err.Error())
	}
}
```

Also add `strings` to the import list if it is not already present:

```go
import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)
```

- [ ] **Step 2: Run the focused backend test and verify failure**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/production -run TestCapabilityValidatorRejectsUnsupportedInputNodeType
```

Expected: FAIL because `ValidateCapability` does not inspect `intent.InputRefs`.

- [ ] **Step 3: Implement input type validation**

In `apps/server/internal/production/capability.go`, add this block after the supported operation check and before prompt length validation:

```go
	if len(capability.SupportedInputNodeTypes) > 0 {
		for _, ref := range intent.InputRefs {
			if ref.NodeType == "" || ref.Kind == "reference_pack" {
				continue
			}
			if !contains(capability.SupportedInputNodeTypes, ref.NodeType) {
				return fmt.Errorf("%w: model %s/%s does not support input node type %s", ErrCapabilityMismatch, capability.ProviderID, capability.ModelID, ref.NodeType)
			}
		}
	}
```

This deliberately skips the Reference Pack container input ref and validates concrete dependency/member refs.

- [ ] **Step 4: Move input loading before validation**

In `apps/server/internal/production/service.go`, change `runNodeAttempt` so `loadInputContext` happens before `ValidateCapability`:

```go
	inputs, err := loadInputContext(ctx, s.queries, node.ID)
	if err != nil {
		return RunResult{}, err
	}
	intent.InputRefs = inputs.InputRefs

	maxAttempts := maxAttemptsForRun(options, capability)
	if err := ValidateCapability(intent, capability); err != nil {
		if _, jobErr := s.createFailedJob(ctx, node, intent, err, pgtype.UUID{}, 1, maxAttempts); jobErr != nil {
			return RunResult{}, jobErr
		}
		return RunResult{}, err
	}
```

Remove the later duplicate `loadInputContext` / `intent.InputRefs = inputs.InputRefs` block and keep `inputHash` computed from the already-loaded `inputs`.

- [ ] **Step 5: Run backend production tests**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/production
```

Expected: PASS.

- [ ] **Step 6: Commit backend validation**

```bash
git add apps/server/internal/production/capability.go apps/server/internal/production/service.go apps/server/internal/production/service_test.go
git commit -m "fix: validate production input node types"
```

---

## Task 2: Add Frontend Production Panel Helpers

**Files:**
- Create: `apps/web/src/lib/productionPanel.ts`
- Create: `apps/web/src/lib/productionPanel.test.mjs`
- Modify: `apps/web/tsconfig.test.json`
- Modify: `apps/web/package.json`

- [ ] **Step 1: Add failing frontend helper tests**

Create `apps/web/src/lib/productionPanel.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  capabilitiesForNode,
  defaultOperationForNode,
  formatJobAttempt,
  modelParamsForCapability,
  runDisabledReason,
  selectedCapabilityKey,
} from "../../dist-test/lib/productionPanel.js";

const node = {
  id: "node-1",
  workspace_id: "workspace",
  node_type: "text",
  title: "文案",
  prompt: "write a line",
  status: "draft",
  canvas_x: 0,
  canvas_y: 0,
  canvas_w: 220,
  canvas_h: 132,
  created_at: "2026-06-19T00:00:00Z",
  updated_at: "2026-06-19T00:00:00Z",
};

const capabilities = [
  {
    provider_id: "mock",
    model_id: "mock-text",
    display_name: "Mock Text",
    output_types: ["text"],
    supported_operations: ["text_generation"],
    supported_input_node_types: ["text"],
    limits: { max_attempts: 3 },
    pricing: {},
    defaults: { temperature: 0.2 },
    enabled: true,
  },
  {
    provider_id: "mock",
    model_id: "mock-video",
    display_name: "Mock Video",
    output_types: ["video"],
    supported_operations: ["text_to_video", "image_to_video"],
    supported_input_node_types: ["text", "image"],
    limits: { durations_sec: [4, 5, 8], max_attempts: 3 },
    pricing: {},
    defaults: { duration_sec: 5 },
    enabled: true,
  },
];

describe("production panel helpers", () => {
  it("chooses a node-type aware default operation", () => {
    assert.equal(defaultOperationForNode({ ...node, node_type: "text" }), "text_generation");
    assert.equal(defaultOperationForNode({ ...node, node_type: "image" }), "text_to_image");
    assert.equal(defaultOperationForNode({ ...node, node_type: "video" }), "text_to_video");
    assert.equal(defaultOperationForNode({ ...node, node_type: "reference_pack" }), "collect_references");
  });

  it("filters capabilities by output type and operation", () => {
    assert.deepEqual(
      capabilitiesForNode({ ...node, node_type: "video" }, capabilities, "text_to_video").map(
        (capability) => capability.model_id,
      ),
      ["mock-video"],
    );
    assert.deepEqual(
      capabilitiesForNode(node, capabilities, "text_generation").map(
        (capability) => capability.model_id,
      ),
      ["mock-text"],
    );
  });

  it("picks an explicit model when it remains compatible", () => {
    assert.equal(
      selectedCapabilityKey(
        { ...node, model_provider: "mock", model_id: "mock-text" },
        capabilities,
      ),
      "mock::mock-text",
    );
  });

  it("merges model params over capability defaults", () => {
    assert.deepEqual(
      modelParamsForCapability(
        { ...node, model_params: { temperature: 0.7 } },
        capabilities[0],
      ),
      { temperature: 0.7 },
    );
  });

  it("prevents running reference packs and running nodes", () => {
    assert.equal(
      runDisabledReason({ ...node, node_type: "reference_pack" }, null, capabilities),
      "Reference Pack 在 M5.4 管理成员，不在这里运行。",
    );
    assert.equal(
      runDisabledReason({ ...node, status: "running" }, null, capabilities),
      "节点正在运行。",
    );
  });

  it("formats retry attempts", () => {
    assert.equal(formatJobAttempt({ attempt: 2, max_attempts: 3 }), "Attempt 2 / 3");
  });
});
```

- [ ] **Step 2: Include the helper in the test TypeScript build**

In `apps/web/tsconfig.test.json`, add:

```json
"src/lib/productionPanel.ts"
```

to the `include` array.

- [ ] **Step 3: Add the test file to `test:connections`**

In `apps/web/package.json`, append:

```json
"src/lib/productionPanel.test.mjs"
```

to the `node --test` file list in the `test:connections` script.

- [ ] **Step 4: Run frontend focused tests and verify failure**

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL because `productionPanel.ts` does not exist.

- [ ] **Step 5: Implement production panel helpers**

Create `apps/web/src/lib/productionPanel.ts`:

```ts
import type {
  GenerationJob,
  MediaNode,
  ModelCapability,
  NodeProductionState,
  OperationType,
} from "./api";

export const operationLabels: Record<OperationType, string> = {
  manual: "手动",
  upload: "上传素材",
  collect_references: "收集参考",
  text_generation: "文本生成",
  text_to_image: "文生图",
  image_to_image: "图生图",
  multi_image_to_image: "多图生图",
  text_to_video: "文生视频",
  image_to_video: "图生视频",
  video_to_video: "视频改写",
  multi_reference_to_video: "多参考生视频",
  extract_first_frame: "提取首帧",
  extract_last_frame: "提取尾帧",
};

const defaultOperationByNodeType: Record<MediaNode["node_type"], OperationType> = {
  text: "text_generation",
  image: "text_to_image",
  video: "text_to_video",
  audio: "manual",
  reference_pack: "collect_references",
};

export function defaultOperationForNode(node: Pick<MediaNode, "node_type" | "operation_type">) {
  if (isKnownOperation(node.operation_type)) {
    return node.operation_type;
  }
  return defaultOperationByNodeType[node.node_type];
}

export function capabilitiesForNode(
  node: Pick<MediaNode, "node_type">,
  capabilities: ModelCapability[],
  operation: string,
) {
  return capabilities.filter(
    (capability) =>
      capability.enabled &&
      capability.output_types.includes(node.node_type) &&
      capability.supported_operations.includes(operation),
  );
}

export function capabilityKey(capability: Pick<ModelCapability, "provider_id" | "model_id">) {
  return `${capability.provider_id}::${capability.model_id}`;
}

export function selectedCapabilityKey(
  node: Pick<MediaNode, "node_type" | "operation_type" | "model_provider" | "model_id">,
  capabilities: ModelCapability[],
) {
  const operation = defaultOperationForNode(node);
  const compatible = capabilitiesForNode(node, capabilities, operation);
  const explicit = compatible.find(
    (capability) =>
      capability.provider_id === node.model_provider &&
      capability.model_id === node.model_id,
  );
  return explicit ? capabilityKey(explicit) : capabilityKey(compatible[0]);
}

export function splitCapabilityKey(key: string) {
  const [provider, modelId] = key.split("::");
  return { provider, modelId };
}

export function modelParamsForCapability(
  node: Pick<MediaNode, "model_params">,
  capability?: Pick<ModelCapability, "defaults">,
) {
  return {
    ...(capability?.defaults ?? {}),
    ...objectParamMap(node.model_params),
  };
}

export function runDisabledReason(
  node: MediaNode,
  state: NodeProductionState | null,
  capabilities: ModelCapability[],
) {
  if (node.node_type === "reference_pack") {
    return "Reference Pack 在 M5.4 管理成员，不在这里运行。";
  }
  if (node.operation_type === "upload" || node.asset_id) {
    return "上传素材节点不需要重新运行。";
  }
  if (node.status === "running" || state?.latest_job?.status === "running") {
    return "节点正在运行。";
  }
  const operation = defaultOperationForNode(node);
  if (capabilitiesForNode(node, capabilities, operation).length === 0) {
    return "没有兼容当前节点类型和 Operation 的模型。";
  }
  return null;
}

export function formatJobAttempt(job: Pick<GenerationJob, "attempt" | "max_attempts">) {
  return `Attempt ${job.attempt} / ${job.max_attempts}`;
}

function isKnownOperation(value: unknown): value is OperationType {
  return typeof value === "string" && value in operationLabels;
}

function objectParamMap(value: unknown) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}
```

- [ ] **Step 6: Run focused frontend tests**

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

- [ ] **Step 7: Commit frontend helpers**

```bash
git add apps/web/src/lib/productionPanel.ts apps/web/src/lib/productionPanel.test.mjs apps/web/tsconfig.test.json apps/web/package.json
git commit -m "test: cover studio production panel helpers"
```

---

## Task 3: Preserve Failed Run Payloads In API Errors

**Files:**
- Modify: `apps/web/src/lib/api.ts`

- [ ] **Step 1: Extend `ApiError` with response data**

Replace the `ApiError` class with:

```ts
export class ApiError extends Error {
  status: number;
  data: unknown;

  constructor(status: number, message: string, data?: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.data = data;
  }
}
```

- [ ] **Step 2: Preserve non-OK JSON payloads**

Replace the `!response.ok` block in `apiFetch` with:

```ts
  if (!response.ok) {
    let data: unknown;
    let message = "请求失败";
    try {
      data = await response.json();
      if (isErrorPayload(data)) {
        message = data.error;
      } else if (isRunNodeErrorPayload(data)) {
        message = data.job.error_message || data.job.error_code || message;
      }
    } catch {
      message = response.statusText || message;
    }
    throw new ApiError(response.status, message, data);
  }
```

Add these helpers below `apiFetch`:

```ts
function isErrorPayload(value: unknown): value is { error: string } {
  return (
    Boolean(value) &&
    typeof value === "object" &&
    "error" in value &&
    typeof (value as { error?: unknown }).error === "string"
  );
}

function isRunNodeErrorPayload(value: unknown): value is RunNodeResponse {
  return (
    Boolean(value) &&
    typeof value === "object" &&
    "job" in value &&
    typeof (value as { job?: { error_message?: unknown } }).job === "object"
  );
}
```

- [ ] **Step 3: Run TypeScript build**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [ ] **Step 4: Commit API error support**

```bash
git add apps/web/src/lib/api.ts
git commit -m "feat: preserve production run error payloads"
```

---

## Task 4: Wire Production State And Mutations Into Studio Page

**Files:**
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/components/PropertyPanel.tsx`

- [ ] **Step 1: Import M5.2 API helpers**

In `WorkspaceDetailPage.tsx`, add these imports from `../lib/api`:

```ts
  fetchModelCapabilities,
  fetchNodeProductionState,
  retryJob,
  runNode,
  type ModelCapability,
  type NodeProductionState,
```

Also import the panel:

```ts
import { PropertyPanel } from "../components/PropertyPanel";
```

- [ ] **Step 2: Fetch model capabilities**

After `canvasQuery`, add:

```ts
  const modelCapabilitiesQuery = useQuery({
    queryKey: ["model-capabilities"],
    queryFn: fetchModelCapabilities,
  });
```

- [ ] **Step 3: Fetch selected-node production state**

After `selectedNode`, add:

```ts
  const selectedNodeProductionStateQuery = useQuery({
    queryKey: ["node", selectedNodeId, "production-state"],
    queryFn: () => fetchNodeProductionState(selectedNodeId ?? ""),
    enabled: Boolean(selectedNodeId),
  });
```

- [ ] **Step 4: Add a generic node patch mutation**

Add this mutation near the other mutations:

```ts
  const updateNodeMutation = useMutation({
    mutationFn: async (input: {
      nodeId: string;
      patch: Parameters<typeof updateMediaNode>[1];
    }) => updateMediaNode(input.nodeId, input.patch),
    onSuccess: (node) => {
      nodeSnapshotsRef.current.set(node.id, node);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (payload) => replaceCanvasNode(payload, node),
      );
      editorRef.current?.store.mergeRemoteChanges(() => {
        editorRef.current?.updateShapes([
          {
            id: shapeIdForNode(node.id),
            type: "media",
            props: {
              prompt: node.prompt,
              status: node.status,
              title: node.title,
            },
          },
        ]);
      });
      void queryClient.invalidateQueries({
        queryKey: ["node", node.id, "production-state"],
      });
    },
    onError: (_error, input) => {
      void queryClient.invalidateQueries({
        queryKey: ["node", input.nodeId, "production-state"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
    },
  });
```

- [ ] **Step 5: Add run and retry mutations**

Add these mutations:

```ts
  const runNodeMutation = useMutation({
    mutationFn: async (nodeId: string) => runNode(nodeId),
    onMutate: (nodeId) => {
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (payload) => {
          const node = optimisticRunningNode(payload, nodeId);
          return node ? replaceCanvasNode(payload, node) : payload;
        },
      );
    },
    onSettled: (_data, _error, nodeId) => {
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["node", nodeId, "production-state"],
      });
    },
  });

  const retryJobMutation = useMutation({
    mutationFn: async (jobId: string) => retryJob(jobId),
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["node", selectedNodeId, "production-state"],
      });
    },
  });
```

Add this helper near the other canvas payload helpers:

```ts
function optimisticRunningNode(payload: CanvasPayload | undefined, nodeId: string) {
  const node = payload?.nodes.find((item) => item.id === nodeId);
  return node ? { ...node, status: "running" as const } : undefined;
}
```

- [ ] **Step 6: Mount the right-side production inspector**

Inside `.studio-canvas-frame`, after `AutoLayoutControls`, mount:

```tsx
        <div className="studio-property-dock">
          <PropertyPanel
            edges={canvasQuery.data?.edges ?? []}
            groups={groups}
            isModelCapabilitiesLoading={modelCapabilitiesQuery.isLoading}
            isProductionStateLoading={selectedNodeProductionStateQuery.isLoading}
            isRetryingJob={retryJobMutation.isPending}
            isRunningNode={runNodeMutation.isPending}
            isUpdatingNode={updateNodeMutation.isPending}
            modelCapabilities={modelCapabilitiesQuery.data ?? []}
            nodeProductionState={selectedNodeProductionStateQuery.data ?? null}
            nodes={nodes}
            selectedEdgeId={selectedEdgeId}
            selectedGroupId={selectedGroupId}
            selectedNodeId={selectedNodeId}
            onChangeNodeGroup={(nodeId, groupId) =>
              updateNodeMutation.mutate({
                nodeId,
                patch: { group_id: groupId },
              })
            }
            onDeleteEdge={deleteEdgeById}
            onRetryJob={(jobId) => retryJobMutation.mutate(jobId)}
            onRunNode={(nodeId) => runNodeMutation.mutate(nodeId)}
            onUpdateNode={(nodeId, patch) =>
              updateNodeMutation.mutate({ nodeId, patch })
            }
          />
        </div>
```

Do not wire group create/delete/member editing in M5.2. Existing group interactions remain sidebar/drag behaviors; group inspector editing can be re-enabled after this production panel is stable.

- [ ] **Step 7: Add dock CSS**

In `apps/web/src/main.css`, add:

```css
.studio-property-dock {
  pointer-events: none;
  position: absolute;
  right: 16px;
  top: 88px;
  z-index: 80;
  width: min(360px, calc(100vw - 32px));
  max-height: calc(100vh - 132px);
}

.studio-property-dock > .property-panel {
  pointer-events: all;
  max-height: calc(100vh - 132px);
}
```

- [ ] **Step 8: Type-check the page wiring**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS after `PropertyPanel` props are updated in Task 5.

Do not commit Task 4 before Task 5 compiles.

---

## Task 5: Build The Production Node Property Panel UI

**Files:**
- Modify: `apps/web/src/components/PropertyPanel.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Extend `PropertyPanelProps`**

Add these props:

```ts
import type {
  MediaEdge,
  MediaGroup,
  MediaNode,
  ModelCapability,
  NodeProductionState,
  updateMediaNode,
} from "../lib/api";
import {
  capabilitiesForNode,
  capabilityKey,
  defaultOperationForNode,
  formatJobAttempt,
  modelParamsForCapability,
  operationLabels,
  runDisabledReason,
  selectedCapabilityKey,
  splitCapabilityKey,
} from "../lib/productionPanel";

type NodePatch = Parameters<typeof updateMediaNode>[1];
```

Then change the callback part of `PropertyPanelProps` to make group/member editing optional and add production callbacks:

```ts
  isModelCapabilitiesLoading: boolean;
  isProductionStateLoading: boolean;
  isRetryingJob: boolean;
  isRunningNode: boolean;
  isUpdatingNode: boolean;
  modelCapabilities: ModelCapability[];
  nodeProductionState: NodeProductionState | null;
  onAddGroupMember?: (groupId: string, nodeId: string) => void;
  onChangeNodeGroup: (nodeId: string, groupId: string | null) => void;
  onDeleteEdge?: (edgeId: string) => void;
  onDeleteGroup?: (groupId: string) => void;
  onRemoveGroupMember?: (groupId: string, nodeId: string) => void;
  onRenameGroup?: (groupId: string, name: string) => void;
  onRetryJob: (jobId: string) => void;
  onRunNode: (nodeId: string) => void;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
```

- [ ] **Step 2: Make group and edge sections safe without edit callbacks**

In `GroupPropertyPanel`, hide edit-only controls when the callback is absent:

```tsx
      {onRenameGroup ? (
        <label className="property-field">
          <span>名称</span>
          <input
            defaultValue={group.name}
            key={group.id}
            onBlur={(event) => {
              const nextName = event.currentTarget.value.trim();
              if (nextName && nextName !== group.name) {
                onRenameGroup(group.id, nextName);
              }
            }}
          />
        </label>
      ) : null}
```

Apply the same pattern to add/remove/delete group controls and the edge delete button. Keep read-only labels, member rows, and dependency direction visible.

- [ ] **Step 3: Pass production props into `NodePropertyPanel`**

When `selectedNode` exists, pass the new props through:

```tsx
      <NodePropertyPanel
        edges={edges}
        groups={groups}
        isModelCapabilitiesLoading={isModelCapabilitiesLoading}
        isProductionStateLoading={isProductionStateLoading}
        isRetryingJob={isRetryingJob}
        isRunningNode={isRunningNode}
        isUpdatingNode={isUpdatingNode}
        modelCapabilities={modelCapabilities}
        node={selectedNode}
        nodeProductionState={nodeProductionState}
        nodes={nodes}
        onChangeNodeGroup={onChangeNodeGroup}
        onRetryJob={onRetryJob}
        onRunNode={onRunNode}
        onUpdateNode={onUpdateNode}
      />
```

- [ ] **Step 4: Add operation and model selectors**

Inside `NodePropertyPanel`, compute:

```ts
  const operation = defaultOperationForNode(node);
  const compatibleCapabilities = capabilitiesForNode(
    node,
    modelCapabilities,
    operation,
  );
  const selectedModelKey = selectedCapabilityKey(node, modelCapabilities);
  const selectedCapability = compatibleCapabilities.find(
    (capability) => capabilityKey(capability) === selectedModelKey,
  );
  const modelParams = modelParamsForCapability(node, selectedCapability);
  const disabledReason = runDisabledReason(
    node,
    nodeProductionState,
    modelCapabilities,
  );
```

Add this operation field after the group field:

```tsx
      <label className="property-field">
        <span>Operation</span>
        <select
          disabled={node.node_type === "reference_pack" || isUpdatingNode}
          onChange={(event) =>
            onUpdateNode(node.id, { operation_type: event.currentTarget.value })
          }
          value={operation}
        >
          {Object.entries(operationLabels).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
      </label>
```

Add this model field after operation:

```tsx
      <label className="property-field">
        <span>Model</span>
        <select
          disabled={
            isModelCapabilitiesLoading ||
            isUpdatingNode ||
            compatibleCapabilities.length === 0
          }
          onChange={(event) => {
            const { provider, modelId } = splitCapabilityKey(
              event.currentTarget.value,
            );
            const capability = compatibleCapabilities.find(
              (item) =>
                item.provider_id === provider && item.model_id === modelId,
            );
            onUpdateNode(node.id, {
              model_provider: provider,
              model_id: modelId,
              model_params: modelParamsForCapability(node, capability),
            });
          }}
          value={selectedModelKey}
        >
          {compatibleCapabilities.length > 0 ? (
            compatibleCapabilities.map((capability) => (
              <option key={capabilityKey(capability)} value={capabilityKey(capability)}>
                {capability.display_name}
              </option>
            ))
          ) : (
            <option value="">没有兼容模型</option>
          )}
        </select>
      </label>
```

- [ ] **Step 5: Add params controls**

Add a compact params section. M5.2 supports the seeded mock params explicitly:

```tsx
      <div className="property-section">
        <p className="studio-section-label">Params</p>
        {"temperature" in modelParams ? (
          <label className="property-field">
            <span>Temperature</span>
            <input
              min="0"
              max="1"
              onBlur={(event) =>
                onUpdateNode(node.id, {
                  model_params: {
                    ...modelParams,
                    temperature: Number(event.currentTarget.value),
                  },
                })
              }
              step="0.1"
              type="number"
              defaultValue={String(modelParams.temperature)}
            />
          </label>
        ) : null}
        {"duration_sec" in modelParams ? (
          <label className="property-field">
            <span>Duration</span>
            <select
              onChange={(event) =>
                onUpdateNode(node.id, {
                  model_params: {
                    ...modelParams,
                    duration_sec: Number(event.currentTarget.value),
                  },
                })
              }
              value={String(modelParams.duration_sec)}
            >
              {[4, 5, 8].map((duration) => (
                <option key={duration} value={duration}>
                  {duration}s
                </option>
              ))}
            </select>
          </label>
        ) : null}
        <label className="property-check-row">
          <input
            checked={Boolean(modelParams.mock_fail)}
            onChange={(event) =>
              onUpdateNode(node.id, {
                model_params: {
                  ...modelParams,
                  mock_fail: event.currentTarget.checked,
                },
              })
            }
            type="checkbox"
          />
          <span>Mock failure</span>
        </label>
      </div>
```

- [ ] **Step 6: Add editable prompt textarea**

Replace the read-only prompt paragraph with:

```tsx
      <label className="property-field">
        <span>Prompt</span>
        <textarea
          defaultValue={node.prompt}
          key={`${node.id}-${node.updated_at}`}
          onBlur={(event) => {
            if (event.currentTarget.value !== node.prompt) {
              onUpdateNode(node.id, { prompt: event.currentTarget.value });
            }
          }}
          placeholder="输入生成文本、画面描述或旁白方向"
        />
      </label>
```

- [ ] **Step 7: Add run and retry controls**

Add a Run section after Inputs:

```tsx
      <div className="property-section">
        <p className="studio-section-label">Run</p>
        {disabledReason ? (
          <p className="property-empty">{disabledReason}</p>
        ) : null}
        <button
          className="studio-secondary-button property-run-button"
          disabled={Boolean(disabledReason) || isRunningNode}
          onClick={() => onRunNode(node.id)}
          type="button"
        >
          {isRunningNode ? "运行中" : "运行节点"}
        </button>
        {nodeProductionState?.latest_job?.status === "failed" ? (
          <button
            className="studio-secondary-button"
            disabled={isRetryingJob}
            onClick={() => onRetryJob(nodeProductionState.latest_job!.id)}
            type="button"
          >
            {isRetryingJob ? "重试中" : "重试失败任务"}
          </button>
        ) : null}
      </div>
```

- [ ] **Step 8: Add current version and latest job summaries**

Add this after the Run section:

```tsx
      <div className="property-section">
        <p className="studio-section-label">Current Version</p>
        {isProductionStateLoading ? (
          <p className="property-empty">正在读取 production state。</p>
        ) : nodeProductionState?.current_version ? (
          <dl className="property-list">
            <div>
              <dt>Version</dt>
              <dd>v{nodeProductionState.current_version.version_no}</dd>
            </div>
            <div>
              <dt>Asset</dt>
              <dd>{nodeProductionState.current_version.asset?.type ?? "output"}</dd>
            </div>
          </dl>
        ) : (
          <p className="property-empty">尚无 current winner。</p>
        )}
      </div>

      <div className="property-section">
        <p className="studio-section-label">Latest Job</p>
        {nodeProductionState?.latest_job ? (
          <dl className="property-list">
            <div>
              <dt>Status</dt>
              <dd>{nodeProductionState.latest_job.status}</dd>
            </div>
            <div>
              <dt>Attempt</dt>
              <dd>{formatJobAttempt(nodeProductionState.latest_job)}</dd>
            </div>
            <div>
              <dt>Model</dt>
              <dd>
                {nodeProductionState.latest_job.provider}/
                {nodeProductionState.latest_job.model_id}
              </dd>
            </div>
          </dl>
        ) : (
          <p className="property-empty">尚未运行。</p>
        )}
        {nodeProductionState?.latest_job?.error_message ? (
          <p className="property-error">
            {nodeProductionState.latest_job.error_message}
          </p>
        ) : null}
      </div>
```

- [ ] **Step 9: Add CSS for textareas, check row, run button, and error**

In `apps/web/src/main.css`, extend the existing property styles:

```css
.property-field textarea {
  width: 100%;
  min-height: 88px;
  resize: vertical;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--fg-primary) 4%, transparent);
  color: var(--fg-primary);
  font-size: 12px;
  line-height: 1.5;
  outline: none;
  padding: 9px 10px;
}

.property-field textarea:focus {
  border-color: color-mix(in srgb, var(--accent) 44%, var(--border-default));
}

.property-check-row {
  display: flex;
  min-height: 34px;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--fg-primary) 4%, transparent);
  padding: 0 10px;
  color: var(--fg-secondary);
  font-size: 12px;
}

.property-run-button {
  justify-content: center;
  border-color: color-mix(in srgb, var(--accent) 34%, var(--border-default));
}

.property-error {
  margin: 0;
  border: 1px solid color-mix(in srgb, var(--status-failed) 34%, var(--border-default));
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--status-failed) 9%, transparent);
  padding: 10px;
  color: var(--status-failed);
  font-size: 12px;
  line-height: 1.5;
}
```

- [ ] **Step 10: Run frontend build**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [ ] **Step 11: Commit UI panel**

```bash
git add apps/web/src/components/PropertyPanel.tsx apps/web/src/pages/WorkspaceDetailPage.tsx apps/web/src/main.css
git commit -m "feat: add studio production run panel"
```

---

## Task 6: Verify M5.2 Acceptance

**Files:**
- No source edits expected.

- [ ] **Step 1: Run backend tests**

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: PASS.

- [ ] **Step 2: Run frontend focused tests**

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

- [ ] **Step 3: Run frontend build and lint**

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: both PASS.

- [ ] **Step 4: Check whitespace**

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Run worktree-safe local app**

Print the profile:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Start the app:

```bash
./scripts/dev-start.sh
```

Use the Vite URL printed by the script.

- [ ] **Step 6: Browser E2E smoke**

Use a fresh test account and Studio workspace:

1. Register or log in.
2. Create a Studio Workspace.
3. Create a Text Node.
4. Select the Text Node.
5. In the property panel, enter prompt `write a compact product tagline`.
6. Select operation `文本生成`.
7. Select model `Mock Text`.
8. Click `运行节点`.
9. Confirm the panel shows latest job `succeeded`, current version `v1`, and model `mock/mock-text`.
10. Create an Image Node.
11. Select operation `文生图`.
12. Select model `Mock Image Only`.
13. Click `运行节点`.
14. Confirm latest job `succeeded` and current version exists.
15. Create a Video Node.
16. Select operation `文生视频`.
17. Select model `Mock Video`.
18. Set duration to `5s`.
19. Click `运行节点`.
20. Confirm latest job `succeeded` and current version exists.
21. Select any mock-capable node, enable `Mock failure`, and run.
22. Confirm latest job shows `failed` and an error message.
23. Click `重试失败任务`.
24. Confirm a new attempt is visible. If `Mock failure` remains enabled, the retry may fail again; the acceptance point is that retry creates the next attempt and the panel updates.
25. Select a Reference Pack node and confirm the Run section explains it cannot be run in M5.2.

- [ ] **Step 7: Stop local app**

```bash
./scripts/dev-stop.sh
```

Expected: frontend/backend for this profile are stopped; shared middleware remains running.

---

## Self-Review Checklist

- M5.2 backend gap is covered: `supported_input_node_types` is validated after input refs are loaded.
- M5.2 frontend foundation is covered: pure helpers are tested before component wiring.
- Property panel scope matches the spec: basic info, operation, prompt, model, params, inputs, run, retry, current version, latest job.
- Reference Pack run behavior is explicit and disabled.
- Upload asset rerun behavior is explicit and disabled.
- M5.3-M5.5 features are deliberately excluded from implementation tasks.
- Verification includes backend tests, frontend tests, build, lint, diff check, and browser E2E smoke.
