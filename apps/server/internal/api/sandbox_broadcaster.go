package api

import "github.com/jackc/pgx/v5/pgtype"

type SandboxBroadcaster struct {
	hub *CanvasHub
}

func NewSandboxBroadcaster(hub *CanvasHub) *SandboxBroadcaster {
	return &SandboxBroadcaster{hub: hub}
}

func (b *SandboxBroadcaster) Broadcast(workspaceID pgtype.UUID, event string, payload map[string]any) {
	if b == nil || b.hub == nil {
		return
	}
	b.hub.Broadcast(workspaceID, CanvasEvent{Type: event, Payload: payload})
}
