package api

import "testing"

func TestAgentHubRegisterAndUnregister(t *testing.T) {
	workspaceID := testUUID(0x21)
	hub := NewAgentHub()

	hub.Register(workspaceID, nil)

	if got := len(hub.conns[workspaceID]); got != 1 {
		t.Fatalf("connection count = %d, want 1", got)
	}

	hub.Unregister(workspaceID, nil)

	if _, ok := hub.conns[workspaceID]; ok {
		t.Fatal("workspace connections should be removed after unregister")
	}
}

func TestAgentHubBroadcastWithoutConnections(t *testing.T) {
	hub := NewAgentHub()

	hub.Broadcast(testUUID(0x22), AgentSocketEvent{
		Type:    "agent.event.created",
		Payload: map[string]any{"event_id": "event"},
	})
}
