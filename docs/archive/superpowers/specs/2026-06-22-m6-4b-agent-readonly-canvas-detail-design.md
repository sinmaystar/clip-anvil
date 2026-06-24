# M6.4B Agent Read-Only Canvas And Node Detail Design

**Status**: Draft
**Date**: 2026-06-22
**Milestone**: M6 MultiAgent Agent Mode

## Goal

M6.4B upgrades the Agent workspace from a simplified node list to a Studio-derived read-only production canvas. Users can see uploaded images, videos, text assets, generated artifacts, versions, prompts, parameters, dependencies, task status, and errors with the same underlying data model used by Studio, while all editing actions remain disabled in Agent mode.

This phase also fixes Agent chat attachment rendering: image attachments sent through the composer must render as thumbnails in the message bubble instead of filename-only chips.

## Background

M6.4 introduced Agent chat attachments. Uploading an image from the Agent composer already stores the asset and creates an Agent-owned media node through the same backend production tables used by Studio. The current UX is still misleading because the Agent page renders canvas nodes with a temporary text list:

```tsx
<strong>{node.title}</strong>
<span>{node.node_type}</span>
```

That makes an image node appear as `design_pic.png image`, even though the backend has richer asset and node data available.

The chat message renderer has a related limitation. Attachment blocks persist stable references such as `asset_id` and `node_id`, but do not hydrate a fresh image URL for rendering. The frontend therefore only has enough information to render the attachment name.

## Product Decisions

### Agent canvas must be Studio-derived, not a separate simplified renderer

Agent mode should reuse Studio's canvas rendering path for nodes and previews:

- React Flow viewport and node rendering.
- `nodeToFlowNode` / view-model projection helpers.
- media node utilities for image, video, text, and generated media.
- production preview metadata where available.

Agent mode may wrap these pieces in a dedicated read-only component, but it must not fork a separate visual system that only approximates Studio.

### Read-only does not mean low-information

Agent users should be able to inspect the full production information for any visible node. Agent mode disables mutations; it does not hide the data users need to understand what the Agent produced.

Agent node detail must include the same categories Studio exposes:

- node identity: title, type, source, status;
- prompt and negative prompt;
- model, provider, seed, size, duration, aspect ratio, and generation parameters;
- asset preview and storage metadata;
- artifact versions and winner/current version state;
- production preview;
- dependency, sequence, and reference relationships;
- generation job/task status;
- review score and rubric summary when available;
- stale state and downstream impact;
- error and retry diagnostics.

The exact first implementation may reuse existing Studio inspector sections where possible, but the Agent surface must render them as read-only fields.

### Agent mode keeps the Studio/Agent boundary explicit

Agent mode can browse, select, inspect, copy, and reference nodes. Agent mode cannot edit them directly.

Blocked in Agent mode:

- dragging nodes to persist new positions;
- editing prompt or parameters;
- deleting nodes;
- creating nodes through Studio tools;
- creating or changing edges manually;
- submitting Studio mutation APIs directly.

Allowed in Agent mode:

- pan and zoom the canvas;
- select nodes for reading;
- open read-only detail;
- copy node metadata;
- reference a node in the Agent composer;
- ask ClipAnvil to modify or regenerate through conversation.

## Scope

In scope:

- Hydrate Agent attachment blocks with fresh image/video URLs in API and WebSocket responses.
- Render image attachments as thumbnails in user message bubbles.
- Keep persisted message content stable by storing IDs, not expiring signed URLs.
- Replace the Agent page's temporary node list with a Studio-derived read-only canvas viewport.
- Reuse Studio node rendering for image, video, text, source material, and generated media nodes.
- Add read-only node selection and node detail viewing in Agent mode.
- Ensure Agent mode can view complete node production details.
- Preserve Agent write restrictions on Studio mutation APIs.
- Add unit tests and browser E2E coverage for thumbnails, canvas rendering, and read-only detail.

Out of scope:

- Editing nodes from Agent mode.
- Studio / Agent import-export.
- Full Studio sidebars, toolbars, and creation tools inside Agent mode.
- New production generation capabilities.
- New custom HITL card types beyond what the existing UI message protocol already supports.
- Historical message migration for old attachment blocks without asset IDs.

## Architecture

### Attachment preview hydration

Persisted Agent message content remains stable:

```json
{
  "type": "attachment",
  "attachments": [
    {
      "asset_id": "uuid",
      "node_id": "uuid",
      "kind": "image",
      "name": "design_pic.png",
      "mime": "image/png",
      "size_bytes": 123456
    }
  ]
}
```

API and WebSocket responses may hydrate transient render fields:

```json
{
  "asset_id": "uuid",
  "node_id": "uuid",
  "kind": "image",
  "name": "design_pic.png",
  "mime": "image/png",
  "size_bytes": 123456,
  "url": "https://signed-preview-url",
  "thumbnail_url": "https://signed-thumbnail-url"
}
```

Rules:

- `url` and `thumbnail_url` are response-only render affordances.
- They are not the canonical message source of truth.
- If a signed URL expires, refreshing messages must produce a fresh URL.
- If URL hydration fails, the frontend falls back to a filename attachment chip and the backend logs the asset ID and reason.

### Read-only canvas viewport

Create a shared canvas viewport boundary instead of importing the full Studio page into Agent:

```text
Studio page
  -> CanvasViewport(mode="edit")
  -> NodeInspector(mode="edit")

Agent page
  -> CanvasViewport(mode="readonly")
  -> NodeInspector(mode="readonly")
  -> floating ClipAnvil chat
```

`CanvasViewport(mode="readonly")` responsibilities:

- render the same node nodes as Studio;
- render existing groups and edges when available;
- allow pan and zoom;
- allow node selection;
- prevent mutations from pointer gestures and keyboard shortcuts;
- never show Studio creation toolbar or context menus;
- notify the Agent page when selection changes.

### Read-only node detail

Agent mode should expose node detail through a read-only inspector surface. Because the ClipAnvil chat panel floats on the right, the detail surface should not compete for the same space.

Preferred layout:

- desktop: left-side or bottom drawer above the canvas;
- small screens: bottom sheet;
- no right-side property panel by default, because the right side belongs to chat.

The read-only inspector should reuse Studio detail sections where practical, but it must have an explicit `mode="readonly"` or equivalent permission gate. Hiding form controls is not enough; mutation handlers must be absent or blocked.

### Node detail data

The Agent page should first use the existing canvas read API when it already returns enough data. If the current canvas response lacks full Studio inspector fields, add a read-only detail endpoint:

```http
GET /api/workspaces/:workspaceID/nodes/:nodeID/detail
```

Requirements:

- allowed in both Studio and Agent mode;
- requires workspace ownership;
- returns only data from the requested workspace;
- includes asset, production preview, versions, prompt fields, params, jobs, edges, review data, and diagnostics needed by the inspector;
- does not expose write-only or provider secret fields.

This endpoint is read-only and should not weaken the existing Agent-mode write guard.

## Frontend Behavior

### Chat attachment rendering

Image attachments render as compact thumbnails:

- thumbnail preview;
- filename;
- file type/size metadata where space allows;
- fallback chip if preview URL is unavailable.

Video attachments may render as a media tile with a play indicator if a preview URL exists. Text attachments remain compact chips unless a text preview block is added later.

### Canvas rendering

Agent canvas empty state remains visible only when there are no nodes. Once the user uploads an image or Agent creates a node, the canvas renders the same visual node style used by Studio.

Expected image upload result:

- chat bubble shows image thumbnail;
- canvas shows an image/source-material node with the actual image preview;
- selecting the node opens read-only detail;
- the detail contains asset ID, node ID, title, type, MIME, and storage/preview metadata;
- if prompt or generation fields are empty for a source material node, the detail explicitly shows them as empty or not applicable rather than hiding the whole section.

### Selection and composer reference

Selecting a node should expose a lightweight action to reference it in the Agent composer. The inserted reference should use a stable node reference, not just display text.

Initial acceptable behavior:

```text
@design_pic.png
```

Future-ready internal representation:

```json
{
  "type": "node_reference",
  "node_id": "uuid",
  "label": "design_pic.png"
}
```

The first implementation can keep this as a UI affordance without requiring the Agent model to fully resolve rich references, as long as the stored message can carry the node ID when sent.

## Backend Behavior

### Agent attachment endpoint

The existing Agent attachment endpoint remains responsible for:

- storing uploaded files as media assets;
- creating Agent-owned media/source material nodes;
- returning the created asset and node;
- broadcasting canvas updates.

This phase adds response hydration for attachment render URLs and strengthens tests around image attachments.

### Message response hydration

Every Agent message response path must hydrate attachment previews consistently:

- list messages;
- post message response;
- WebSocket message-created events;
- WebSocket streaming completion events if they include a final message payload.

The hydrator should:

1. parse UI message blocks;
2. find attachment blocks;
3. load referenced assets;
4. sign preview URLs using the storage service;
5. inject response-only URL fields;
6. log non-fatal hydration failures with message ID, asset ID, workspace ID, and error reason.

### Agent mode authorization

Read APIs remain allowed in Agent mode:

- canvas read;
- node detail read;
- asset preview URL hydration;
- message list/read.

Write APIs remain blocked unless they are explicit Agent-approved operations:

- Agent attachment upload is allowed;
- Agent message send is allowed;
- Studio node mutation APIs remain blocked;
- Studio edge mutation APIs remain blocked;
- Studio camera write can remain disabled or treated separately according to the existing mode policy.

## Testing Strategy

### Unit and integration tests

Backend:

- attachment hydration injects image URL fields for valid assets;
- hydration does not persist signed URLs back into `agent_message.content`;
- missing asset or signing failure falls back without failing message list;
- Agent mode can read node detail;
- Agent mode cannot mutate nodes through Studio APIs.

Frontend:

- `AgentAttachmentBlock` renders image thumbnail when `url` or `thumbnail_url` exists;
- attachment block falls back to filename chip when no URL exists;
- Agent read-only canvas builds nodes from canvas nodes using shared conversion helpers;
- read-only inspector renders prompt/params/versions/status fields as non-editable;
- unsupported or empty detail fields render stable empty states.

### Browser E2E

1. Start the app through `./scripts/dev-start.sh`.
2. Create or open an Agent workspace.
3. Upload a PNG image through the ClipAnvil composer.
4. Confirm the pending/sent user message renders a thumbnail, not only the filename.
5. Send a message with the image attached.
6. Confirm the Agent canvas renders an image node with the actual preview.
7. Select the image node.
8. Confirm read-only node detail opens.
9. Confirm node detail shows node ID/title/type/source, asset metadata, and preview metadata.
10. Confirm no prompt/parameter edit input is enabled in Agent mode.
11. Try a Studio mutation path from Agent mode and confirm it is rejected.
12. Refresh the page and confirm the message thumbnail is still visible through freshly hydrated URLs.

## Required Verification Commands

Run these after implementation:

```bash
make server-build
make server-test
make server-lint
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

If sqlc queries or migrations change, also run:

```bash
make sqlc-generate
make server-test
```

Browser verification must use the Vite URL printed by:

```bash
./scripts/dev-start.sh
```

Do not assume a fixed frontend port in multi-worktree mode.

## Acceptance Criteria

This phase is complete only when all of the following are true:

- Image attachments in Agent chat render as thumbnails when preview URLs are available.
- Persisted message content still stores stable IDs rather than expiring signed URLs.
- Refreshing the Agent page rehydrates message attachment previews.
- Agent canvas no longer renders nodes as a text-only list.
- Agent canvas uses Studio-derived node nodes for media/source/generated nodes.
- Uploaded image nodes render with image preview on the Agent canvas.
- Selecting a node in Agent mode exposes full read-only production detail.
- Prompt, params, versions, assets, job status, relationships, and errors are visible when present.
- No Studio edit controls are enabled from Agent mode.
- Existing Agent chat panel remains floating on the right above the canvas.
- Automated tests and browser E2E pass.

## Risks And Mitigations

### Risk: accidentally enabling Studio edits in Agent mode

Mitigation: read-only must be enforced at both UI and API layers. UI should not bind mutation handlers, and backend mode guards must continue rejecting Studio write APIs.

### Risk: signed URLs become stale in persisted messages

Mitigation: persist IDs only. Hydrate URLs on response.

### Risk: duplicating Studio inspector logic

Mitigation: extract shared read/display sections where possible. If Studio components are too mutation-heavy, split them into presentational read sections plus edit wrappers instead of copying code.

### Risk: React Flow read-only mode still allows local visual changes

Mitigation: distinguish local viewport interactions from persisted mutations. Pan, zoom, and selection are allowed. Node creation, deletion, movement persistence, and keyboard editing are not.

## Open Follow-Ups

- Whether Agent node references should be stored as a dedicated `node_reference` message block in this phase or the next UI protocol phase.
- Whether video attachments should render a playable inline preview immediately or start with a poster tile.
- Whether the read-only inspector should support a "compare versions" view in this phase or defer to the full M6 production review work.
