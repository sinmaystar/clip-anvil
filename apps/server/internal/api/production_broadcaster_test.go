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

func TestProductionBroadcasterCreatesProducerSignalOnAgentWorkerCompletion(t *testing.T) {
	store := &fakeProductionBroadcasterStore{
		node: db.MediaNode{
			ID:          broadcasterUUID(20),
			WorkspaceID: broadcasterUUID(1),
			NodeType:    db.NodeTypeImage,
			ShotID:      broadcasterUUID(2),
			Metadata:    []byte(`{"agent_artifact_kind":"preview_image","shot_client_key":"shot_03"}`),
		},
		job: db.GenerationJob{
			ID:              broadcasterUUID(30),
			WorkspaceID:     broadcasterUUID(1),
			TargetNodeID:    broadcasterUUID(20),
			RequestedByType: "agent_worker",
			RequestedByID:   pgtype.Text{String: uuidString(broadcasterUUID(40)), Valid: true},
			Status:          db.JobStatusSucceeded,
			SemanticKey:     "job.shot_03.preview_image.r1",
			Intent:          []byte(`{"semantic":{"scope_key":"shot_03","render_plan_key":"shot_03.preview_image.r1","artifact_kind":"preview_image"}}`),
		},
		version: db.ArtifactVersion{ID: broadcasterUUID(50), NodeID: broadcasterUUID(20), JobID: broadcasterUUID(30), Status: db.JobStatusSucceeded, SemanticKey: "shot_03.preview_image.r1.artifact.v1"},
		task: db.AgentTask{
			ID:           broadcasterUUID(40),
			WorkspaceID:  broadcasterUUID(1),
			ThreadID:     broadcasterUUID(60),
			ScopeType:    "shot",
			ScopeID:      broadcasterUUID(2),
			RenderPlanID: broadcasterUUID(80),
			SemanticKey:  "worker.shot_03.preview_image.r1",
		},
	}
	sink := &fakeAgentPreviewEventSink{
		producerThread: db.AgentThread{ID: broadcasterUUID(90), WorkspaceID: broadcasterUUID(1), Role: "producer"},
		createdTask:    db.AgentTask{ID: broadcasterUUID(91), WorkspaceID: broadcasterUUID(1), ThreadID: broadcasterUUID(90), Role: "producer", TaskType: "producer_turn", Status: "queued"},
	}
	broadcaster := NewProductionBroadcaster(nil, store, nil)
	broadcaster.SetAgentPreviewEventSink(sink)

	broadcaster.PublishProductionEvent(production.ProductionEvent{
		WorkspaceID:  broadcasterUUID(1),
		TargetNodeID: broadcasterUUID(20),
		JobID:        broadcasterUUID(30),
		Type:         production.ProductionEventJobSucceeded,
		Progress:     100,
	})

	if len(sink.signals) != 1 {
		t.Fatalf("signals = %#v", sink.signals)
	}
	signal := sink.signals[0]
	if signal.SignalType != "worker_generation_completed" ||
		signal.ProducerThreadID != broadcasterUUID(90) ||
		signal.SourceRole != "worker" ||
		signal.SourceTaskID != broadcasterUUID(40) ||
		signal.SourceThreadID != broadcasterUUID(60) ||
		signal.ScopeType != "shot" ||
		signal.ScopeID != broadcasterUUID(2) ||
		signal.RenderPlanID != broadcasterUUID(80) ||
		signal.DedupeKey != "worker_generation_completed:1e000000-0000-0000-0000-000000000000" {
		t.Fatalf("signal = %#v", signal)
	}
	var payload map[string]any
	if err := json.Unmarshal(signal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["generation_job_id"] != uuidString(broadcasterUUID(30)) ||
		payload["artifact_version_id"] != uuidString(broadcasterUUID(50)) ||
		payload["render_plan_status"] != "succeeded" ||
		payload["target_phase"] != "preview_image" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["scope_key"] != "shot_03" ||
		payload["render_plan_key"] != "shot_03.preview_image.r1" ||
		payload["generation_job_key"] != "job.shot_03.preview_image.r1" ||
		payload["artifact_version_key"] != "shot_03.preview_image.r1.artifact.v1" {
		t.Fatalf("payload semantic keys = %#v", payload)
	}
	if len(sink.enqueuedProducerTasks) != 1 || sink.enqueuedProducerTasks[0].ID != broadcasterUUID(91) {
		t.Fatalf("enqueued producer tasks = %#v", sink.enqueuedProducerTasks)
	}
}

func TestProductionBroadcasterCreatesProducerSignalOnAgentComposerCompletion(t *testing.T) {
	store := &fakeProductionBroadcasterStore{
		node: db.MediaNode{
			ID:            broadcasterUUID(20),
			WorkspaceID:   broadcasterUUID(1),
			NodeType:      db.NodeTypeVideo,
			OperationType: "compose_final_video",
			Metadata:      []byte(`{"agent_artifact_kind":"final_video"}`),
			SemanticKey:   "final_video.0c59850d.node",
			ArtifactKind:  "final_video",
		},
		job: db.GenerationJob{
			ID:              broadcasterUUID(30),
			WorkspaceID:     broadcasterUUID(1),
			TargetNodeID:    broadcasterUUID(20),
			RequestedByType: "agent_composer",
			RequestedByID:   pgtype.Text{String: uuidString(broadcasterUUID(40)), Valid: true},
			Status:          db.JobStatusSucceeded,
			SemanticKey:     "final_video.0c59850d.compose.job.a1",
			Intent:          []byte(`{"semantic":{"render_plan_key":"final_video.0c59850d.compose","artifact_kind":"final_video"}}`),
		},
		version: db.ArtifactVersion{
			ID:           broadcasterUUID(50),
			NodeID:       broadcasterUUID(20),
			JobID:        broadcasterUUID(30),
			Status:       db.JobStatusSucceeded,
			SemanticKey:  "final_video.0c59850d.compose.artifact.v1",
			ArtifactKind: "final_video",
		},
		task: db.AgentTask{
			ID:          broadcasterUUID(40),
			WorkspaceID: broadcasterUUID(1),
			ThreadID:    broadcasterUUID(60),
			Role:        "composer",
			ScopeType:   "final_output",
			ScopeID:     broadcasterUUID(20),
			SemanticKey: "composer.final_output.0c59850d.composer_turn.40000000",
		},
	}
	sink := &fakeAgentPreviewEventSink{
		producerThread: db.AgentThread{ID: broadcasterUUID(90), WorkspaceID: broadcasterUUID(1), Role: "producer"},
		createdTask:    db.AgentTask{ID: broadcasterUUID(91), WorkspaceID: broadcasterUUID(1), ThreadID: broadcasterUUID(90), Role: "producer", TaskType: "producer_turn", Status: "queued"},
	}
	broadcaster := NewProductionBroadcaster(nil, store, nil)
	broadcaster.SetAgentPreviewEventSink(sink)

	broadcaster.PublishProductionEvent(production.ProductionEvent{
		WorkspaceID:  broadcasterUUID(1),
		TargetNodeID: broadcasterUUID(20),
		JobID:        broadcasterUUID(30),
		Type:         production.ProductionEventJobSucceeded,
		Progress:     100,
	})

	if len(sink.signals) != 1 {
		t.Fatalf("signals = %#v", sink.signals)
	}
	signal := sink.signals[0]
	if signal.SignalType != "composition_completed" ||
		signal.ProducerThreadID != broadcasterUUID(90) ||
		signal.SourceRole != "composer" ||
		signal.SourceTaskID != broadcasterUUID(40) ||
		signal.SourceThreadID != broadcasterUUID(60) ||
		signal.ScopeType != "final_output" ||
		signal.ScopeID != broadcasterUUID(20) ||
		signal.DedupeKey != "composition_completed:1e000000-0000-0000-0000-000000000000" {
		t.Fatalf("signal = %#v", signal)
	}
	var payload map[string]any
	if err := json.Unmarshal(signal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["trigger"] != "composition_completed" ||
		payload["node_key"] != "final_video.0c59850d.node" ||
		payload["generation_job_key"] != "final_video.0c59850d.compose.job.a1" ||
		payload["artifact_version_key"] != "final_video.0c59850d.compose.artifact.v1" ||
		payload["artifact_kind"] != "final_video" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(sink.enqueuedProducerTasks) != 1 || sink.enqueuedProducerTasks[0].ID != broadcasterUUID(91) {
		t.Fatalf("enqueued producer tasks = %#v", sink.enqueuedProducerTasks)
	}
}

func TestProductionBroadcasterDoesNotWakeProducerWhenProducerRunning(t *testing.T) {
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
		task: db.AgentTask{
			ID:           broadcasterUUID(40),
			WorkspaceID:  broadcasterUUID(1),
			ThreadID:     broadcasterUUID(60),
			ScopeType:    "shot",
			ScopeID:      broadcasterUUID(2),
			RenderPlanID: broadcasterUUID(80),
		},
	}
	sink := &fakeAgentPreviewEventSink{
		producerThread: db.AgentThread{ID: broadcasterUUID(90), WorkspaceID: broadcasterUUID(1), Role: "producer"},
		activeTasks: []db.AgentTask{
			{ID: broadcasterUUID(92), Role: "producer", TaskType: "producer_turn", Status: "running"},
		},
	}
	broadcaster := NewProductionBroadcaster(nil, store, nil)
	broadcaster.SetAgentPreviewEventSink(sink)

	broadcaster.PublishProductionEvent(production.ProductionEvent{
		WorkspaceID:  broadcasterUUID(1),
		TargetNodeID: broadcasterUUID(20),
		JobID:        broadcasterUUID(30),
		Type:         production.ProductionEventJobSucceeded,
		Progress:     100,
	})

	if len(sink.signals) != 1 {
		t.Fatalf("signals = %#v", sink.signals)
	}
	if len(sink.enqueuedProducerTasks) != 0 {
		t.Fatalf("enqueued producer tasks = %#v", sink.enqueuedProducerTasks)
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
	created               agentruntime.CreateEventParams
	broadcast             db.AgentEvent
	producerThread        db.AgentThread
	activeTasks           []db.AgentTask
	createdTask           db.AgentTask
	createdTasks          []agentruntime.CreateTaskParams
	signals               []agentruntime.CreateProducerPendingSignalParams
	enqueuedProducerTasks []db.AgentTask
}

func (f *fakeAgentPreviewEventSink) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.created = params
	return db.AgentEvent{ID: broadcasterUUID(70), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskID: params.TaskID, EventType: params.EventType}, nil
}

func (f *fakeAgentPreviewEventSink) BroadcastAgentEvent(_ pgtype.UUID, event db.AgentEvent) {
	f.broadcast = event
}

func (f *fakeAgentPreviewEventSink) GetOrCreateProducerThread(context.Context, pgtype.UUID) (db.AgentThread, error) {
	return f.producerThread, nil
}

func (f *fakeAgentPreviewEventSink) CreateProducerPendingSignal(_ context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error) {
	f.signals = append(f.signals, params)
	return db.ProducerPendingSignal{ID: broadcasterUUID(71), WorkspaceID: params.WorkspaceID, ProducerThreadID: params.ProducerThreadID, SignalType: params.SignalType}, nil
}

func (f *fakeAgentPreviewEventSink) ListActiveAgentTasksByWorkspace(context.Context, pgtype.UUID) ([]db.AgentTask, error) {
	return f.activeTasks, nil
}

func (f *fakeAgentPreviewEventSink) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	f.createdTasks = append(f.createdTasks, params)
	if f.createdTask.ID.Valid {
		return f.createdTask, nil
	}
	return db.AgentTask{ID: broadcasterUUID(91), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, Role: params.Role, TaskType: params.TaskType, Status: "queued"}, nil
}

func (f *fakeAgentPreviewEventSink) EnqueueProducerTask(_ context.Context, task db.AgentTask) {
	f.enqueuedProducerTasks = append(f.enqueuedProducerTasks, task)
}

func broadcasterUUID(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}
