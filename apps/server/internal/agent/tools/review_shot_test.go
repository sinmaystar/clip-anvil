package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestReviewShotQueuesShotVideoReviewTask(t *testing.T) {
	store := &fakeReviewShotStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", Title: "开场", Status: "video_ready"},
		},
		nodes: []db.MediaNode{
			{
				ID:               uuidWithByte(3),
				WorkspaceID:      uuidWithByte(1),
				ShotID:           uuidWithByte(2),
				NodeType:         db.NodeTypeVideo,
				Title:            "shot-01 shot video",
				CurrentVersionID: uuidWithByte(4),
				Metadata:         []byte(`{"agent_artifact_kind":"shot_video"}`),
			},
		},
		versions: map[pgtype.UUID]db.ArtifactVersion{
			uuidWithByte(4): {ID: uuidWithByte(4), WorkspaceID: uuidWithByte(1), NodeID: uuidWithByte(3), JobID: uuidWithByte(5), Status: db.JobStatusSucceeded},
		},
	}
	runtime := &fakeReviewRuntime{}
	enqueuer := &fakeReviewerEnqueuer{}
	tool := NewReviewShotTool(store, runtime, enqueuer)

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		TaskID:      uuidWithByte(9),
		Arguments: map[string]any{
			"shot_refs":    []any{"shot-01"},
			"target_phase": "shot_video",
			"auto_retry":   true,
		},
	})
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
	if input["target_phase"] != "shot_video" || input["node_id"] != uuidString(uuidWithByte(3)) {
		t.Fatalf("task input = %#v", input)
	}
	if out.Result["status"] != "queued" {
		t.Fatalf("result = %#v", out.Result)
	}
}

type fakeReviewShotStore struct {
	workspace db.Workspace
	shots     []db.Shot
	nodes     []db.MediaNode
	versions  map[pgtype.UUID]db.ArtifactVersion
}

func (f *fakeReviewShotStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeReviewShotStore) ListActiveShotsByWorkspace(context.Context, pgtype.UUID) ([]db.Shot, error) {
	return f.shots, nil
}

func (f *fakeReviewShotStore) GetShotByID(_ context.Context, id pgtype.UUID) (db.Shot, error) {
	for _, shot := range f.shots {
		if shot.ID == id {
			return shot, nil
		}
	}
	return db.Shot{}, errShotNotFound
}

func (f *fakeReviewShotStore) GetShotByClientKey(_ context.Context, params db.GetShotByClientKeyParams) (db.Shot, error) {
	for _, shot := range f.shots {
		if shot.WorkspaceID == params.WorkspaceID && shot.ClientKey == params.ClientKey {
			return shot, nil
		}
	}
	return db.Shot{}, errShotNotFound
}

func (f *fakeReviewShotStore) ListMediaNodesByShot(_ context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error) {
	out := []db.MediaNode{}
	for _, node := range f.nodes {
		if node.WorkspaceID == params.WorkspaceID && node.ShotID == params.ShotID {
			out = append(out, node)
		}
	}
	return out, nil
}

func (f *fakeReviewShotStore) GetArtifactVersionByID(_ context.Context, id pgtype.UUID) (db.ArtifactVersion, error) {
	version, ok := f.versions[id]
	if !ok {
		return db.ArtifactVersion{}, errShotNotFound
	}
	return version, nil
}

type fakeReviewRuntime struct {
	createdTasks []db.AgentTask
}

func (f *fakeReviewRuntime) GetOrCreateReviewerThread(_ context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error) {
	return db.AgentThread{ID: uuidWithByte(7), WorkspaceID: workspaceID, Role: "reviewer", ScopeType: "shot", ScopeID: shotID}, nil
}

func (f *fakeReviewRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	task := db.AgentTask{ID: uuidWithByte(byte(60 + len(f.createdTasks))), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, Role: params.Role, ScopeType: params.ScopeType, ScopeID: params.ScopeID, TaskType: params.TaskType, Status: "queued", MaxAttempts: params.MaxAttempts, Input: params.Input}
	f.createdTasks = append(f.createdTasks, task)
	return task, nil
}

func (f *fakeReviewRuntime) CreateEvent(context.Context, agentruntime.CreateEventParams) (db.AgentEvent, error) {
	return db.AgentEvent{}, nil
}

type fakeReviewerEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeReviewerEnqueuer) EnqueueReviewerTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}
