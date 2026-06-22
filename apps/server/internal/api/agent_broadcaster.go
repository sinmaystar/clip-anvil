package api

import (
	"github.com/jackc/pgx/v5/pgtype"

	agentproducer "github.com/sinmaystar/clip-anvil/internal/agent/producer"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type AgentBroadcaster struct {
	hub *AgentHub
}

func NewAgentBroadcaster(hub *AgentHub) *AgentBroadcaster {
	return &AgentBroadcaster{hub: hub}
}

func (b *AgentBroadcaster) BroadcastAgentMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	b.hub.Broadcast(workspaceID, AgentSocketEvent{
		Type: "agent.message.created",
		Payload: map[string]any{
			"workspace_id": uuidToString(workspaceID),
			"thread_id":    uuidToString(message.ThreadID),
			"message":      toAgentMessageResponse(message),
			"event":        toAgentEventResponse(event),
		},
	})
}

func (b *AgentBroadcaster) BroadcastAgentTask(workspaceID pgtype.UUID, task db.AgentTask) {
	b.hub.Broadcast(workspaceID, AgentSocketEvent{
		Type: "agent.task.updated",
		Payload: map[string]any{
			"workspace_id": uuidToString(workspaceID),
			"thread_id":    nullableUUIDString(task.ThreadID),
			"task":         toAgentTaskResponse(task),
		},
	})
}

func (b *AgentBroadcaster) BroadcastAgentEvent(workspaceID pgtype.UUID, event db.AgentEvent) {
	b.hub.Broadcast(workspaceID, AgentSocketEvent{
		Type: "agent.event.created",
		Payload: map[string]any{
			"workspace_id": uuidToString(workspaceID),
			"thread_id":    nullableUUIDString(event.ThreadID),
			"task_id":      nullableUUIDString(event.TaskID),
			"event":        toAgentEventResponse(event),
		},
	})
}

func (b *AgentBroadcaster) BroadcastAgentMessageDelta(workspaceID pgtype.UUID, delta agentproducer.ProducerStreamDelta) {
	b.hub.Broadcast(workspaceID, AgentSocketEvent{
		Type: "agent.message.delta",
		Payload: map[string]any{
			"workspace_id": delta.WorkspaceID,
			"thread_id":    delta.ThreadID,
			"task_id":      delta.TaskID,
			"message_id":   delta.MessageID,
			"block_id":     delta.BlockID,
			"block_type":   delta.BlockType,
			"delta":        delta.Delta,
			"sequence":     delta.Sequence,
		},
	})
}
