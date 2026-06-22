package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestDispatchCraftsmanDefinition(t *testing.T) {
	tool := NewDispatchCraftsmanTool(&fakeCraftsmanDispatchStore{}, &fakeCraftsmanRuntime{}, &fakeCraftsmanEnqueuer{})
	def := tool.Definition()
	if def.Name != "dispatch_craftsman" {
		t.Fatalf("Name = %q", def.Name)
	}
	mode, ok := def.Parameters["properties"].(map[string]any)["mode"].(map[string]any)
	if !ok {
		t.Fatalf("mode schema missing: %#v", def.Parameters)
	}
	enum, ok := mode["enum"].([]string)
	if !ok || len(enum) != 1 || enum[0] != "preview_image" {
		t.Fatalf("mode enum = %#v", mode["enum"])
	}
	if !def.Safety.UsesProductionService || def.Safety.WritesCanvas {
		t.Fatalf("Safety = %#v", def.Safety)
	}
	if def.Visibility.UserLabel != "开始生成预览图" {
		t.Fatalf("UserLabel = %q", def.Visibility.UserLabel)
	}
}

func TestDispatchCraftsmanDispatchesAllActiveShotsByDefault(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", Title: "开场", Status: "planned"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", Title: "演示", Status: "draft"},
			{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), ClientKey: "shot-03", Title: "收尾", Status: "failed"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	enqueuer := &fakeCraftsmanEnqueuer{}
	tool := NewDispatchCraftsmanTool(store, runtime, enqueuer)

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments:   map[string]any{"mode": "preview_image"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatched := out.Result["dispatched"].([]map[string]any)
	if len(dispatched) != 3 {
		t.Fatalf("dispatched = %#v", out.Result["dispatched"])
	}
	if len(runtime.createdTasks) != 3 || len(enqueuer.tasks) != 3 {
		t.Fatalf("created tasks = %d, enqueued = %d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	for _, task := range runtime.createdTasks {
		if task.Role != "craftsman" || task.TaskType != "craftsman_turn" || task.ScopeType != "shot" {
			t.Fatalf("task = %#v", task)
		}
		if task.MaxAttempts != 3 {
			t.Fatalf("max attempts = %d", task.MaxAttempts)
		}
		var input map[string]any
		if err := json.Unmarshal(task.Input, &input); err != nil {
			t.Fatal(err)
		}
		if input["mode"] != "preview_image" {
			t.Fatalf("task input = %#v", input)
		}
	}
}

func TestDispatchCraftsmanResolvesShotRefsAndCapsAttempts(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", Title: "开场", Status: "planned"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", Title: "演示", Status: "planned"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanTool(store, runtime, &fakeCraftsmanEnqueuer{})

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"mode":         "preview_image",
			"shot_refs":    []any{"shot-02"},
			"max_attempts": 99.0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatched := out.Result["dispatched"].([]map[string]any)
	if len(dispatched) != 1 || dispatched[0]["client_key"] != "shot-02" {
		t.Fatalf("dispatched = %#v", dispatched)
	}
	if len(runtime.createdTasks) != 1 {
		t.Fatalf("created tasks = %d", len(runtime.createdTasks))
	}
	if runtime.createdTasks[0].MaxAttempts != 3 {
		t.Fatalf("max attempts = %d", runtime.createdTasks[0].MaxAttempts)
	}
}

type fakeCraftsmanDispatchStore struct {
	workspace db.Workspace
	shots     []db.Shot
	linked    []db.SetShotCraftsmanThreadParams
}

func (f *fakeCraftsmanDispatchStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeCraftsmanDispatchStore) ListActiveShotsByWorkspace(context.Context, pgtype.UUID) ([]db.Shot, error) {
	return f.shots, nil
}

func (f *fakeCraftsmanDispatchStore) GetShotByID(_ context.Context, id pgtype.UUID) (db.Shot, error) {
	for _, shot := range f.shots {
		if shot.ID == id {
			return shot, nil
		}
	}
	return db.Shot{}, errShotNotFound
}

func (f *fakeCraftsmanDispatchStore) GetShotByClientKey(_ context.Context, params db.GetShotByClientKeyParams) (db.Shot, error) {
	for _, shot := range f.shots {
		if shot.WorkspaceID == params.WorkspaceID && shot.ClientKey == params.ClientKey {
			return shot, nil
		}
	}
	return db.Shot{}, errShotNotFound
}

func (f *fakeCraftsmanDispatchStore) SetShotCraftsmanThread(_ context.Context, params db.SetShotCraftsmanThreadParams) (db.Shot, error) {
	f.linked = append(f.linked, params)
	for _, shot := range f.shots {
		if shot.ID == params.ID {
			shot.CraftsmanThreadID = params.CraftsmanThreadID
			return shot, nil
		}
	}
	return db.Shot{}, errShotNotFound
}

type fakeCraftsmanRuntime struct {
	createdTasks []db.AgentTask
	events       []agentruntime.CreateEventParams
}

func (f *fakeCraftsmanRuntime) GetOrCreateCraftsmanThread(_ context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error) {
	return db.AgentThread{ID: pgtype.UUID{Bytes: shotID.Bytes, Valid: true}, WorkspaceID: workspaceID, Role: "craftsman", ScopeType: "shot", ScopeID: shotID}, nil
}

func (f *fakeCraftsmanRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	task := db.AgentTask{
		ID:          uuidWithByte(byte(80 + len(f.createdTasks))),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		ScopeType:   params.ScopeType,
		ScopeID:     params.ScopeID,
		TaskType:    params.TaskType,
		Status:      "queued",
		MaxAttempts: params.MaxAttempts,
		Input:       params.Input,
	}
	f.createdTasks = append(f.createdTasks, task)
	return task, nil
}

func (f *fakeCraftsmanRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.events = append(f.events, params)
	return db.AgentEvent{ID: uuidWithByte(90), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskID: params.TaskID, EventType: params.EventType}, nil
}

type fakeCraftsmanEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeCraftsmanEnqueuer) EnqueueCraftsmanTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}
