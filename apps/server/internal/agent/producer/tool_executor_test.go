package producer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestRegistryToolExecutorPersistsToolCallAndResult(t *testing.T) {
	runtime := &fakeToolRuntime{}
	broadcaster := &fakeBroadcaster{}
	tool := &recordingTool{name: "create_agent_text_node"}
	registry, err := agenttools.NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}
	executor := NewRegistryToolExecutor(RegistryToolExecutorConfig{
		Registry:    registry,
		Runtime:     runtime,
		Broadcaster: broadcaster,
	})

	result, err := executor.ExecuteProducerTool(context.Background(), ProducerContext{
		Input: ProducerTurnInput{
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(2),
			TaskID:      uuidWithByte(3),
		},
	}, ToolCall{Name: "create_agent_text_node", Arguments: map[string]any{"title": "brief"}})
	if err != nil {
		t.Fatal(err)
	}

	if result.Interrupted {
		t.Fatal("tool should not interrupt")
	}
	if tool.receivedTaskID != uuidWithByte(3) {
		t.Fatal("tool did not receive producer task id")
	}
	if runtime.createdTaskType != "tool_call" {
		t.Fatalf("task type = %q, want tool_call", runtime.createdTaskType)
	}
	if len(runtime.messageTypes) != 1 || runtime.messageTypes[0] != "tool_call" {
		t.Fatalf("message types = %#v", runtime.messageTypes)
	}
	if runtime.messages[0].Content == nil || runtime.updatedMessage.Content == nil {
		t.Fatalf("messages = %#v", runtime.messages)
	}
	assertToolStatusBlock(t, runtime.messages[0].Content, "create_agent_text_node", "running", true, false)
	assertToolStatusBlock(t, runtime.updatedMessage.Content, "create_agent_text_node", "succeeded", true, true)
	if runtime.eventTypes[0] != "tool_call_started" || runtime.eventTypes[1] != "tool_call_completed" {
		t.Fatalf("event types = %#v", runtime.eventTypes)
	}
	if runtime.succeededTask != uuidWithByte(4) {
		t.Fatal("tool task was not marked succeeded")
	}
	if broadcaster.messageCount != 1 || broadcaster.messageUpdateCount != 1 || broadcaster.taskCount < 2 || broadcaster.eventCount != 2 {
		t.Fatalf("broadcasts message=%d updates=%d task=%d event=%d", broadcaster.messageCount, broadcaster.messageUpdateCount, broadcaster.taskCount, broadcaster.eventCount)
	}
}

func TestRegistryToolExecutorMarksHITLInterrupted(t *testing.T) {
	runtime := &fakeToolRuntime{}
	tool := &recordingTool{name: "request_user_decision", requiresHITL: true}
	registry, err := agenttools.NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}
	executor := NewRegistryToolExecutor(RegistryToolExecutorConfig{Registry: registry, Runtime: runtime})

	result, err := executor.ExecuteProducerTool(context.Background(), ProducerContext{
		Input: ProducerTurnInput{
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(2),
			TaskID:      uuidWithByte(3),
		},
	}, ToolCall{Name: "request_user_decision", Arguments: map[string]any{"title": "确认"}})
	if err != nil {
		t.Fatal(err)
	}

	if !result.Interrupted {
		t.Fatal("request_user_decision should interrupt producer turn")
	}
	if tool.receivedTaskID != uuidWithByte(3) {
		t.Fatal("HITL tool did not receive producer task id")
	}
}

type recordingTool struct {
	name           string
	requiresHITL   bool
	receivedTaskID pgtype.UUID
}

func (t *recordingTool) Definition() agenttools.Definition {
	return agenttools.Definition{
		Name:        t.name,
		Description: "recording tool",
		Parameters:  map[string]any{"type": "object"},
		Result:      map[string]any{"type": "object"},
		Safety:      agenttools.SafetySpec{RequiresHITL: t.requiresHITL},
	}
}

func (t *recordingTool) Execute(_ context.Context, input agenttools.ExecuteInput) (agenttools.ExecuteOutput, error) {
	t.receivedTaskID = input.TaskID
	return agenttools.ExecuteOutput{Result: map[string]any{"ok": true}}, nil
}

type fakeToolRuntime struct {
	messageTypes    []string
	messages        []db.AgentMessage
	updatedMessage  db.AgentMessage
	eventTypes      []string
	createdTaskType string
	runningTask     pgtype.UUID
	succeededTask   pgtype.UUID
	failedTask      pgtype.UUID
}

func (f *fakeToolRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.messageTypes = append(f.messageTypes, params.MessageType)
	msg := db.AgentMessage{
		ID:          uuidWithByte(byte(20 + len(f.messageTypes))),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		MessageType: params.MessageType,
		Content:     params.Content,
		RawMessage:  params.RawMessage,
		TaskID:      params.TaskID,
		EventID:     params.EventID,
	}
	f.messages = append(f.messages, msg)
	return msg, nil
}

func (f *fakeToolRuntime) UpdateMessage(_ context.Context, params agentruntime.UpdateMessageParams) (db.AgentMessage, error) {
	f.updatedMessage = db.AgentMessage{
		ID:          params.ID,
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		Role:        "assistant",
		MessageType: "tool_call",
		Content:     params.Content,
		RawMessage:  params.RawMessage,
		TaskID:      uuidWithByte(4),
		EventID:     params.EventID,
	}
	return f.updatedMessage, nil
}

func (f *fakeToolRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.eventTypes = append(f.eventTypes, params.EventType)
	return db.AgentEvent{
		ID:          uuidWithByte(byte(30 + len(f.eventTypes))),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		TaskID:      params.TaskID,
		EventType:   params.EventType,
		SourceRole:  params.SourceRole,
	}, nil
}

func (f *fakeToolRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	f.createdTaskType = params.TaskType
	return db.AgentTask{
		ID:          uuidWithByte(4),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		TaskType:    params.TaskType,
		Status:      "queued",
		Input:       params.Input,
	}, nil
}

func (f *fakeToolRuntime) MarkTaskRunning(_ context.Context, taskID pgtype.UUID) (db.AgentTask, error) {
	f.runningTask = taskID
	return db.AgentTask{ID: taskID, Status: "running"}, nil
}

func (f *fakeToolRuntime) MarkTaskSucceeded(_ context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error) {
	f.succeededTask = taskID
	var result map[string]any
	_ = json.Unmarshal(output, &result)
	return db.AgentTask{ID: taskID, Status: "succeeded", Output: output}, nil
}

func (f *fakeToolRuntime) MarkTaskFailed(_ context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error) {
	f.failedTask = taskID
	return db.AgentTask{ID: taskID, Status: "failed", ErrorCode: pgtype.Text{String: code, Valid: true}, ErrorMessage: pgtype.Text{String: message, Valid: true}}, nil
}

func assertToolStatusBlock(t *testing.T, raw []byte, toolName string, status string, wantArguments bool, wantResult bool) {
	t.Helper()
	var content struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type      string         `json:"type"`
			ToolName  string         `json:"tool_name"`
			Status    string         `json:"status"`
			Arguments map[string]any `json:"arguments"`
			Result    map[string]any `json:"result"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if content.Schema != uimessage.SchemaV1 {
		t.Fatalf("schema = %q", content.Schema)
	}
	if len(content.Blocks) != 1 || content.Blocks[0].Type != "tool_status" {
		t.Fatalf("blocks = %#v", content.Blocks)
	}
	if content.Blocks[0].ToolName != toolName || content.Blocks[0].Status != status {
		t.Fatalf("tool block = %#v", content.Blocks[0])
	}
	if (content.Blocks[0].Arguments != nil) != wantArguments {
		t.Fatalf("arguments = %#v, want present %v", content.Blocks[0].Arguments, wantArguments)
	}
	if (content.Blocks[0].Result != nil) != wantResult {
		t.Fatalf("result = %#v, want present %v", content.Blocks[0].Result, wantResult)
	}
}
