package reviewer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestReviewerExecutorRunsReviewerTurnTask(t *testing.T) {
	runtime := &fakeReviewerExecutorRuntime{}
	graph := &fakeReviewerRunner{output: GraphOutput{Decision: ReviewDecision{Status: ReviewStatusAccepted}}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})
	input := TaskInput{
		TargetPhase:       TargetPhasePreviewImage,
		ShotID:            uuidString(uuidWithByte(2)),
		NodeID:            uuidString(uuidWithByte(3)),
		ArtifactVersionID: uuidString(uuidWithByte(4)),
		AttemptNo:         1,
		MaxAttempts:       3,
	}
	raw, _ := json.Marshal(input)
	task := db.AgentTask{
		ID:          uuidWithByte(9),
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		Role:        "reviewer",
		TaskType:    "reviewer_turn",
		Input:       raw,
	}

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if !runtime.running || !runtime.succeeded {
		t.Fatalf("runtime running=%v succeeded=%v", runtime.running, runtime.succeeded)
	}
	if graph.input.Task.NodeID != input.NodeID {
		t.Fatalf("graph input = %#v", graph.input)
	}
	wantCheckpoint := "agent:eino:reviewer_preview:01000000-0000-0000-0000-000000000000:08000000-0000-0000-0000-000000000000:09000000-0000-0000-0000-000000000000"
	if graph.runOptions.CheckPointID != wantCheckpoint {
		t.Fatalf("checkpoint id = %q, want %q", graph.runOptions.CheckPointID, wantCheckpoint)
	}
	if runtime.threadCheckpoint != wantCheckpoint {
		t.Fatalf("thread checkpoint = %q, want %q", runtime.threadCheckpoint, wantCheckpoint)
	}
}

type fakeReviewerExecutorRuntime struct {
	running          bool
	succeeded        bool
	failed           bool
	threadCheckpoint string
}

func (f *fakeReviewerExecutorRuntime) MarkTaskRunning(context.Context, pgtype.UUID) (db.AgentTask, error) {
	f.running = true
	return db.AgentTask{}, nil
}

func (f *fakeReviewerExecutorRuntime) MarkTaskSucceeded(context.Context, pgtype.UUID, []byte) (db.AgentTask, error) {
	f.succeeded = true
	return db.AgentTask{}, nil
}

func (f *fakeReviewerExecutorRuntime) MarkTaskFailed(context.Context, pgtype.UUID, string, string) (db.AgentTask, error) {
	f.failed = true
	return db.AgentTask{}, nil
}

func (f *fakeReviewerExecutorRuntime) SetThreadCheckpoint(_ context.Context, _ pgtype.UUID, checkpointKey string) (db.AgentThread, error) {
	f.threadCheckpoint = checkpointKey
	return db.AgentThread{CurrentCheckpointKey: pgtype.Text{String: checkpointKey, Valid: checkpointKey != ""}}, nil
}

type fakeReviewerRunner struct {
	input      GraphInput
	output     GraphOutput
	runOptions agenteino.RunOptions
}

func (f *fakeReviewerRunner) Run(_ context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error) {
	f.input = input
	if len(options) > 0 {
		f.runOptions = options[0]
	}
	return f.output, nil
}
