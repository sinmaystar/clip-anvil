# Canvas Connection Drag Flow Design

## Goal

Make canvas dependency linking feel direct and expressive: drag from a node output port, see a long curved preview line, release over any other node to create the edge, and render saved edges with the same flowing visual language.

## Current Behavior

The current canvas uses a custom DOM output port on each media node. Clicking the output port enters a pending state, shows "选择目标节点", and the next target click creates a dependency edge. Saved edges are tldraw arrow shapes with `kind: "arc"` but `bend: 0`, so they read as straight lines and have no motion.

## Interaction Design

- Pressing the right output port starts a drag connection.
- While dragging, the canvas shows a live preview from the source node's right midpoint to the pointer.
- Releasing over any other media node creates a dependency edge from the source node to that target node.
- Releasing on empty canvas, the same node, or a non-node target cancels without an error toast.
- Clicking the output port without dragging keeps the existing click-to-connect fallback for accessibility and trackpad users who prefer tap flows.
- During drag, valid target nodes get a subtle input-port highlight so the user can see where release will connect.

## Visual Design

Use the selected option B direction:

- Long cubic Bezier curve from source to target, with stronger horizontal pull so the line has flow.
- Blue-to-purple/orange accent stroke, depending on what tldraw's built-in arrow styling can support safely.
- Animated light band or dash moving from start to end.
- Arrowhead remains at the target side.
- Preview line matches the saved edge style closely enough that drag and final result feel like one interaction.

## Architecture

Keep the database and API unchanged. `media_edge` remains the single persisted relationship record. The frontend continues to project edges into tldraw arrows for editing/deletion, and adds a lightweight SVG overlay for custom animated visuals and drag preview.

The overlay reads node and edge geometry from React query canvas data and editor camera state. It does not persist independent state. Edge creation still goes through `POST /api/edges`, preserving backend duplicate and cycle validation.

## Testing

- Extract the Bezier geometry into a small pure TypeScript helper and test it with Node's built-in test runner.
- Verify drag release target logic through focused helper tests where possible.
- Run `pnpm --filter @clip-anvil/web... build`, `pnpm --filter @clip-anvil/web lint`, and `git diff --check`.
- If the local dev stack starts cleanly, visually smoke test in the browser using `./scripts/dev-start.sh` and the script-reported Vite URL.
