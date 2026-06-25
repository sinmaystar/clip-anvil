package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestReviewerGraphAcceptsAndSelectsVersion(t *testing.T) {
	input := GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		TaskID:      uuidWithByte(9),
		Task: TaskInput{
			TargetPhase:       TargetPhasePreviewImage,
			ShotID:            uuidString(uuidWithByte(2)),
			NodeID:            uuidString(uuidWithByte(3)),
			ArtifactVersionID: uuidString(uuidWithByte(4)),
			GenerationJobID:   uuidString(uuidWithByte(6)),
			AttemptNo:         1,
			MaxAttempts:       3,
			AutoRetry:         true,
		},
	}
	runtime := &fakeReviewerRuntime{}
	store := &fakeReviewStore{}
	selector := &fakeVersionSelector{}
	dependency := &fakeDependencyNotifier{}
	graph, err := NewGraph(GraphConfig{
		Loader: fakeReviewLoader{context: Context{
			Input:   input,
			Shot:    db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", Title: "开场"},
			Node:    db.MediaNode{ID: uuidWithByte(3), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), Title: "shot-01 preview"},
			Version: db.ArtifactVersion{ID: uuidWithByte(4), WorkspaceID: uuidWithByte(1), NodeID: uuidWithByte(3), VersionNo: 1, Status: db.JobStatusSucceeded},
		}},
		Responder:  fakeReviewResponder{result: passingReviewResult()},
		Runtime:    runtime,
		Store:      store,
		Selector:   selector,
		Dependency: dependency,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision.Status != ReviewStatusAccepted {
		t.Fatalf("decision = %#v", out.Decision)
	}
	if !selector.called || selector.nodeID != uuidWithByte(3) || selector.versionID != uuidWithByte(4) {
		t.Fatalf("selector called=%v node=%v version=%v", selector.called, selector.nodeID, selector.versionID)
	}
	if store.completedStatus != ReviewStatusAccepted {
		t.Fatalf("completed status = %q", store.completedStatus)
	}
	if !strings.Contains(string(runtime.appendedContent), `"type":"review_card"`) {
		t.Fatalf("message content = %s", runtime.appendedContent)
	}
	if runtime.eventTypes[0] != "review_started" || runtime.eventTypes[len(runtime.eventTypes)-1] != "review_accepted" {
		t.Fatalf("events = %#v", runtime.eventTypes)
	}
	if !dependency.called || dependency.phase != "review" || dependency.shotID != uuidWithByte(2) {
		t.Fatalf("dependency notifier = %#v", dependency)
	}
}

func TestReviewerGraphCompileCapturesGraphInfo(t *testing.T) {
	registry := agenteino.NewGraphInfoRegistry()
	input := GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		TaskID:      uuidWithByte(9),
		Task: TaskInput{
			TargetPhase:       TargetPhasePreviewImage,
			ShotID:            uuidString(uuidWithByte(2)),
			NodeID:            uuidString(uuidWithByte(3)),
			ArtifactVersionID: uuidString(uuidWithByte(4)),
		},
	}
	_, err := NewGraph(GraphConfig{
		Loader:    fakeReviewLoader{context: Context{Input: input}},
		Responder: fakeReviewResponder{result: passingReviewResult()},
		Runtime:   &fakeReviewerRuntime{},
		Store:     &fakeReviewStore{},
		CompileCallbacks: []compose.GraphCompileCallback{
			registry.CompileCallback(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, ok := registry.Get("reviewer_preview")
	if !ok {
		t.Fatal("reviewer graph info was not captured")
	}
	for _, node := range []string{"load_review_context", "review_artifact"} {
		if _, ok := info.Nodes[node]; !ok {
			t.Fatalf("node %q missing from graph info", node)
		}
	}
}

func TestReviewerGraphDispatchesRetryWhenRejected(t *testing.T) {
	input := GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		TaskID:      uuidWithByte(9),
		Task: TaskInput{
			TargetPhase:       TargetPhasePreviewImage,
			ShotID:            uuidString(uuidWithByte(2)),
			NodeID:            uuidString(uuidWithByte(3)),
			ArtifactVersionID: uuidString(uuidWithByte(4)),
			AttemptNo:         1,
			MaxAttempts:       3,
			AutoRetry:         true,
		},
	}
	runtime := &fakeReviewerRuntime{}
	store := &fakeReviewStore{}
	retryDispatcher := &fakeRetryDispatcher{}
	graph, err := NewGraph(GraphConfig{
		Loader: fakeReviewLoader{context: Context{
			Input:   input,
			Shot:    db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", Title: "开场"},
			Node:    db.MediaNode{ID: uuidWithByte(3), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), Title: "shot-01 preview"},
			Version: db.ArtifactVersion{ID: uuidWithByte(4), WorkspaceID: uuidWithByte(1), NodeID: uuidWithByte(3), VersionNo: 1, Status: db.JobStatusSucceeded},
		}},
		Responder:       fakeReviewResponder{result: failingReviewResult()},
		Runtime:         runtime,
		Store:           store,
		RetryDispatcher: retryDispatcher,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision.Status != ReviewStatusRejected || !out.Decision.ShouldRetry {
		t.Fatalf("decision = %#v", out.Decision)
	}
	if !retryDispatcher.called {
		t.Fatal("retry dispatcher was not called")
	}
	if retryDispatcher.input.ShotRef != "shot-01" || retryDispatcher.input.AttemptNo != 2 || retryDispatcher.input.MaxAttempts != 3 {
		t.Fatalf("retry input = %#v", retryDispatcher.input)
	}
	if !containsString(runtime.eventTypes, "retry_requested") || !containsString(runtime.eventTypes, "review_rejected") {
		t.Fatalf("events = %#v", runtime.eventTypes)
	}
}

type fakeReviewLoader struct {
	context Context
}

func (f fakeReviewLoader) Load(context.Context, GraphInput) (Context, error) {
	return f.context, nil
}

type fakeReviewResponder struct {
	result ReviewResult
}

func (f fakeReviewResponder) Review(context.Context, Context) (ReviewResult, map[string]any, error) {
	return f.result, map[string]any{"provider": "test", "model_id": "reviewer-test"}, nil
}

type fakeReviewStore struct {
	completedStatus string
}

func (f *fakeReviewStore) CreateReviewRecord(_ context.Context, params db.CreateReviewRecordParams) (db.ReviewRecord, error) {
	return db.ReviewRecord{
		ID:                uuidWithByte(10),
		WorkspaceID:       params.WorkspaceID,
		ShotID:            params.ShotID,
		NodeID:            params.NodeID,
		ArtifactVersionID: params.ArtifactVersionID,
		ReviewerThreadID:  params.ReviewerThreadID,
		ReviewerTaskID:    params.ReviewerTaskID,
		TargetPhase:       params.TargetPhase,
		Status:            ReviewStatusRunning,
		AttemptNo:         params.AttemptNo,
		MaxAttempts:       params.MaxAttempts,
		ModelProvider:     params.ModelProvider,
		ModelID:           params.ModelID,
	}, nil
}

func (f *fakeReviewStore) CompleteReviewRecord(_ context.Context, params db.CompleteReviewRecordParams) (db.ReviewRecord, error) {
	f.completedStatus = params.Status
	return db.ReviewRecord{
		ID:                  params.ID,
		Status:              params.Status,
		OverallScore:        params.OverallScore,
		Rubric:              params.Rubric,
		Critique:            params.Critique,
		RetryRecommendation: params.RetryRecommendation,
	}, nil
}

func (f *fakeReviewStore) FailReviewRecord(context.Context, db.FailReviewRecordParams) (db.ReviewRecord, error) {
	return db.ReviewRecord{}, nil
}

type fakeVersionSelector struct {
	called    bool
	nodeID    pgtype.UUID
	versionID pgtype.UUID
}

type fakeRetryDispatcher struct {
	called bool
	input  RetryDispatchInput
}

type fakeDependencyNotifier struct {
	called bool
	shotID pgtype.UUID
	phase  string
}

func (f *fakeDependencyNotifier) NotifyShotUpdated(_ context.Context, _ pgtype.UUID, shotID pgtype.UUID, phase string) error {
	f.called = true
	f.shotID = shotID
	f.phase = phase
	return nil
}

func (f *fakeRetryDispatcher) DispatchRetry(_ context.Context, input RetryDispatchInput) error {
	f.called = true
	f.input = input
	return nil
}

func (f *fakeVersionSelector) SelectArtifactVersion(_ context.Context, nodeID, versionID pgtype.UUID) (production.ArtifactSelectionResult, error) {
	f.called = true
	f.nodeID = nodeID
	f.versionID = versionID
	return production.ArtifactSelectionResult{}, nil
}

type fakeReviewerRuntime struct {
	appendedContent []byte
	eventTypes      []string
}

func (f *fakeReviewerRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.appendedContent = params.Content
	return db.AgentMessage{ID: uuidWithByte(11), Content: params.Content}, nil
}

func (f *fakeReviewerRuntime) UpsertCheckpoint(context.Context, agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error) {
	return db.EinoCheckpoint{}, nil
}

func (f *fakeReviewerRuntime) SetThreadCheckpoint(context.Context, pgtype.UUID, string) (db.AgentThread, error) {
	return db.AgentThread{}, nil
}

func (f *fakeReviewerRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.eventTypes = append(f.eventTypes, params.EventType)
	return db.AgentEvent{ID: uuidWithByte(byte(len(f.eventTypes) + 20)), EventType: params.EventType}, nil
}

func failingReviewResult() ReviewResult {
	result := passingReviewResult()
	result.Verdict = ReviewStatusRejected
	result.OverallScore = 0.42
	result.Critique = "主体不清晰，需要重新生成。"
	result.Rubric[AxisCompositionProportion] = RubricAxis{Score: 0.35, Pass: false, Reason: "构图偏离", FixHint: "拉近产品特写"}
	result.Issues = []ReviewIssue{{
		Dimension:        AxisCompositionProportion,
		Severity:         IssueSeverityBlocking,
		Title:            "构图偏离",
		Description:      "商品主体不清晰，不能进入下一阶段。",
		TargetObjectType: "artifact_version",
		TargetObjectID:   uuidString(uuidWithByte(4)),
		SuggestedFix:     "regenerate",
		FixHint:          "拉近产品特写",
	}}
	result.RetryRecommendation = RetryRecommendation{ShouldRetry: true, FixHints: []string{"拉近产品特写"}}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
