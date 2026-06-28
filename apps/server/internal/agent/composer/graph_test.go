package composer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestComposerGraphRequiresNativeToolLoopDependencies(t *testing.T) {
	_, err := NewGraph(GraphConfig{
		Runtime:    &fakeComposerRuntime{},
		Store:      &fakeComposerStore{},
		Production: &fakeComposerProduction{result: production.RunResult{Node: db.MediaNode{ID: uuidWithByte(50)}, Job: db.GenerationJob{ID: uuidWithByte(60)}, Version: db.ArtifactVersion{ID: uuidWithByte(70)}}},
	})
	if err == nil {
		t.Fatal("expected missing native Composer tool-loop dependencies to fail")
	}
}

func TestComposerGraphUsesNativeToolLoop(t *testing.T) {
	registry := agenteino.NewGraphInfoRegistry()
	nativeTools := fakeComposerNativeRegistry(t, &runtimeCaptureTool{})
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeComposerLoader{context: Context{WorkspaceID: uuidWithByte(1), SourceStoryboardNodeID: uuidWithByte(9), Summary: "context"}},
		Runtime:            &fakeComposerRuntime{},
		Store:              &fakeComposerStore{},
		Production:         &fakeComposerProduction{result: production.RunResult{Node: db.MediaNode{ID: uuidWithByte(50)}, Job: db.GenerationJob{ID: uuidWithByte(60)}, Version: db.ArtifactVersion{ID: uuidWithByte(70)}}},
		ToolResponder:      fakeComposerToolResponder{},
		NativeToolRegistry: nativeTools,
		CompileCallbacks: []compose.GraphCompileCallback{
			registry.CompileCallback(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = graph
	info, ok := registry.Get("composer_timeline")
	if !ok {
		t.Fatal("composer_timeline graph info was not captured")
	}
	for _, want := range []string{"load_context", "prepare_turn_state", "before_model", "call_model", "prepare_tool_message", "execute_tools", "append_tool_results", "finalize_response"} {
		if _, ok := info.Nodes[want]; !ok {
			t.Fatalf("composer graph missing node %q", want)
		}
	}
}

func TestComposerNativeToolLoopInjectsRuntime(t *testing.T) {
	capture := &runtimeCaptureTool{}
	nativeTools := fakeComposerNativeRegistry(t, capture)
	graph, err := NewGraph(GraphConfig{
		Loader: fakeComposerLoader{context: Context{
			WorkspaceID:            uuidWithByte(1),
			SourceStoryboardNodeID: uuidWithByte(9),
			Summary:                "context",
		}},
		Runtime:            &fakeComposerRuntime{},
		Store:              &fakeComposerStore{},
		Production:         &fakeComposerProduction{},
		ToolResponder:      scriptedComposerResponder{},
		NativeToolRegistry: nativeTools,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := graph.Run(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Input:       CompositionInput{SourceStoryboardNodeID: uuidString(uuidWithByte(9)), Instructions: "render final"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if capture.runtime.WorkspaceID != uuidWithByte(1) || capture.runtime.ThreadID != uuidWithByte(2) || capture.runtime.TaskID != uuidWithByte(3) {
		t.Fatalf("runtime = %#v", capture.runtime)
	}
	if capture.runtime.ScopeType != "final_output" || capture.runtime.ScopeID != uuidWithByte(9) {
		t.Fatalf("scope runtime = %#v", capture.runtime)
	}
	if out.Output.Status != "completed" || out.Output.TimelinePlanID != "timeline-1" {
		t.Fatalf("output = %#v", out.Output)
	}
	if len(out.Output.SameTurnMessages) != 2 {
		t.Fatalf("same-turn messages = %#v", out.Output.SameTurnMessages)
	}
}

func TestComposerNativeToolLoopEmitsTraceSinkEvents(t *testing.T) {
	capture := &runtimeCaptureTool{}
	traceSink := &fakeComposerTraceSink{}
	nativeTools := fakeComposerNativeRegistry(t, capture)
	graph, err := NewGraph(GraphConfig{
		Loader: fakeComposerLoader{context: Context{
			WorkspaceID:            uuidWithByte(1),
			SourceStoryboardNodeID: uuidWithByte(9),
			Summary:                "context",
		}},
		Runtime:            &fakeComposerRuntime{},
		Store:              &fakeComposerStore{},
		Production:         &fakeComposerProduction{},
		ToolResponder:      scriptedComposerResponder{},
		NativeToolRegistry: nativeTools,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := agenttools.WithNativeToolTraceSink(context.Background(), traceSink)
	if _, err := graph.Run(ctx, GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Input:       CompositionInput{SourceStoryboardNodeID: uuidString(uuidWithByte(9)), Instructions: "render final"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(traceSink.started) != 1 || len(traceSink.completed) != 1 {
		t.Fatalf("trace events started=%#v completed=%#v", traceSink.started, traceSink.completed)
	}
	if traceSink.started[0].runtime.ThreadID != uuidWithByte(2) || traceSink.started[0].trace.ToolName != "capture_runtime" {
		t.Fatalf("started trace = %#v", traceSink.started[0])
	}
	if !strings.Contains(traceSink.completed[0].trace.Result, `"ok":"true"`) {
		t.Fatalf("completed trace = %#v", traceSink.completed[0])
	}
}

func TestFinalizeComposerOutputUsesSubmitArtifactToolResult(t *testing.T) {
	out := finalizeComposerOutput(composerLoopState{
		Context: Context{SameTurnMessages: []ComposerSameTurnMessage{
			{
				Role:        "tool",
				MessageType: "tool_result",
				ToolName:    "submit_composition_artifact",
				Content:     `{"output_node_id":"node-1","generation_job_id":"job-1","artifact_version_id":"version-1","timeline_plan_id":"timeline-1","sandbox_job_id":"sandbox-1"}`,
			},
		}},
		LastOutput: ComposerTurnOutput{Result: CompositionOutput{OperationType: "compose_final_video"}},
	})
	if out.Status != "completed" || out.NodeID != "node-1" || out.GenerationJobID != "job-1" || out.ArtifactVersionID != "version-1" || out.TimelinePlanID != "timeline-1" || out.SandboxJobID != "sandbox-1" {
		t.Fatalf("output = %#v", out)
	}
}

type fakeComposerToolResponder struct{}

func (fakeComposerToolResponder) Respond(context.Context, Context) (ComposerTurnOutput, error) {
	return ComposerTurnOutput{
		AssistantText: "Composer tool loop finished.",
		ModelMessage:  &schema.Message{Role: schema.Assistant, Content: "Composer tool loop finished."},
	}, nil
}

type scriptedComposerResponder struct{}

func (scriptedComposerResponder) Respond(_ context.Context, context Context) (ComposerTurnOutput, error) {
	if len(context.SameTurnMessages) == 0 {
		return ComposerTurnOutput{
			AssistantText: "Calling Composer tool.",
			ModelMessage: &schema.Message{Role: schema.Assistant, Content: "Calling Composer tool.", ToolCalls: []schema.ToolCall{{
				ID:   "call-runtime",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "capture_runtime",
					Arguments: `{}`,
				},
			}}},
		}, nil
	}
	return ComposerTurnOutput{
		AssistantText: "Composer completed.",
		Result: CompositionOutput{
			Status:         "completed",
			TimelinePlanID: "timeline-1",
			NodeID:         "node-1",
			OperationType:  "compose_final_video",
		},
		ModelMessage: &schema.Message{Role: schema.Assistant, Content: "Composer completed."},
	}, nil
}

type runtimeCaptureTool struct {
	runtime agenttools.NativeRuntimeContext
}

func (t *runtimeCaptureTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "capture_runtime", Desc: "capture Composer native runtime"}, nil
}

func (t *runtimeCaptureTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	runtime, ok := agenttools.NativeRuntimeFromContext(ctx)
	if !ok {
		return "", nil
	}
	t.runtime = runtime
	result := map[string]string{"ok": "true"}
	encoded, _ := json.Marshal(result)
	return string(encoded), nil
}

type fakeComposerTraceSink struct {
	started   []fakeComposerTraceEvent
	completed []fakeComposerTraceEvent
}

type fakeComposerTraceEvent struct {
	runtime agenttools.NativeRuntimeContext
	trace   agenttools.NativeToolTrace
}

func (f *fakeComposerTraceSink) NativeToolCallStarted(_ context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
	f.started = append(f.started, fakeComposerTraceEvent{runtime: runtime, trace: trace})
	return nil
}

func (f *fakeComposerTraceSink) NativeToolCallCompleted(_ context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
	f.completed = append(f.completed, fakeComposerTraceEvent{runtime: runtime, trace: trace})
	return nil
}

type fakeComposerLoader struct {
	context Context
}

func (f fakeComposerLoader) LoadCompositionContext(context.Context, Request) (Context, error) {
	return f.context, nil
}

func fakeComposerNativeRegistry(t *testing.T, tools ...agenttools.NativeTool) *agenttools.NativeRegistry {
	t.Helper()
	registry, err := agenttools.NewNativeRegistry(tools...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

var _ = pgtype.UUID{}
