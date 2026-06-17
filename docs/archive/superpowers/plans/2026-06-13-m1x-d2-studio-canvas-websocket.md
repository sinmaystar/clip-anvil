# M1.x-D2 Studio Canvas WebSocket Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `/ws/canvas` so Studio changes are reflected across browser tabs, with DB refetch as the recovery path after reconnect.

**Architecture:** REST remains the write authority. Handlers broadcast workspace-scoped events after successful DB writes; the frontend applies events idempotently and invalidates the canvas query after reconnect to recover from missed messages.

**Tech Stack:** Go 1.26, Hertz, `github.com/hertz-contrib/websocket`, JWT auth, React 19, TypeScript 6, TanStack Query, native browser WebSocket.

---

## File Structure

- Modify `apps/server/go.mod` and `apps/server/go.sum`: add Hertz websocket dependency.
- Create `apps/server/internal/api/ws_hub.go`: workspace connection registry and broadcast.
- Create `apps/server/internal/api/ws_handler.go`: `/ws/canvas` upgrade and auth.
- Create `apps/server/internal/api/ws_handler_test.go`: connection auth tests.
- Modify `apps/server/internal/api/node_handler.go`: broadcast node events.
- Modify `apps/server/internal/api/edge_handler.go`: broadcast edge events.
- Modify `apps/server/internal/api/group_handler.go`: broadcast group events.
- Modify `apps/server/cmd/server/main.go`: initialize hub and register route.
- Create `apps/web/src/lib/ws.ts`: reconnecting canvas socket client.
- Create `apps/web/src/components/ConnectionStatus.tsx`: status display.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`: connect socket and apply events.
- Modify `apps/web/src/lib/canvas.ts`: reusable apply helpers if needed.

### Task 1: Backend WebSocket Dependency and Hub

**Files:**
- Modify: `apps/server/go.mod`
- Modify: `apps/server/go.sum`
- Create: `apps/server/internal/api/ws_hub.go`

- [ ] **Step 1: Add dependency**

Run:

```bash
cd apps/server && go get github.com/hertz-contrib/websocket
```

Expected: `go.mod` and `go.sum` update.

- [ ] **Step 2: Create event and hub types**

Create `apps/server/internal/api/ws_hub.go`:

```go
package api

import (
	"encoding/json"
	"sync"

	"github.com/hertz-contrib/websocket"
	"github.com/jackc/pgx/v5/pgtype"
)

type CanvasEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type CanvasHub struct {
	mu    sync.RWMutex
	conns map[pgtype.UUID]map[*websocket.Conn]struct{}
}

func NewCanvasHub() *CanvasHub {
	return &CanvasHub{conns: map[pgtype.UUID]map[*websocket.Conn]struct{}{}}
}

func (h *CanvasHub) Register(workspaceID pgtype.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[workspaceID] == nil {
		h.conns[workspaceID] = map[*websocket.Conn]struct{}{}
	}
	h.conns[workspaceID][conn] = struct{}{}
}

func (h *CanvasHub) Unregister(workspaceID pgtype.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[workspaceID], conn)
	if len(h.conns[workspaceID]) == 0 {
		delete(h.conns, workspaceID)
	}
}

func (h *CanvasHub) Broadcast(workspaceID pgtype.UUID, event CanvasEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.conns[workspaceID]))
	for conn := range h.conns[workspaceID] {
		conns = append(conns, conn)
	}
	h.mu.RUnlock()
	for _, conn := range conns {
		_ = conn.WriteMessage(websocket.TextMessage, payload)
	}
}
```

- [ ] **Step 3: Run backend tests**

```bash
make server-test
```

Expected: PASS.

### Task 2: WebSocket Handler and Route

**Files:**
- Create: `apps/server/internal/api/ws_handler.go`
- Create: `apps/server/internal/api/ws_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Create handler**

Create `ws_handler.go`:

```go
package api

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/websocket"

	"github.com/sinmaystar/clip-anvil/internal/auth"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type CanvasWSHandler struct {
	queries   *db.Queries
	hub       *CanvasHub
	jwtSecret string
	upgrader  websocket.HertzUpgrader
}

func NewCanvasWSHandler(queries *db.Queries, hub *CanvasHub, jwtSecret string) *CanvasWSHandler {
	return &CanvasWSHandler{
		queries:   queries,
		hub:       hub,
		jwtSecret: jwtSecret,
		upgrader:  websocket.HertzUpgrader{},
	}
}

func (h *CanvasWSHandler) Canvas(ctx context.Context, c *app.RequestContext) {
	workspaceID, ok := uuidFromString(string(c.Query("workspaceId")))
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	token := string(c.Query("token"))
	accountID, err := auth.VerifyToken(token, h.jwtSecret)
	if err != nil {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	workspace, err := h.queries.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return
	}
	if workspace.OwnerID != accountID {
		writeError(c, consts.StatusForbidden, "forbidden")
		return
	}

	_ = h.upgrader.Upgrade(c, func(conn *websocket.Conn) {
		h.hub.Register(workspaceID, conn)
		defer h.hub.Unregister(workspaceID, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
}
```

- [ ] **Step 2: Register handler**

In `main.go`:

```go
canvasHub := api.NewCanvasHub()
canvasWSHandler := api.NewCanvasWSHandler(queries, canvasHub, cfg.JWT.Secret)
h.GET("/ws/canvas", canvasWSHandler.Canvas)
```

- [ ] **Step 3: Add connection auth tests**

Create tests with concrete HTTP assertions:

```go
func TestCanvasWSRejectsMissingToken(t *testing.T) {
	server, _, workspaceID := newCanvasWSTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/ws/canvas?workspaceId="+workspaceID, nil)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestCanvasWSRejectsForeignWorkspace(t *testing.T) {
	server, foreignToken, workspaceID := newCanvasWSTestServerWithForeignAccount(t)
	req := httptest.NewRequest(http.MethodGet, "/ws/canvas?workspaceId="+workspaceID+"&token="+url.QueryEscape(foreignToken), nil)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusForbidden, resp.Code)
}
```

Keep full upgrade behavior for manual verification because websocket integration tests need a running server and a real browser or websocket client.

- [ ] **Step 4: Run tests**

```bash
cd apps/server && go test ./internal/api -run CanvasWS -count=1
make server-test
```

Expected: PASS.

### Task 3: Broadcast REST Events

**Files:**
- Modify: `apps/server/internal/api/node_handler.go`
- Modify: `apps/server/internal/api/edge_handler.go`
- Modify: `apps/server/internal/api/group_handler.go`
- Modify: constructors and `apps/server/cmd/server/main.go`

- [ ] **Step 1: Add optional hub to handlers**

Update handlers to include:

```go
hub *CanvasHub
```

Constructors:

```go
func NewNodeHandler(pool *pgxpool.Pool, queries *db.Queries, hub *CanvasHub) *NodeHandler {
	return &NodeHandler{pool: pool, queries: queries, hub: hub}
}
```

Apply the same pattern to edge and group handlers.

- [ ] **Step 2: Broadcast node events after DB success**

In `NodeHandler.Create`, after successful insert:

```go
if h.hub != nil {
	h.hub.Broadcast(node.WorkspaceID, CanvasEvent{Type: "NodeCreated", Payload: map[string]any{"node": node}})
}
```

In `Update`, after final node update:

```go
if h.hub != nil {
	h.hub.Broadcast(node.WorkspaceID, CanvasEvent{Type: "NodeUpdated", Payload: map[string]any{"node": node}})
}
```

In `Delete`, save `workspaceID := node.WorkspaceID` before delete, then:

```go
if h.hub != nil {
	h.hub.Broadcast(workspaceID, CanvasEvent{Type: "NodeDeleted", Payload: map[string]any{"node_id": node.ID}})
}
```

- [ ] **Step 3: Broadcast edge events**

After create:

```go
h.hub.Broadcast(edge.WorkspaceID, CanvasEvent{Type: "EdgeCreated", Payload: map[string]any{"edge": edge}})
```

After delete:

```go
h.hub.Broadcast(edge.WorkspaceID, CanvasEvent{Type: "EdgeDeleted", Payload: map[string]any{"edge_id": edge.ID}})
```

- [ ] **Step 4: Broadcast group events**

Use event types `GroupCreated`, `GroupUpdated`, `GroupDeleted`. Include full `group` payload for create/update and `group_id` for delete.

- [ ] **Step 5: Run backend tests**

```bash
make server-test
```

Expected: PASS. Existing API tests should not require a WebSocket connection because hub broadcast is in-memory and non-blocking.

### Task 4: Frontend Reconnecting Canvas Socket

**Files:**
- Create: `apps/web/src/lib/ws.ts`
- Create: `apps/web/src/components/ConnectionStatus.tsx`

- [ ] **Step 1: Create ws client**

Create `apps/web/src/lib/ws.ts`:

```ts
export type CanvasConnectionStatus = "connecting" | "connected" | "reconnecting" | "offline";

export type CanvasEvent =
  | { type: "NodeCreated"; payload: { node: unknown } }
  | { type: "NodeUpdated"; payload: { node: unknown } }
  | { type: "NodeDeleted"; payload: { node_id: string } }
  | { type: "EdgeCreated"; payload: { edge: unknown } }
  | { type: "EdgeDeleted"; payload: { edge_id: string } }
  | { type: "GroupCreated"; payload: { group: unknown } }
  | { type: "GroupUpdated"; payload: { group: unknown } }
  | { type: "GroupDeleted"; payload: { group_id: string } };

export function connectCanvasSocket(input: {
  workspaceId: string;
  token: string;
  onEvent: (event: CanvasEvent) => void;
  onStatusChange: (status: CanvasConnectionStatus) => void;
  onReconnect: () => void;
}) {
  let closed = false;
  let attempt = 0;
  let socket: WebSocket | null = null;
  let timer: number | undefined;

  const connect = () => {
    input.onStatusChange(attempt === 0 ? "connecting" : "reconnecting");
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    socket = new WebSocket(`${protocol}//${window.location.host}/ws/canvas?workspaceId=${input.workspaceId}&token=${encodeURIComponent(input.token)}`);

    socket.onopen = () => {
      attempt = 0;
      input.onStatusChange("connected");
      input.onReconnect();
    };
    socket.onmessage = (message) => {
      input.onEvent(JSON.parse(message.data) as CanvasEvent);
    };
    socket.onclose = () => {
      if (closed) {
        input.onStatusChange("offline");
        return;
      }
      const delay = Math.min(30000, 1000 * 2 ** attempt);
      attempt += 1;
      timer = window.setTimeout(connect, delay);
    };
    socket.onerror = () => {
      socket?.close();
    };
  };

  connect();

  return () => {
    closed = true;
    window.clearTimeout(timer);
    socket?.close();
  };
}
```

- [ ] **Step 2: Create status component**

Create `ConnectionStatus.tsx`:

```tsx
import type { CanvasConnectionStatus } from "../lib/ws";

const label: Record<CanvasConnectionStatus, string> = {
  connecting: "连接中",
  connected: "已连接",
  reconnecting: "重连中",
  offline: "离线",
};

export function ConnectionStatus({ status }: { status: CanvasConnectionStatus }) {
  return <span className="connection-status" data-status={status}>{label[status]}</span>;
}
```

### Task 5: Apply WebSocket Events in Studio Page

**Files:**
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`

- [ ] **Step 1: Connect socket on page mount**

Import `connectCanvasSocket`, `ConnectionStatus`, and auth token. Add state:

```ts
const [connectionStatus, setConnectionStatus] = useState<CanvasConnectionStatus>("offline");
const token = useAuthStore((state) => state.token);
```

Add effect:

```ts
useEffect(() => {
  if (!id || !token) {
    return;
  }
  return connectCanvasSocket({
    workspaceId: id,
    token,
    onStatusChange: setConnectionStatus,
    onReconnect: () => {
      void queryClient.invalidateQueries({ queryKey: ["workspace", id, "canvas"] });
    },
    onEvent: (event) => applyCanvasEvent(event),
  });
}, [id, queryClient, token]);
```

- [ ] **Step 2: Implement event application**

Add `applyCanvasEvent` inside the component:

```ts
const applyCanvasEvent = useCallback((event: CanvasEvent) => {
  if (!id) {
    return;
  }
  switch (event.type) {
    case "NodeCreated":
    case "NodeUpdated":
    case "NodeDeleted":
    case "EdgeCreated":
    case "EdgeDeleted":
    case "GroupCreated":
    case "GroupUpdated":
    case "GroupDeleted":
      void queryClient.invalidateQueries({ queryKey: ["workspace", id, "canvas"] });
      break;
  }
}, [id, queryClient]);
```

This first implementation uses refetch as the idempotent correctness path. Direct shape mutation can be added after this works reliably.

- [ ] **Step 3: Render status**

Place in sidebar footer or header:

```tsx
<ConnectionStatus status={connectionStatus} />
```

- [ ] **Step 4: Build**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

### Task 6: Manual Multi-Tab Verification and Commit

**Files:**
- All files changed in Tasks 1-5.

- [ ] **Step 1: Run backend and frontend checks**

```bash
make server-test
pnpm --filter @clip-anvil/web... build
```

Expected: both PASS.

- [ ] **Step 2: Manual check**

Run app:

```bash
docker compose -f deploy/docker-compose.yml up -d
make server-dev
pnpm --filter @clip-anvil/web dev
```

Expected:

- Opening one workspace in two tabs shows `已连接`.
- Creating a node in tab 1 causes tab 2 to refetch and show it.
- Updating a node title in tab 1 appears in tab 2 after the event-triggered refetch.
- Closing and reopening the backend causes the frontend to move through reconnecting and recover after server restart.

- [ ] **Step 3: Commit**

```bash
git add apps/server/go.mod apps/server/go.sum apps/server/internal/api apps/server/cmd/server/main.go apps/web/src
git commit -m "feat: add studio canvas websocket"
```
