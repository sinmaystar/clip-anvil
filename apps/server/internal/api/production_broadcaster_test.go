package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestProductionBroadcasterEmitsAgentPreviewEventOnSuccess(t *testing.T) {
	store := &fakeProductionBroadcasterStore{
		node: db.MediaNode{
			ID:          broadcasterUUID(20),
			WorkspaceID: broadcasterUUID(1),
			NodeType:    db.NodeTypeImage,
			ShotID:      broadcasterUUID(2),
			Metadata:    []byte(`{"agent_artifact_kind":"preview_image"}`),
		},
		job: db.GenerationJob{
			ID:              broadcasterUUID(30),
			WorkspaceID:     broadcasterUUID(1),
			TargetNodeID:    broadcasterUUID(20),
			RequestedByType: "agent_worker",
			RequestedByID:   pgtype.Text{String: uuidString(broadcasterUUID(40)), Valid: true},
			Status:          db.JobStatusSucceeded,
		},
		version: db.ArtifactVersion{ID: broadcasterUUID(50), NodeID: broadcasterUUID(20), JobID: broadcasterUUID(30), Status: db.JobStatusSucceeded},
		task:    db.AgentTask{ID: broadcasterUUID(40), WorkspaceID: broadcasterUUID(1), ThreadID: broadcasterUUID(60), ScopeType: "shot", ScopeID: broadcasterUUID(2)},
	}
	sink := &fakeAgentPreviewEventSink{}
	broadcaster := NewProductionBroadcaster(nil, store, nil)
	broadcaster.SetAgentPreviewEventSink(sink)

	broadcaster.PublishProductionEvent(production.ProductionEvent{
		WorkspaceID:  broadcasterUUID(1),
		TargetNodeID: broadcasterUUID(20),
		JobID:        broadcasterUUID(30),
		Type:         production.ProductionEventJobSucceeded,
		Progress:     100,
	})

	if len(store.statusUpdates) != 1 {
		t.Fatalf("status updates = %#v", store.statusUpdates)
	}
	if store.statusUpdates[0].ID != broadcasterUUID(2) || store.statusUpdates[0].Status != "preview_ready" {
		t.Fatalf("status update = %#v", store.statusUpdates[0])
	}
	if sink.created.EventType != "preview_generation_succeeded" {
		t.Fatalf("event params = %#v", sink.created)
	}
	if sink.created.ThreadID != broadcasterUUID(60) || sink.created.TaskID != broadcasterUUID(40) {
		t.Fatalf("event linkage = %#v", sink.created)
	}
	if sink.broadcast.ID != broadcasterUUID(70) {
		t.Fatalf("broadcast event = %#v", sink.broadcast)
	}
	var payload map[string]any
	if err := json.Unmarshal(sink.created.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["node_id"] != uuidString(broadcasterUUID(20)) || payload["job_id"] != uuidString(broadcasterUUID(30)) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestProductionBroadcasterEmitsAgentShotVideoEventOnSuccess(t *testing.T) {
	store := &fakeProductionBroadcasterStore{
		node: db.MediaNode{
			ID:          broadcasterUUID(20),
			WorkspaceID: broadcasterUUID(1),
			NodeType:    db.NodeTypeVideo,
			ShotID:      broadcasterUUID(2),
			Metadata:    []byte(`{"agent_artifact_kind":"shot_video"}`),
		},
		job: db.GenerationJob{
			ID:              broadcasterUUID(30),
			WorkspaceID:     broadcasterUUID(1),
			TargetNodeID:    broadcasterUUID(20),
			RequestedByType: "agent_worker",
			RequestedByID:   pgtype.Text{String: uuidString(broadcasterUUID(40)), Valid: true},
			Status:          db.JobStatusSucceeded,
		},
		version: db.ArtifactVersion{ID: broadcasterUUID(50), NodeID: broadcasterUUID(20), JobID: broadcasterUUID(30), Status: db.JobStatusSucceeded},
		task:    db.AgentTask{ID: broadcasterUUID(40), WorkspaceID: broadcasterUUID(1), ThreadID: broadcasterUUID(60), ScopeType: "shot", ScopeID: broadcasterUUID(2)},
	}
	sink := &fakeAgentPreviewEventSink{}
	broadcaster := NewProductionBroadcaster(nil, store, nil)
	broadcaster.SetAgentPreviewEventSink(sink)

	broadcaster.PublishProductionEvent(production.ProductionEvent{
		WorkspaceID:  broadcasterUUID(1),
		TargetNodeID: broadcasterUUID(20),
		JobID:        broadcasterUUID(30),
		Type:         production.ProductionEventJobSucceeded,
		Progress:     100,
	})

	if len(store.statusUpdates) != 1 {
		t.Fatalf("status updates = %#v", store.statusUpdates)
	}
	if store.statusUpdates[0].ID != broadcasterUUID(2) || store.statusUpdates[0].Status != "video_ready" {
		t.Fatalf("status update = %#v", store.statusUpdates[0])
	}
	if sink.created.EventType != "shot_video_succeeded" {
		t.Fatalf("event params = %#v", sink.created)
	}
}

type fakeProductionBroadcasterStore struct {
	node          db.MediaNode
	job           db.GenerationJob
	version       db.ArtifactVersion
	task          db.AgentTask
	statusUpdates []db.UpdateShotStatusParams
}

func (f *fakeProductionBroadcasterStore) GetMediaNodeByID(context.Context, pgtype.UUID) (db.MediaNode, error) {
	return f.node, nil
}

func (f *fakeProductionBroadcasterStore) GetGenerationJobByID(context.Context, pgtype.UUID) (db.GenerationJob, error) {
	return f.job, nil
}

func (f *fakeProductionBroadcasterStore) GetAgentTaskByID(context.Context, pgtype.UUID) (db.AgentTask, error) {
	return f.task, nil
}

func (f *fakeProductionBroadcasterStore) GetArtifactVersionByJobID(context.Context, pgtype.UUID) (db.ArtifactVersion, error) {
	return f.version, nil
}

func (f *fakeProductionBroadcasterStore) GetArtifactVersionByID(context.Context, pgtype.UUID) (db.ArtifactVersion, error) {
	return f.version, nil
}

func (f *fakeProductionBroadcasterStore) UpdateShotStatus(_ context.Context, params db.UpdateShotStatusParams) (db.Shot, error) {
	f.statusUpdates = append(f.statusUpdates, params)
	return db.Shot{ID: params.ID, WorkspaceID: params.WorkspaceID, Status: params.Status}, nil
}

func (f *fakeProductionBroadcasterStore) ListMediaAssetsByWorkspace(context.Context, pgtype.UUID) ([]db.MediaAsset, error) {
	return nil, nil
}

func (f *fakeProductionBroadcasterStore) ListActiveStaleReasonsByNode(context.Context, pgtype.UUID) ([]db.NodeStaleReason, error) {
	return nil, nil
}

func (f *fakeProductionBroadcasterStore) ListReferencePackItemNodes(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return nil, nil
}

type fakeAgentPreviewEventSink struct {
	created   agentruntime.CreateEventParams
	broadcast db.AgentEvent
}

func (f *fakeAgentPreviewEventSink) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.created = params
	return db.AgentEvent{ID: broadcasterUUID(70), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskID: params.TaskID, EventType: params.EventType}, nil
}

func (f *fakeAgentPreviewEventSink) BroadcastAgentEvent(_ pgtype.UUID, event db.AgentEvent) {
	f.broadcast = event
}

func broadcasterUUID(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}
