package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestComposeFinalQueuesComposerTaskWithShotVideoWinners(t *testing.T) {
	store := &fakeComposeFinalStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SortOrder: 1, Status: "video_ready"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", SortOrder: 2, Status: "video_ready"},
		},
		nodes: map[pgtype.UUID][]db.MediaNode{
			uuidWithByte(11): {{ID: uuidWithByte(21), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(11), NodeType: db.NodeTypeVideo, Title: "shot-01 shot video", CurrentVersionID: uuidWithByte(31), Metadata: []byte(`{"agent_artifact_kind":"shot_video"}`)}},
			uuidWithByte(12): {{ID: uuidWithByte(22), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(12), NodeType: db.NodeTypeVideo, Title: "shot-02 shot video", CurrentVersionID: uuidWithByte(32), Metadata: []byte(`{"agent_artifact_kind":"shot_video"}`)}},
		},
		versions: map[pgtype.UUID]db.ArtifactVersion{
			uuidWithByte(31): {ID: uuidWithByte(31), WorkspaceID: uuidWithByte(1), NodeID: uuidWithByte(21), Status: db.JobStatusSucceeded},
			uuidWithByte(32): {ID: uuidWithByte(32), WorkspaceID: uuidWithByte(1), NodeID: uuidWithByte(22), Status: db.JobStatusSucceeded},
		},
	}
	runtime := &fakeComposeRuntime{}
	enqueuer := &fakeComposerEnqueuer{}
	tool := NewComposeFinalTool(store, runtime, enqueuer)

	_, err := tool.Execute(context.Background(), ExecuteInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3), Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.createdTasks) != 1 || len(enqueuer.tasks) != 1 {
		t.Fatalf("created=%d enqueued=%d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.createdTasks[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	refs, _ := input["video_node_refs"].([]any)
	if len(refs) != 2 || refs[0] != "shot-01 shot video" || refs[1] != "shot-02 shot video" {
		t.Fatalf("task input = %#v", input)
	}
	if runtime.createdTasks[0].Role != "composer" || runtime.createdTasks[0].TaskType != "composer_turn" {
		t.Fatalf("task = %#v", runtime.createdTasks[0])
	}
}

type fakeComposeFinalStore struct {
	workspace db.Workspace
	shots     []db.Shot
	nodes     map[pgtype.UUID][]db.MediaNode
	versions  map[pgtype.UUID]db.ArtifactVersion
}

func (f *fakeComposeFinalStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeComposeFinalStore) ListActiveShotsByWorkspace(context.Context, pgtype.UUID) ([]db.Shot, error) {
	return f.shots, nil
}

func (f *fakeComposeFinalStore) ListMediaNodesByShot(_ context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error) {
	return f.nodes[params.ShotID], nil
}

func (f *fakeComposeFinalStore) GetArtifactVersionByID(_ context.Context, id pgtype.UUID) (db.ArtifactVersion, error) {
	return f.versions[id], nil
}

type fakeComposeRuntime struct {
	createdTasks []db.AgentTask
}

func (f *fakeComposeRuntime) GetOrCreateComposerThread(_ context.Context, workspaceID pgtype.UUID) (db.AgentThread, error) {
	return db.AgentThread{ID: uuidWithByte(44), WorkspaceID: workspaceID, Role: "composer", ScopeType: "final_output"}, nil
}

func (f *fakeComposeRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	task := db.AgentTask{ID: uuidWithByte(byte(70 + len(f.createdTasks))), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, Role: params.Role, ScopeType: params.ScopeType, ScopeID: params.ScopeID, TaskType: params.TaskType, Status: "queued", MaxAttempts: params.MaxAttempts, Input: params.Input}
	f.createdTasks = append(f.createdTasks, task)
	return task, nil
}

func (f *fakeComposeRuntime) CreateEvent(context.Context, agentruntime.CreateEventParams) (db.AgentEvent, error) {
	return db.AgentEvent{}, nil
}

type fakeComposerEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeComposerEnqueuer) EnqueueComposerTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}
