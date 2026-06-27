package reviewer

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/compose"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
)

func TestReviewerGraphUsesNativeToolLoop(t *testing.T) {
	registry := agenteino.NewGraphInfoRegistry()
	nativeTools, err := agenttools.NewNativeRegistry(agenttools.NewSubmitReviewResultNativeTool(nil))
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeReviewLoader{context: Context{Input: GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3)}}},
		ToolResponder:      fakeReviewerToolResponder{},
		NativeToolRegistry: nativeTools,
		CompileCallbacks: []compose.GraphCompileCallback{
			registry.CompileCallback(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = graph
	info, ok := registry.Get("reviewer_gate")
	if !ok {
		t.Fatal("reviewer_gate graph info was not captured")
	}
	for _, want := range []string{"load_context", "prepare_turn_state", "before_model", "call_model", "prepare_tool_message", "execute_tools", "append_tool_results", "finalize_response"} {
		if _, ok := info.Nodes[want]; !ok {
			t.Fatalf("reviewer graph missing node %q", want)
		}
	}
}

func TestReviewerDefaultMaxToolCallsIsLargeDuringArchitectureIteration(t *testing.T) {
	if got := maxReviewerToolCalls(); got != 1000 {
		t.Fatalf("maxReviewerToolCalls() = %d, want 1000", got)
	}
}

func TestReviewerNativeToolMiddlewareInjectsReviewTargetRuntime(t *testing.T) {
	stateStore := newReviewerLoopToolStateStore()
	stateKey := "state-1"
	state := reviewerLoopState{Context: Context{Input: GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Task: TaskInput{
			ReviewTask: ReviewTaskPreviewImage,
			Target: ReviewTarget{
				ShotID:            uuidString(uuidWithByte(4)),
				NodeID:            uuidString(uuidWithByte(5)),
				ArtifactVersionID: uuidString(uuidWithByte(6)),
				GenerationJobID:   uuidString(uuidWithByte(7)),
				RenderPlanID:      uuidString(uuidWithByte(8)),
			},
		},
	}}}
	stateStore.rememberKey(stateKey, state)
	middleware := nativeReviewerToolRuntimeMiddleware(stateStore)
	var got agenttools.NativeRuntimeContext
	endpoint := middleware.Invokable(func(ctx context.Context, _ *compose.ToolInput) (*compose.ToolOutput, error) {
		var ok bool
		got, ok = agenttools.NativeRuntimeFromContext(ctx)
		if !ok {
			t.Fatal("missing native runtime")
		}
		return &compose.ToolOutput{Result: "ok"}, nil
	})
	if _, err := endpoint(context.Background(), &compose.ToolInput{
		Name:      "submit_review_result",
		CallID:    "call-1",
		Arguments: `{"_clipanvil_reviewer_loop_state_key":"state-1"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if got.ReviewVersionID != uuidString(uuidWithByte(6)) || got.ReviewNodeID != uuidString(uuidWithByte(5)) || got.ReviewRenderPlanID != uuidString(uuidWithByte(8)) {
		t.Fatalf("review runtime target = %#v", got)
	}
}

type fakeReviewerToolResponder struct{}

func (fakeReviewerToolResponder) Respond(context.Context, Context) (ReviewerTurnOutput, error) {
	return ReviewerTurnOutput{AssistantText: "ok"}, nil
}

type fakeReviewLoader struct {
	context Context
}

func (f fakeReviewLoader) Load(context.Context, GraphInput) (Context, error) {
	return f.context, nil
}
