package api

import (
	"encoding/json"
	"sync"

	"github.com/hertz-contrib/websocket"
	"github.com/jackc/pgx/v5/pgtype"
)

type AgentSocketEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type AgentHub struct {
	mu    sync.RWMutex
	conns map[pgtype.UUID]map[*agentConn]struct{}
	byRaw map[*websocket.Conn]*agentConn
}

type agentConn struct {
	raw *websocket.Conn
	mu  sync.Mutex
}

func NewAgentHub() *AgentHub {
	return &AgentHub{
		conns: map[pgtype.UUID]map[*agentConn]struct{}{},
		byRaw: map[*websocket.Conn]*agentConn{},
	}
}

func (h *AgentHub) Register(workspaceID pgtype.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[workspaceID] == nil {
		h.conns[workspaceID] = map[*agentConn]struct{}{}
	}
	wrapped := h.byRaw[conn]
	if wrapped == nil {
		wrapped = &agentConn{raw: conn}
		h.byRaw[conn] = wrapped
	}
	h.conns[workspaceID][wrapped] = struct{}{}
}

func (h *AgentHub) Unregister(workspaceID pgtype.UUID, conn *websocket.Conn) {
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

func (h *AgentHub) Broadcast(workspaceID pgtype.UUID, event AgentSocketEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	conns := make([]*agentConn, 0, len(h.conns[workspaceID]))
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
