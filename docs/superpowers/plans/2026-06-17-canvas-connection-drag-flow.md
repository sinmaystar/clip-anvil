# Canvas Connection Drag Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build drag-to-connect canvas edges with long curved animated connection visuals.

**Architecture:** Keep edge persistence unchanged and add frontend-only interaction/visual layers. A pure geometry helper computes Bezier paths for tests and rendering. `WorkspaceDetailPage` owns drag state and edge creation; a new overlay component renders previews, target highlights, and animated saved-edge paths from existing canvas data.

**Tech Stack:** React 19, TypeScript 6, Vite 8, tldraw 5, Node built-in test runner for pure helper tests.

---

## File Structure

- Create `apps/web/src/lib/connectionGeometry.ts`: pure functions for node anchor points, cubic Bezier path strings, and overlay bounds.
- Create `apps/web/src/lib/connectionGeometry.test.mjs`: Node test runner coverage for the geometry helper by importing built JS from `apps/web/dist-test`.
- Create `apps/web/tsconfig.test.json`: emits the pure helper to `apps/web/dist-test` without changing the app build.
- Create `apps/web/src/components/ConnectionOverlay.tsx`: SVG overlay for saved animated edges, drag preview, and target highlights.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`: track pointer drag state, update drag preview, detect release target, and render `ConnectionOverlay`.
- Modify `apps/web/src/shapes/MediaShapeUtil.tsx`: make output port pointer-down start drag cleanly, keep click fallback, and expose source pointer metadata.
- Modify `apps/web/src/main.css`: add overlay animation, target highlight states, and B-style flow visuals.
- Modify `apps/web/package.json`: add a focused `test:connections` script.

## Task 1: Geometry Helper With Red-Green Test

**Files:**
- Create: `apps/web/src/lib/connectionGeometry.ts`
- Create: `apps/web/src/lib/connectionGeometry.test.mjs`
- Create: `apps/web/tsconfig.test.json`
- Modify: `apps/web/package.json`

- [ ] **Step 1: Write the failing geometry test**

Create `apps/web/src/lib/connectionGeometry.test.mjs`:

```javascript
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  connectionPath,
  mediaNodeBounds,
  outputAnchor,
  inputAnchor,
} from "../dist-test/lib/connectionGeometry.js";

describe("connection geometry", () => {
  it("uses right and left midpoints as node anchors", () => {
    const source = mediaNodeBounds({
      id: "a",
      canvas_x: 40,
      canvas_y: 60,
      canvas_w: 120,
      canvas_h: 80,
    });
    const target = mediaNodeBounds({
      id: "b",
      canvas_x: 300,
      canvas_y: 110,
      canvas_w: 140,
      canvas_h: 100,
    });

    assert.deepEqual(outputAnchor(source), { x: 160, y: 100 });
    assert.deepEqual(inputAnchor(target), { x: 300, y: 160 });
  });

  it("builds a long cubic path with horizontal pull", () => {
    const path = connectionPath({ x: 160, y: 100 }, { x: 300, y: 160 });

    assert.equal(path, "M 160 100 C 244 100, 216 160, 300 160");
  });

  it("keeps a visible curve when the target is close to the source", () => {
    const path = connectionPath({ x: 160, y: 100 }, { x: 178, y: 116 });

    assert.equal(path, "M 160 100 C 220 100, 118 116, 178 116");
  });
});
```

- [ ] **Step 2: Add the test script and test tsconfig**

Add this script to `apps/web/package.json`:

```json
"test:connections": "tsc -p tsconfig.test.json && node --test src/lib/connectionGeometry.test.mjs"
```

Create `apps/web/tsconfig.test.json`:

```json
{
  "extends": "./tsconfig.json",
  "compilerOptions": {
    "composite": false,
    "declaration": false,
    "declarationMap": false,
    "emitDeclarationOnly": false,
    "noEmit": false,
    "outDir": "dist-test",
    "rootDir": "src",
    "tsBuildInfoFile": "dist-test/tsconfig.test.tsbuildinfo"
  },
  "include": ["src/lib/connectionGeometry.ts"]
}
```

- [ ] **Step 3: Run the test and verify it fails**

Run: `pnpm --filter @clip-anvil/web test:connections`

Expected: FAIL because `apps/web/src/lib/connectionGeometry.ts` does not exist yet.

- [ ] **Step 4: Implement the geometry helper**

Create `apps/web/src/lib/connectionGeometry.ts`:

```typescript
export interface ConnectionNodeBounds {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface CanvasNodeLike {
  id: string;
  canvas_x: number;
  canvas_y: number;
  canvas_w: number;
  canvas_h: number;
}

export interface Point {
  x: number;
  y: number;
}

export function mediaNodeBounds(node: CanvasNodeLike): ConnectionNodeBounds {
  return {
    id: node.id,
    x: node.canvas_x,
    y: node.canvas_y,
    w: node.canvas_w,
    h: node.canvas_h,
  };
}

export function outputAnchor(bounds: ConnectionNodeBounds): Point {
  return {
    x: bounds.x + bounds.w,
    y: bounds.y + bounds.h / 2,
  };
}

export function inputAnchor(bounds: ConnectionNodeBounds): Point {
  return {
    x: bounds.x,
    y: bounds.y + bounds.h / 2,
  };
}

export function connectionPath(start: Point, end: Point): string {
  const distance = Math.abs(end.x - start.x);
  const pull = Math.max(60, Math.min(180, distance * 0.6));
  const c1 = { x: start.x + pull, y: start.y };
  const c2 = { x: end.x - pull, y: end.y };
  return `M ${round(start.x)} ${round(start.y)} C ${round(c1.x)} ${round(
    c1.y,
  )}, ${round(c2.x)} ${round(c2.y)}, ${round(end.x)} ${round(end.y)}`;
}

function round(value: number) {
  return Math.round(value * 100) / 100;
}
```

- [ ] **Step 5: Run the focused test and verify it passes**

Run: `pnpm --filter @clip-anvil/web test:connections`

Expected: PASS with 3 tests.

## Task 2: Drag State and Overlay Rendering

**Files:**
- Create: `apps/web/src/components/ConnectionOverlay.tsx`
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Create the overlay component**

Create `apps/web/src/components/ConnectionOverlay.tsx` with props for `nodes`, `edges`, `dragConnection`, `hoveredTargetNodeId`, and `editor`. Render a full-frame SVG using `editor.pageToViewport` for node anchors and pointer positions. Use `connectionPath` for every path.

- [ ] **Step 2: Add overlay CSS**

Add `.connection-overlay`, `.connection-overlay-path`, `.connection-overlay-flow`, `.connection-overlay-preview`, and `.connection-overlay-target` classes to `apps/web/src/main.css`. Use a visible animated dash offset and long-arc B styling.

- [ ] **Step 3: Render overlay in the canvas frame**

In `WorkspaceDetailPage.tsx`, render `ConnectionOverlay` above the tldraw host and below menus/drop zones. Pass `canvasQuery.data?.nodes`, `canvasQuery.data?.edges`, current editor, and drag state.

## Task 3: Pointer Drag Interaction

**Files:**
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/shapes/MediaShapeUtil.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Extend connection event detail**

In `MediaShapeUtil.tsx`, include `clientX` and `clientY` in `clip-anvil:connection-start` detail when pointer down starts a drag.

- [ ] **Step 2: Track drag preview**

In `WorkspaceDetailPage.tsx`, add drag state with source node id, pointer id, current page point, and dragging boolean. Update it on window `pointermove`.

- [ ] **Step 3: Detect release target**

On `pointerup`, use `document.elementFromPoint(event.clientX, event.clientY)?.closest(".media-node-shell")`. If the target is another node, clear drag state and call `createDependencyEdge`.

- [ ] **Step 4: Preserve click fallback**

Keep the pending click-to-connect path when `pointerId` is null or no meaningful drag happened.

- [ ] **Step 5: Highlight valid targets**

Set a `data-connection-target` attribute or overlay highlight for nodes other than the source while dragging.

## Task 4: Verification and Browser Smoke

**Files:**
- Verify: frontend tests/build/lint and browser behavior

- [ ] **Step 1: Run focused geometry tests**

Run: `pnpm --filter @clip-anvil/web test:connections`

Expected: PASS.

- [ ] **Step 2: Run frontend build**

Run: `pnpm --filter @clip-anvil/web... build`

Expected: PASS.

- [ ] **Step 3: Run frontend lint**

Run: `pnpm --filter @clip-anvil/web lint`

Expected: PASS.

- [ ] **Step 4: Run whitespace check**

Run: `git diff --check`

Expected: no output and exit 0.

- [ ] **Step 5: Start local app through repo script if browser verification is needed**

Run: `./scripts/dev-start.sh`

Expected: script prints a Vite URL and backend health succeeds. Use the script-reported URL, not an assumed localhost port.

## Self-Review

- Spec coverage: drag start, preview, release-to-node creation, cancel behavior, click fallback, B-style animated saved edges, and no backend changes are covered.
- Placeholder scan: no TBD/TODO placeholders remain.
- Type consistency: geometry helper names match the test and overlay plan.
