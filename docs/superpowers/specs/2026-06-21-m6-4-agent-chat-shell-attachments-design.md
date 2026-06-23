# M6.4 Agent Chat Shell And Attachments Design

## Goal

M6.4 turns the Agent workspace chat from a visible Producer prototype into a user-facing ClipAnvil assistant surface, and adds the first Agent-owned attachment ingestion path. Users can upload image, video, and text files from the chat composer; the backend stores them as workspace assets, creates Agent-owned source material nodes, persists attachment metadata on the user message, and keeps the Agent canvas read-only for ordinary Studio mutation APIs.

## Scope

In scope:

- Rename user-facing chat identity from `Producer` to `ClipAnvil`.
- Redesign the right floating chat panel with a flatter, modern visual style.
- Add connection status dot states instead of visible connection text.
- Support desktop panel width resizing with persisted local width.
- Replace the text send button with an icon-only send button inside the composer.
- Add attachment selection for `image/*`, `video/*`, and `.txt` / `text/plain`.
- Upload attachments through an Agent-specific endpoint that is legal in Agent mode.
- Create `source='agent'` source material nodes for uploaded attachments.
- Persist user message attachment metadata for future model/tool context.
- Show uploaded source nodes in the read-only Agent canvas after send/upload.
- Verify with unit tests, build/lint, and browser e2e.

Out of scope:

- Full multimodal model reasoning over image/video bytes.
- Agent tool calling against the uploaded materials.
- Audio input and microphone recording.
- Drag and drop attachment upload.
- Full Studio-grade tldraw rendering inside Agent mode.

## Product Decisions

The UI must not expose internal role names such as Producer. Internal task/thread roles can remain `producer` because they are part of MultiAgent orchestration, but user-visible labels use `ClipAnvil`.

The chat panel remains anchored on the right side above the canvas. It should feel like a product workspace assistant, not an admin card: lower visual weight, softer surface, icon controls, and no heavy borders around every element.

The status indicator is a dot:

- Green for connected.
- Amber for connecting/reconnecting.
- Red for offline.

## Backend Design

Add `POST /api/agent/workspaces/:workspaceID/attachments`.

Request:

- `multipart/form-data`
- `file`: required.

Authorization and mode checks:

- JWT required.
- Workspace must belong to the account.
- Workspace must be `agent` mode.

Accepted files:

- Image: existing upload MIME support.
- Video: existing upload MIME support.
- Text: `text/plain` and common browser `text/plain; charset=utf-8` cases.

Behavior:

1. Validate file size using existing `maxUploadBytes`.
2. Detect MIME from the file head.
3. For image/video, upload to MinIO using the existing storage service and create `media_asset`.
4. For text, read content into `media_asset.text_content` through `CreateTextMediaAsset`.
5. Create a source material node using existing `CreateAgentMediaNode`.
6. Return asset and node response.
7. Broadcast `NodeCreated` over the canvas hub so any canvas subscribers can pick it up later.

The normal `/api/nodes` Studio mutation path remains blocked in Agent mode.

## Message Persistence

Extend `POST /api/agent/workspaces/:workspaceID/messages` request body with optional `attachments`.

Each attachment contains:

- `asset_id`
- `node_id`
- `kind`: `image`, `video`, or `text`
- `name`
- `mime`
- `size_bytes`

The backend validates that every referenced node and asset belongs to the same Agent workspace. Valid attachments are persisted in `agent_message.content.attachments`.

The model prompt should mention attachment names and kinds as context, but does not need to ingest file bytes in this phase.

## Frontend Design

Add focused Agent UI helpers:

- `agentAttachments.ts`: accepted file detection, upload state, preview labels, message attachment formatting.
- `agentLayout.ts`: chat panel width clamp and persistence.

Agent workspace UI:

- Header label: `ClipAnvil`.
- Restore button label: `ClipAnvil`.
- Textarea aria-label: `发送给 ClipAnvil`.
- Empty/running copy avoids Producer.
- Composer uses icon-only controls:
  - `+` attachment button.
  - paper-plane/send icon button.
- Attachment chips render before send and inside sent user messages.
- Canvas query is refreshed after attachment upload so new source nodes appear.

## Acceptance Criteria

Automated:

- Server tests pass.
- Web helper tests pass.
- Web build and lint pass.
- `git diff --check` passes.

Browser e2e:

1. Start the app with `./scripts/dev-start.sh`.
2. Register a new user.
3. Create an Agent workspace.
4. Confirm the chat title is `ClipAnvil` and no visible `Producer` label appears.
5. Upload a text file from the composer.
6. Confirm an attachment chip appears.
7. Send a message with the attachment.
8. Confirm the read-only canvas shows a text source material node.
9. Confirm the user message persists attachment metadata after refresh.
10. Confirm the LLM streaming reply still works.
11. Confirm database `agent_message.content.attachments` contains the uploaded node/asset IDs.
