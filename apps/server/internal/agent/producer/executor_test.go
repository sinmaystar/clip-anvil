package producer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestExecutorPersistsAssistantMessageOnSuccess(t *testing.T) {
	runtime := &fakeRuntime{}
	broadcaster := &fakeBroadcaster{}
	executor := NewExecutor(ExecutorConfig{
		Runtime:     runtime,
		Graph:       fakeGraph{output: ProducerTurnOutput{AssistantText: "assistant reply"}},
		Broadcaster: broadcaster,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	if runtime.runningTask != uuidWithByte(3) {
		t.Fatal("task was not marked running")
	}
	if runtime.assistantText != "assistant reply" {
		t.Fatalf("assistant text = %q", runtime.assistantText)
	}
	if runtime.succeededTask != uuidWithByte(3) {
		t.Fatal("task was not marked succeeded")
	}
	if broadcaster.messageCount != 1 || broadcaster.taskCount == 0 {
		t.Fatalf("broadcast counts = messages %d tasks %d", broadcaster.messageCount, broadcaster.taskCount)
	}
}

func TestExecutorBroadcastsStreamDeltasBeforeFinalMessage(t *testing.T) {
	runtime := &fakeRuntime{}
	broadcaster := &fakeBroadcaster{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph: fakeGraph{
			output: ProducerTurnOutput{AssistantText: "streamed reply"},
			deltas: []string{"streamed ", "reply"},
		},
		Broadcaster: broadcaster,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(broadcaster.deltas, ""); got != "streamed reply" {
		t.Fatalf("stream deltas = %q", got)
	}
	if broadcaster.messageCount != 1 {
		t.Fatalf("message broadcasts = %d, want 1", broadcaster.messageCount)
	}
}

func TestExecutorPersistsSelectedModelMetadata(t *testing.T) {
	runtime := &fakeRuntime{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph: fakeGraph{output: ProducerTurnOutput{
			AssistantText: "assistant reply",
			Metadata: map[string]any{
				"provider":           "volcengine",
				"model_id":           "workspace-model",
				"model_display_name": "Workspace Model",
			},
		}},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(runtime.assistantRawMessage, &raw); err != nil {
		t.Fatalf("unmarshal raw message: %v", err)
	}
	if raw["model_id"] != "workspace-model" || raw["model_display_name"] != "Workspace Model" {
		t.Fatalf("raw metadata = %#v", raw)
	}
}

func TestExecutorUsesAgentModelUnavailableErrorCode(t *testing.T) {
	runtime := &fakeRuntime{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph:   fakeGraph{err: NewAgentError("agent_model_unavailable", "model disabled")},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if runtime.failedCode != "agent_model_unavailable" {
		t.Fatalf("failed code = %q, want agent_model_unavailable", runtime.failedCode)
	}
}

func TestExecutorPersistsErrorMessageOnFailure(t *testing.T) {
	runtime := &fakeRuntime{}
	broadcaster := &fakeBroadcaster{}
	executor := NewExecutor(ExecutorConfig{
		Runtime:     runtime,
		Graph:       fakeGraph{err: errors.New("model unavailable")},
		Broadcaster: broadcaster,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if runtime.failedTask != uuidWithByte(3) {
		t.Fatal("task was not marked failed")
	}
	if runtime.assistantMessageType != "error" {
		t.Fatalf("assistant message type = %q, want error", runtime.assistantMessageType)
	}
	if strings.Contains(runtime.assistantText, "Producer") {
		t.Fatalf("user visible error leaked internal role: %q", runtime.assistantText)
	}
	if broadcaster.messageCount != 1 || broadcaster.taskCount == 0 {
		t.Fatalf("broadcast counts = messages %d tasks %d", broadcaster.messageCount, broadcaster.taskCount)
	}
}

func TestExecutorLogsTaskFailureContext(t *testing.T) {
	runtime := &fakeRuntime{}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph:   fakeGraph{err: NewAgentError("agent_model_unavailable", "model disabled")},
		Logger:  logger,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	logText := logs.String()
	if !strings.Contains(logText, "producer task failed") ||
		!strings.Contains(logText, `"error_code":"agent_model_unavailable"`) ||
		!strings.Contains(logText, `"workspace_id":"01000000-0000-0000-0000-000000000000"`) ||
		!strings.Contains(logText, `"task_id":"03000000-0000-0000-0000-000000000000"`) {
		t.Fatalf("logs = %s", logText)
	}
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}

type fakeGraph struct {
	output ProducerTurnOutput
	deltas []string
	err    error
}

func (f fakeGraph) Run(ctx context.Context, input ProducerTurnInput) (ProducerTurnOutput, error) {
	for _, delta := range f.deltas {
		if input.EmitDelta != nil {
			if err := input.EmitDelta(ctx, ProducerStreamDelta{Delta: delta}); err != nil {
				return ProducerTurnOutput{}, err
			}
		}
	}
	return f.output, f.err
}

type fakeBroadcaster struct {
	messageCount       int
	messageUpdateCount int
	taskCount          int
	eventCount         int
	deltas             []string
}

func (f *fakeBroadcaster) BroadcastAgentMessage(pgtype.UUID, db.AgentMessage, db.AgentEvent) {
	f.messageCount++
}

func (f *fakeBroadcaster) BroadcastAgentMessageUpdated(pgtype.UUID, db.AgentMessage, db.AgentEvent) {
	f.messageUpdateCount++
}

func (f *fakeBroadcaster) BroadcastAgentTask(pgtype.UUID, db.AgentTask) {
	f.taskCount++
}

func (f *fakeBroadcaster) BroadcastAgentEvent(pgtype.UUID, db.AgentEvent) {
	f.eventCount++
}

func (f *fakeBroadcaster) BroadcastAgentMessageDelta(_ pgtype.UUID, delta ProducerStreamDelta) {
	f.deltas = append(f.deltas, delta.Delta)
}

type fakeRuntime struct {
	runningTask          pgtype.UUID
	succeededTask        pgtype.UUID
	failedTask           pgtype.UUID
	failedCode           string
	assistantText        string
	assistantMessageType string
	assistantRawMessage  []byte
}

func (f *fakeRuntime) MarkTaskRunning(_ context.Context, taskID pgtype.UUID) (db.AgentTask, error) {
	f.runningTask = taskID
	return db.AgentTask{ID: taskID, WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), Role: "producer", TaskType: "producer_turn", Status: "running"}, nil
}

func (f *fakeRuntime) MarkTaskSucceeded(_ context.Context, taskID pgtype.UUID, _ []byte) (db.AgentTask, error) {
	f.succeededTask = taskID
	return db.AgentTask{ID: taskID, WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), Role: "producer", TaskType: "producer_turn", Status: "succeeded"}, nil
}

func (f *fakeRuntime) MarkTaskFailed(_ context.Context, taskID pgtype.UUID, code, _ string) (db.AgentTask, error) {
	f.failedTask = taskID
	f.failedCode = code
	return db.AgentTask{ID: taskID, WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), Role: "producer", TaskType: "producer_turn", Status: "failed"}, nil
}

func (f *fakeRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.assistantMessageType = params.MessageType
	if texts := uimessage.ExtractMarkdownTexts(params.Content); len(texts) > 0 {
		f.assistantText = strings.Join(texts, "\n\n")
	}
	f.assistantRawMessage = params.RawMessage
	return db.AgentMessage{
		ID:          uuidWithByte(9),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		MessageType: params.MessageType,
		Content:     params.Content,
		RawMessage:  params.RawMessage,
		TaskID:      params.TaskID,
	}, nil
}

func (f *fakeRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	return db.AgentEvent{
		ID:          uuidWithByte(8),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		TaskID:      params.TaskID,
		EventType:   params.EventType,
		SourceRole:  params.SourceRole,
	}, nil
}
