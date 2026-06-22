package hitl

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestRequestUserDecisionCreatesCardEventCheckpointAndWaitingTask(t *testing.T) {
	runtime := &fakeDecisionRuntime{}
	service := NewService(runtime)

	out, err := service.RequestUserDecision(context.Background(), RequestDecisionInput{
		WorkspaceID:   uuidWithByte(1),
		ThreadID:      uuidWithByte(2),
		TaskID:        uuidWithByte(3),
		CheckpointKey: "cp-1",
		Arguments: map[string]any{
			"title":   "确认方向",
			"message": "选择转化还是品牌",
			"options": []any{
				map[string]any{"id": "conversion", "label": "更偏转化"},
			},
		},
		CheckpointValue: []byte("checkpoint"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.waitingTask != uuidWithByte(3) {
		t.Fatal("producer task was not marked waiting_for_user")
	}
	if runtime.checkpointKey != "cp-1" {
		t.Fatalf("checkpoint key = %q", runtime.checkpointKey)
	}
	if runtime.cardMessageType != "ui_card" {
		t.Fatalf("card message type = %q", runtime.cardMessageType)
	}
	assertDecisionCardBlock(t, runtime.cardMessageContent, "确认方向", "pending")
	if out.EventID == "" {
		t.Fatalf("output = %#v", out)
	}
}

func TestRespondDecisionRejectsInvalidOption(t *testing.T) {
	runtime := &fakeDecisionRuntime{
		event: decisionEvent([]byte(`{"options":[{"id":"a","label":"A"}]}`)),
	}
	service := NewService(runtime)

	_, err := service.RespondDecision(context.Background(), RespondDecisionInput{
		WorkspaceID:       uuidWithByte(1),
		EventID:           uuidWithByte(8),
		SelectedOptionID:  "missing",
		ClientResponseID:  "client-1",
		ResumeTaskThread:  uuidWithByte(2),
		ResumeTaskOwnerID: uuidWithByte(3),
	})
	if !errors.Is(err, ErrInvalidDecisionResponse) {
		t.Fatalf("error = %v, want ErrInvalidDecisionResponse", err)
	}
}

func TestRespondDecisionCreatesResumeTaskAndResolvedEvent(t *testing.T) {
	runtime := &fakeDecisionRuntime{
		event: decisionEvent([]byte(`{"title":"确认方向","options":[{"id":"a","label":"A"}]}`)),
	}
	service := NewService(runtime)

	out, err := service.RespondDecision(context.Background(), RespondDecisionInput{
		WorkspaceID:       uuidWithByte(1),
		EventID:           uuidWithByte(8),
		SelectedOptionID:  "a",
		ClientResponseID:  "client-1",
		ResumeTaskThread:  uuidWithByte(2),
		ResumeTaskOwnerID: uuidWithByte(3),
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.handledEvent != uuidWithByte(8) {
		t.Fatal("decision event was not marked handled")
	}
	if runtime.createdTaskType != "decision_resume" {
		t.Fatalf("task type = %q", runtime.createdTaskType)
	}
	if out.Task.ID != uuidWithByte(7) {
		t.Fatalf("task = %#v", out.Task)
	}
}

type fakeDecisionRuntime struct {
	event              db.AgentEvent
	waitingTask        pgtype.UUID
	checkpointKey      string
	cardMessageType    string
	cardMessageContent []byte
	handledEvent       pgtype.UUID
	createdTaskType    string
}

func (f *fakeDecisionRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	event := db.AgentEvent{
		ID:          uuidWithByte(8),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		TaskID:      params.TaskID,
		EventType:   params.EventType,
		SourceRole:  params.SourceRole,
		Payload:     params.Payload,
		Status:      "pending",
	}
	return event, nil
}

func (f *fakeDecisionRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.cardMessageType = params.MessageType
	f.cardMessageContent = params.Content
	return db.AgentMessage{ID: uuidWithByte(9), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, Role: params.Role, MessageType: params.MessageType, Content: params.Content, EventID: params.EventID}, nil
}

func (f *fakeDecisionRuntime) UpsertCheckpoint(_ context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error) {
	f.checkpointKey = params.Key
	return db.EinoCheckpoint{Key: params.Key, Value: params.Value}, nil
}

func (f *fakeDecisionRuntime) SetThreadCheckpoint(_ context.Context, _ pgtype.UUID, checkpointKey string) (db.AgentThread, error) {
	f.checkpointKey = checkpointKey
	return db.AgentThread{}, nil
}

func (f *fakeDecisionRuntime) MarkTaskWaitingForUser(_ context.Context, taskID pgtype.UUID) (db.AgentTask, error) {
	f.waitingTask = taskID
	return db.AgentTask{ID: taskID, Status: "waiting_for_user"}, nil
}

func (f *fakeDecisionRuntime) GetAgentEventForWorkspace(_ context.Context, eventID, _ pgtype.UUID) (db.AgentEvent, error) {
	f.event.ID = eventID
	return f.event, nil
}

func (f *fakeDecisionRuntime) MarkEventHandled(_ context.Context, eventID pgtype.UUID) (db.AgentEvent, error) {
	f.handledEvent = eventID
	f.event.Status = "handled"
	return f.event, nil
}

func (f *fakeDecisionRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	f.createdTaskType = params.TaskType
	return db.AgentTask{ID: uuidWithByte(7), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskType: params.TaskType, Status: "queued", Input: params.Input}, nil
}

func decisionEvent(payload []byte) db.AgentEvent {
	return db.AgentEvent{
		ID:          uuidWithByte(8),
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		EventType:   "decision_requested",
		SourceRole:  "producer",
		Payload:     payload,
		Status:      "pending",
	}
}

func assertDecisionCardBlock(t *testing.T, raw []byte, title string, status string) {
	t.Helper()
	var content struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if content.Schema != uimessage.SchemaV1 {
		t.Fatalf("schema = %q", content.Schema)
	}
	if len(content.Blocks) != 1 || content.Blocks[0].Type != "decision_card" {
		t.Fatalf("blocks = %#v", content.Blocks)
	}
	if content.Blocks[0].Title != title || content.Blocks[0].Status != status {
		t.Fatalf("decision block = %#v", content.Blocks[0])
	}
}
