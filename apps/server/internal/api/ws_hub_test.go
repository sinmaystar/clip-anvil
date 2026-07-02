package api

import "testing"

func TestCanvasHubRegisterAndUnregister(t *testing.T) {
	workspaceID := testUUID(0x09)
	hub := NewCanvasHub()

	hub.Register(workspaceID, nil)

	if got := len(hub.conns[workspaceID]); got != 1 {
		t.Fatalf("connection count = %d, want 1", got)
	}

	hub.Unregister(workspaceID, nil)

	if _, ok := hub.conns[workspaceID]; ok {
		t.Fatal("workspace connections should be removed after unregister")
	}
}

func TestCanvasHubWrapsRawConnectionForSerializedWrites(t *testing.T) {
	workspaceID := testUUID(0x0b)
	hub := NewCanvasHub()

	hub.Register(workspaceID, nil)

	wrapped := hub.byRaw[nil]
	if wrapped == nil {
		t.Fatal("raw connection was not wrapped")
	}
	if _, ok := hub.conns[workspaceID][wrapped]; !ok {
		t.Fatal("workspace should store the wrapped connection")
	}

	hub.Unregister(workspaceID, nil)

	if _, ok := hub.byRaw[nil]; ok {
		t.Fatal("wrapped connection should be removed after unregister")
	}
}

func TestCanvasHubBroadcastWithoutConnections(t *testing.T) {
	hub := NewCanvasHub()

	hub.Broadcast(testUUID(0x0a), CanvasEvent{Type: "NodeCreated", Payload: map[string]string{"id": "node"}})
}
