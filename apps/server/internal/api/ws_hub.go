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
	conns map[pgtype.UUID]map[*canvasConn]struct{}
	byRaw map[*websocket.Conn]*canvasConn
}

type canvasConn struct {
	raw *websocket.Conn
	mu  sync.Mutex
}

func NewCanvasHub() *CanvasHub {
	return &CanvasHub{
		conns: map[pgtype.UUID]map[*canvasConn]struct{}{},
		byRaw: map[*websocket.Conn]*canvasConn{},
	}
}

func (h *CanvasHub) Register(workspaceID pgtype.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[workspaceID] == nil {
		h.conns[workspaceID] = map[*canvasConn]struct{}{}
	}
	wrapped := h.byRaw[conn]
	if wrapped == nil {
		wrapped = &canvasConn{raw: conn}
		h.byRaw[conn] = wrapped
	}
	h.conns[workspaceID][wrapped] = struct{}{}
}

func (h *CanvasHub) Unregister(workspaceID pgtype.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	wrapped := h.byRaw[conn]
	if wrapped == nil {
		return
	}
	delete(h.conns[workspaceID], wrapped)
	if len(h.conns[workspaceID]) == 0 {
		delete(h.conns, workspaceID)
	}
	delete(h.byRaw, conn)
}

func (h *CanvasHub) Broadcast(workspaceID pgtype.UUID, event CanvasEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	conns := make([]*canvasConn, 0, len(h.conns[workspaceID]))
	for conn := range h.conns[workspaceID] {
		conns = append(conns, conn)
	}
	h.mu.RUnlock()
	for _, conn := range conns {
		conn.mu.Lock()
		_ = conn.raw.WriteMessage(websocket.TextMessage, payload)
		conn.mu.Unlock()
	}
}
