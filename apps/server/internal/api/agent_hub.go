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
	conns map[pgtype.UUID]map[*websocket.Conn]struct{}
}

func NewAgentHub() *AgentHub {
	return &AgentHub{conns: map[pgtype.UUID]map[*websocket.Conn]struct{}{}}
}

func (h *AgentHub) Register(workspaceID pgtype.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[workspaceID] == nil {
		h.conns[workspaceID] = map[*websocket.Conn]struct{}{}
	}
	h.conns[workspaceID][conn] = struct{}{}
}

func (h *AgentHub) Unregister(workspaceID pgtype.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[workspaceID], conn)
	if len(h.conns[workspaceID]) == 0 {
		delete(h.conns, workspaceID)
	}
}

func (h *AgentHub) Broadcast(workspaceID pgtype.UUID, event AgentSocketEvent) {
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
