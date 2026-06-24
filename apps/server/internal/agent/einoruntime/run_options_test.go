package einoruntime

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
)

type recordingCallback struct {
	started bool
	ended   bool
}

func (r *recordingCallback) handler() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
			r.started = true
			return ctx
		}).
		OnEndFn(func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
			r.ended = true
			return ctx
		}).
		Build()
}

func TestApplyRunOptionsAddsCheckpointOption(t *testing.T) {
	_, options := ApplyRunOptions(context.Background(), RunOptions{
		CheckPointID: "cp-1",
		ForceNewRun:  true,
	})

	if len(options) != 2 {
		t.Fatalf("options len = %d", len(options))
	}
}

func TestApplyRunOptionsAddsCallbacks(t *testing.T) {
	recorder := &recordingCallback{}
	ctx, options := ApplyRunOptions(context.Background(), RunOptions{
		Callbacks: []callbacks.Handler{recorder.handler()},
	})

	graph := compose.NewGraph[string, string]()
	if err := graph.AddLambdaNode("echo", compose.InvokableLambda(func(_ context.Context, input string) (string, error) {
		return input, nil
	})); err != nil {
		t.Fatalf("AddLambdaNode: %v", err)
	}
	if err := graph.AddEdge(compose.START, "echo"); err != nil {
		t.Fatalf("AddEdge start: %v", err)
	}
	if err := graph.AddEdge("echo", compose.END); err != nil {
		t.Fatalf("AddEdge end: %v", err)
	}
	runnable, err := graph.Compile(context.Background(), compose.WithGraphName("run_options_callbacks"))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, err := runnable.Invoke(ctx, "ok", options...); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !recorder.started || !recorder.ended {
		t.Fatalf("callback started=%v ended=%v", recorder.started, recorder.ended)
	}
}

func TestApplyRunOptionsAppliesBatchResumeData(t *testing.T) {
	resumeData := map[string]any{"decision": "accepted"}
	ctx, _ := ApplyRunOptions(context.Background(), RunOptions{ResumeData: resumeData})

	if ctx == nil {
		t.Fatal("ApplyRunOptions returned nil context")
	}
}

func TestResumeDecisionDataUsesStableKeys(t *testing.T) {
	eventID := uuidWithByte(9)
	got := ResumeDecisionData(eventID, "option-a", "free text")

	if got["decision_event_id"] != "09000000-0000-0000-0000-000000000000" {
		t.Fatalf("decision_event_id = %#v", got["decision_event_id"])
	}
	if got["selected_option_id"] != "option-a" {
		t.Fatalf("selected_option_id = %#v", got["selected_option_id"])
	}
	if got["free_text"] != "free text" {
		t.Fatalf("free_text = %#v", got["free_text"])
	}
}
