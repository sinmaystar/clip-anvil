package producer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
)

func TestEinoToolInfoUsesRegistryDefinition(t *testing.T) {
	def := agenttools.Definition{
		Name:        "update_storyboard",
		Description: "Update storyboard facts.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"intent": map[string]any{"type": "string"},
			},
		},
		Result: map[string]any{"type": "object"},
		Safety: agenttools.SafetySpec{MaxCallsPerTurn: 5},
		Visibility: agenttools.VisibilitySpec{
			UserLabel: "更新分镜",
		},
	}

	info, err := producerToolInfo(def)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "update_storyboard" || info.Desc != "Update storyboard facts." {
		t.Fatalf("tool info = %#v", info)
	}
	if info.ParamsOneOf == nil {
		t.Fatal("ParamsOneOf is nil")
	}
	if info.Extra["user_label"] != "更新分镜" || info.Extra["max_calls_per_turn"] != 5 {
		t.Fatalf("extra = %#v", info.Extra)
	}
}

func TestEinoToolsNodeInvokesProducerToolExecutor(t *testing.T) {
	registry, err := agenttools.NewRegistry(adapterDefinitionTool{name: "update_storyboard"})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingNativeToolExecutor{}
	node, runState, err := newEinoProducerToolNode(context.Background(), ProducerContext{}, registry, executor)
	if err != nil {
		t.Fatal(err)
	}

	out, err := node.Invoke(context.Background(), &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{
				ID:   "call-storyboard",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "update_storyboard",
					Arguments: `{"intent":"replace"}`,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if executor.calledName != "update_storyboard" {
		t.Fatalf("called name = %q", executor.calledName)
	}
	if executor.calledID != "call-storyboard" {
		t.Fatalf("called id = %q", executor.calledID)
	}
	if len(out) != 1 || out[0].Role != schema.Tool || out[0].ToolCallID != "call-storyboard" {
		t.Fatalf("tool output = %#v", out)
	}
	if !strings.Contains(out[0].Content, `"ok":true`) {
		t.Fatalf("tool output content = %q", out[0].Content)
	}
	result, ok := runState.result("call-storyboard")
	if !ok || result.ToolName != "update_storyboard" {
		t.Fatalf("run state result = %#v, ok=%v", result, ok)
	}
}

func TestEinoToolsNodeInterruptsForDecisionTool(t *testing.T) {
	registry, err := agenttools.NewRegistry(adapterDefinitionTool{name: "request_user_decision"})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingNativeToolExecutor{interrupted: true}
	node, _, err := newEinoProducerToolNode(context.Background(), ProducerContext{}, registry, executor)
	if err != nil {
		t.Fatal(err)
	}

	_, err = node.Invoke(context.Background(), &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{
				ID:   "call-decision",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "request_user_decision",
					Arguments: `{"title":"确认","message":"继续吗"}`,
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected interrupt error")
	}
	if !strings.Contains(err.Error(), "interrupt signal") {
		t.Fatalf("error is not an Eino interrupt signal: %v", err)
	}
}

type adapterDefinitionTool struct {
	name string
}

func (t adapterDefinitionTool) Definition() agenttools.Definition {
	return agenttools.Definition{
		Name:        t.name,
		Description: "Adapter test tool.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"intent": map[string]any{"type": "string"},
			},
		},
		Result: map[string]any{"type": "object"},
		Visibility: agenttools.VisibilitySpec{
			UserLabel: "测试工具",
		},
	}
}

func (t adapterDefinitionTool) Execute(context.Context, agenttools.ExecuteInput) (agenttools.ExecuteOutput, error) {
	return agenttools.ExecuteOutput{}, nil
}

type recordingNativeToolExecutor struct {
	calledName  string
	calledID    string
	calledArgs  map[string]any
	summary     string
	interrupted bool
}

func (e *recordingNativeToolExecutor) ExecuteProducerTool(_ context.Context, _ ProducerContext, call ToolCall) (ToolExecutionResult, error) {
	e.calledName = call.Name
	e.calledID = call.ID
	e.calledArgs = call.Arguments
	return ToolExecutionResult{
		Result:      map[string]any{"ok": true},
		Summary:     e.summary,
		Interrupted: e.interrupted,
		ToolCallID:  call.ID,
		ToolName:    call.Name,
	}, nil
}

func TestEinoToolAdapterInvokableRunDirectly(t *testing.T) {
	registry, err := agenttools.NewRegistry(adapterDefinitionTool{name: "update_storyboard"})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingNativeToolExecutor{}
	tools, _, err := newEinoProducerTools(context.Background(), ProducerContext{}, registry, executor)
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := tools[0].(*einoProducerTool)
	if !ok {
		t.Fatalf("tool type = %T", tools[0])
	}
	got, err := tool.InvokableRun(context.Background(), `{"intent":"replace"}`)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["ok"] != true {
		t.Fatalf("decoded = %#v", decoded)
	}
	if executor.calledName != "update_storyboard" || executor.calledID == "" {
		t.Fatalf("called name/id = %q/%q", executor.calledName, executor.calledID)
	}
}

func TestEinoToolAdapterReturnsSummaryToModelWhenPresent(t *testing.T) {
	registry, err := agenttools.NewRegistry(adapterDefinitionTool{name: "dispatch_craftsman"})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingNativeToolExecutor{summary: "预览图生成任务已加入队列，后续通过画布同步更新。"}
	tools, _, err := newEinoProducerTools(context.Background(), ProducerContext{}, registry, executor)
	if err != nil {
		t.Fatal(err)
	}
	tool := tools[0].(*einoProducerTool)

	got, err := tool.InvokableRun(context.Background(), `{"mode":"preview_image"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "预览图生成任务已加入队列，后续通过画布同步更新。" {
		t.Fatalf("tool output = %q", got)
	}
}
