package producer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestDeterministicResponderUsesLatestUserText(t *testing.T) {
	responder := DeterministicResponder{}

	out, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "做一个咖啡广告",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.AssistantText, "做一个咖啡广告") {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if !strings.Contains(out.AssistantText, "后续阶段拆成分镜和生产任务") {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
}

func TestGraphRunReturnsAssistantText(t *testing.T) {
	graph, err := NewGraph(GraphConfig{
		Loader: fakeContextLoader{
			context: ProducerContext{LatestUserText: "一条运动鞋短片"},
		},
		Responder: DeterministicResponder{},
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.AssistantText, "一条运动鞋短片") {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
}

func TestProducerGraphCompileCapturesGraphInfo(t *testing.T) {
	registry := agenteino.NewGraphInfoRegistry()
	_, err := NewGraph(GraphConfig{
		Loader:           fakeContextLoader{context: ProducerContext{LatestUserText: "brief"}},
		Responder:        DeterministicResponder{},
		CompileCallbacks: []compose.GraphCompileCallback{registry.CompileCallback()},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, ok := registry.Get("producer_turn")
	if !ok {
		t.Fatal("producer graph info was not captured")
	}
	for _, node := range []string{"load_context", "draft_response", "finalize_response"} {
		if _, ok := info.Nodes[node]; !ok {
			t.Fatalf("node %q missing from graph info", node)
		}
	}
}

func TestProducerGraphExplicitToolLoopCapturesGraphInfo(t *testing.T) {
	registry := agenteino.NewGraphInfoRegistry()
	_, err := NewGraph(GraphConfig{
		Mode:               ProducerGraphModeExplicitToolLoop,
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "brief"}},
		Responder:          DeterministicResponder{},
		NativeToolRegistry: mustTestNativeToolRegistry(t, "create_agent_text_node"),
		CompileCallbacks:   []compose.GraphCompileCallback{registry.CompileCallback()},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, ok := registry.Get("producer_turn")
	if !ok {
		t.Fatal("producer graph info was not captured")
	}
	for _, node := range []string{"load_context", "prepare_turn_state", "call_model", "prepare_tool_message", "execute_tools", "append_tool_results", "finalize_response"} {
		if _, ok := info.Nodes[node]; !ok {
			t.Fatalf("node %q missing from graph info", node)
		}
	}
	for _, node := range []string{"execute_legacy_tool", "append_legacy_tool_result"} {
		if _, ok := info.Nodes[node]; ok {
			t.Fatalf("legacy node %q must not exist in producer graph info", node)
		}
	}
	if !graphInfoHasBranchTarget(info.Branches, "call_model", "prepare_tool_message") {
		t.Fatalf("branch call_model -> prepare_tool_message missing from graph info: %#v", info.Branches)
	}
	if graphInfoHasBranchTarget(info.Branches, "call_model", "execute_legacy_tool") {
		t.Fatalf("legacy branch call_model -> execute_legacy_tool must not exist: %#v", info.Branches)
	}
	if !graphInfoHasEdge(info.Edges, "prepare_tool_message", "execute_tools") {
		t.Fatalf("edge prepare_tool_message -> execute_tools missing from graph info: %#v", info.Edges)
	}
	if !graphInfoHasEdge(info.Edges, "execute_tools", "append_tool_results") {
		t.Fatalf("edge execute_tools -> append_tool_results missing from graph info: %#v", info.Edges)
	}
	if !graphInfoHasEdge(info.Edges, "append_tool_results", "call_model") {
		t.Fatalf("edge append_tool_results -> call_model missing from graph info: %#v", info.Edges)
	}
}

func TestProducerGraphExplicitToolLoopExecutesToolWithEinoToolNode(t *testing.T) {
	registry := mustTestNativeToolRegistry(t, "create_agent_text_node")
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-text", "create_agent_text_node", `{"title":"brief","text":"hello"}`),
		{AssistantText: "已保存 brief。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Mode:               ProducerGraphModeExplicitToolLoop,
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "保存 brief"}},
		Responder:          responder,
		NativeToolRegistry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 50})
	if err != nil {
		t.Fatal(err)
	}

	if out.AssistantText != "已保存 brief。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if len(responder.contexts) != 2 {
		t.Fatalf("contexts len = %d, want 2", len(responder.contexts))
	}
	sameTurn := responder.contexts[1].SameTurnMessages
	if len(sameTurn) != 2 {
		t.Fatalf("same-turn messages = %#v", sameTurn)
	}
	if sameTurn[0].Role != "assistant" || sameTurn[0].ToolName != "create_agent_text_node" || sameTurn[0].ToolCallID == "" {
		t.Fatalf("same-turn assistant = %#v", sameTurn[0])
	}
	if sameTurn[1].Role != "tool" || sameTurn[1].ToolCallID != sameTurn[0].ToolCallID || !strings.Contains(sameTurn[1].Content, `"ok":true`) {
		t.Fatalf("same-turn tool result = %#v", sameTurn[1])
	}
}

func TestProducerGraphExplicitToolLoopReturnsSameTurnToolTrace(t *testing.T) {
	registry := mustTestNativeToolRegistry(t, "create_agent_text_node")
	graph, err := NewGraph(GraphConfig{
		Mode:   ProducerGraphModeExplicitToolLoop,
		Loader: fakeContextLoader{context: ProducerContext{LatestUserText: "保存 brief"}},
		Responder: &sequenceResponder{outputs: []ProducerTurnOutput{
			nativeToolCallOutput("call-text", "create_agent_text_node", `{"title":"brief","text":"hello"}`),
			{AssistantText: "已保存 brief。"},
		}},
		NativeToolRegistry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 50})
	if err != nil {
		t.Fatal(err)
	}

	if len(out.SameTurnMessages) != 2 {
		t.Fatalf("same-turn trace = %#v", out.SameTurnMessages)
	}
	if out.SameTurnMessages[0].MessageType != "tool_call" || out.SameTurnMessages[0].ToolName != "create_agent_text_node" {
		t.Fatalf("tool call trace = %#v", out.SameTurnMessages[0])
	}
	if out.SameTurnMessages[1].MessageType != "tool_result" || out.SameTurnMessages[1].ToolCallID != out.SameTurnMessages[0].ToolCallID {
		t.Fatalf("tool result trace = %#v", out.SameTurnMessages[1])
	}
}

func TestProducerGraphExplicitToolLoopAllowsMultipleToolIterations(t *testing.T) {
	registry := mustTestNativeToolRegistry(t, "create_agent_text_node")
	responder := &sequenceResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-a", "create_agent_text_node", `{"title":"a","text":"a"}`),
		nativeToolCallOutput("call-b", "create_agent_text_node", `{"title":"b","text":"b"}`),
		nativeToolCallOutput("call-c", "create_agent_text_node", `{"title":"c","text":"c"}`),
		nativeToolCallOutput("call-d", "create_agent_text_node", `{"title":"d","text":"d"}`),
		{AssistantText: "四个工具调用已完成。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Mode:               ProducerGraphModeExplicitToolLoop,
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "多步写入"}},
		Responder:          responder,
		NativeToolRegistry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 50})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "四个工具调用已完成。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
}

func TestProducerGraphExplicitToolLoopNativeDecisionInterruptResumes(t *testing.T) {
	registry := mustTestNativeToolRegistryWithTools(t, &testNativeTool{name: "request_user_decision", interrupt: true})
	responder := &sequenceResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-decision", "request_user_decision", `{"title":"确认","message":"继续吗"}`),
		{AssistantText: "已根据你的选择继续。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Mode:               ProducerGraphModeExplicitToolLoop,
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "需要决策"}},
		Responder:          responder,
		NativeToolRegistry: registry,
		CheckPointStore:    newMemoryCheckpointStore(),
	})
	if err != nil {
		t.Fatal(err)
	}
	checkpointKey := agenteino.CheckpointKey("producer_turn", uuidWithByte(1), uuidWithByte(2), uuidWithByte(3))
	input := ProducerTurnInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3), MaxToolCalls: 50}

	_, err = graph.Run(context.Background(), input, agenteino.RunOptions{CheckPointID: checkpointKey})
	if err == nil {
		t.Fatal("expected graph interrupt")
	}
	interruptInfo, ok := compose.ExtractInterruptInfo(err)
	if !ok || len(interruptInfo.InterruptContexts) == 0 {
		t.Fatalf("interrupt info = %#v err=%v", interruptInfo, err)
	}

	out, err := graph.Run(context.Background(), input, agenteino.RunOptions{
		CheckPointID: checkpointKey,
		ResumeData: map[string]any{
			interruptInfo.InterruptContexts[0].ID: map[string]any{"selected_option_id": "continue"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "已根据你的选择继续。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
}

func TestProducerGraphExecutesCreateAgentTextNodeTool(t *testing.T) {
	tool := &testNativeTool{name: "create_agent_text_node"}
	registry := mustTestNativeToolRegistryWithTools(t, tool)
	graph, err := NewGraph(GraphConfig{
		Mode: ProducerGraphModeExplicitToolLoop,
		Loader: fakeContextLoader{
			context: ProducerContext{LatestUserText: "保存 brief"},
		},
		Responder: &sequenceResponder{outputs: []ProducerTurnOutput{
			nativeToolCallOutput("call-text", "create_agent_text_node", `{"title":"brief","text":"hello"}`),
			{AssistantText: "已保存 brief。"},
		}},
		NativeToolRegistry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 50})
	if err != nil {
		t.Fatal(err)
	}

	if out.AssistantText != "已保存 brief。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if tool.calledName != "create_agent_text_node" {
		t.Fatalf("called tool = %q", tool.calledName)
	}
}

func TestProducerGraphExecutesUpdateStoryboardTool(t *testing.T) {
	tool := &testNativeTool{name: "update_storyboard"}
	registry := mustTestNativeToolRegistryWithTools(t, tool)
	graph, err := NewGraph(GraphConfig{
		Mode: ProducerGraphModeExplicitToolLoop,
		Loader: fakeContextLoader{
			context: ProducerContext{LatestUserText: "拆成两个分镜"},
		},
		Responder: &sequenceResponder{outputs: []ProducerTurnOutput{
			nativeToolCallOutput("call-storyboard", "update_storyboard", `{"intent":"replace","shots":[{"client_key":"shot-01","sort_order":1,"title":"开场钩子"},{"client_key":"shot-02","sort_order":2,"title":"卖点证明"}],"dependencies":[{"from":"shot-01","to":"shot-02","dependency_type":"story_order"}]}`),
			{AssistantText: "已更新 storyboard。"},
		}},
		NativeToolRegistry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 50})
	if err != nil {
		t.Fatal(err)
	}

	if out.AssistantText != "已更新 storyboard。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if tool.calledName != "update_storyboard" {
		t.Fatalf("called tool = %q", tool.calledName)
	}
}

func TestProducerGraphNativeDecisionInterruptResumes(t *testing.T) {
	registry := mustTestNativeToolRegistryWithTools(t, &testNativeTool{name: "request_user_decision", interrupt: true})
	responder := &sequenceResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-decision", "request_user_decision", `{"title":"确认","message":"继续吗"}`),
		{AssistantText: "已根据你的选择继续。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Mode:               ProducerGraphModeExplicitToolLoop,
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "需要决策"}},
		Responder:          responder,
		NativeToolRegistry: registry,
		CheckPointStore:    newMemoryCheckpointStore(),
	})
	if err != nil {
		t.Fatal(err)
	}
	checkpointKey := agenteino.CheckpointKey("producer_turn", uuidWithByte(1), uuidWithByte(2), uuidWithByte(3))
	input := ProducerTurnInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3), MaxToolCalls: 50}

	_, err = graph.Run(context.Background(), input, agenteino.RunOptions{CheckPointID: checkpointKey})
	if err == nil {
		t.Fatal("expected graph interrupt")
	}
	interruptInfo, ok := compose.ExtractInterruptInfo(err)
	if !ok || len(interruptInfo.InterruptContexts) == 0 {
		t.Fatalf("interrupt info = %#v err=%v", interruptInfo, err)
	}

	out, err := graph.Run(context.Background(), input, agenteino.RunOptions{
		CheckPointID: checkpointKey,
		ResumeData: map[string]any{
			interruptInfo.InterruptContexts[0].ID: map[string]any{"selected_option_id": "continue"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "已根据你的选择继续。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
}

func TestProducerGraphCarriesSameTurnReasoningIntoToolResume(t *testing.T) {
	registry := mustTestNativeToolRegistry(t, "create_agent_text_node")
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		{
			ModelMessage: nativeToolCallMessage("call-text", "create_agent_text_node", `{"title":"brief","text":"hello"}`),
			Metadata: map[string]any{
				"reasoning_content": "需要先保存 brief",
			},
		},
		{AssistantText: "已保存 brief。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Mode:               ProducerGraphModeExplicitToolLoop,
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "保存 brief"}},
		Responder:          responder,
		NativeToolRegistry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 50})
	if err != nil {
		t.Fatal(err)
	}

	if len(responder.contexts) != 2 {
		t.Fatalf("contexts len = %d, want 2", len(responder.contexts))
	}
	sameTurn := responder.contexts[1].SameTurnMessages
	if len(sameTurn) != 2 {
		t.Fatalf("same-turn messages = %#v", sameTurn)
	}
	if sameTurn[0].Role != "assistant" || sameTurn[0].ReasoningContent != "需要先保存 brief" || sameTurn[0].ToolCallID == "" {
		t.Fatalf("same-turn assistant = %#v", sameTurn[0])
	}
	if sameTurn[1].Role != "tool" || sameTurn[1].ToolCallID != sameTurn[0].ToolCallID || !strings.Contains(sameTurn[1].Content, `"ok":true`) {
		t.Fatalf("same-turn tool result = %#v", sameTurn[1])
	}
}

func TestProducerGraphStopsAtMaxToolCalls(t *testing.T) {
	registry := mustTestNativeToolRegistry(t, "create_agent_text_node")
	graph, err := NewGraph(GraphConfig{
		Loader: fakeContextLoader{context: ProducerContext{LatestUserText: "loop"}},
		Responder: &sequenceResponder{outputs: []ProducerTurnOutput{
			nativeToolCallOutput("call-a", "create_agent_text_node", `{"title":"a","text":"b"}`),
			nativeToolCallOutput("call-b", "create_agent_text_node", `{"title":"c","text":"d"}`),
		}},
		Mode:               ProducerGraphModeExplicitToolLoop,
		NativeToolRegistry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 1})
	if !strings.Contains(err.Error(), "agent_tool_loop_exhausted") {
		t.Fatalf("error = %v", err)
	}
}

func TestProducerDefaultMaxToolCallsIsLargeDuringArchitectureIteration(t *testing.T) {
	if got := maxProducerToolCalls(0); got != 1000 {
		t.Fatalf("maxProducerToolCalls(0) = %d, want 1000", got)
	}
}

func TestProducerGraphExplainsReasoningOnlyResponse(t *testing.T) {
	graph, err := NewGraph(GraphConfig{
		Loader: fakeContextLoader{context: ProducerContext{LatestUserText: "写脚本"}},
		Responder: &sequenceResponder{outputs: []ProducerTurnOutput{
			{
				AssistantText: "",
				Metadata: map[string]any{
					"reasoning_content": "已经完成分析，但没有生成最终答案。",
					"reasoning_effort":  "high",
				},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.AssistantText, "没有返回可展示的回复") {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if out.Metadata["empty_content_fallback"] != true {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
}

func TestProducerGraphExplicitToolLoopUsesFallbackForEmptyFinalResponse(t *testing.T) {
	graph, err := NewGraph(GraphConfig{
		Mode: ProducerGraphModeExplicitToolLoop,
		Loader: fakeContextLoader{context: ProducerContext{
			LatestUserText: "现在什么进展了",
		}},
		Responder: &sequenceResponder{outputs: []ProducerTurnOutput{
			{
				AssistantText: "",
				Metadata: map[string]any{
					"provider":               "volcengine",
					"finish_reason":          "stop",
					"native_tool_call_count": 0,
				},
			},
		}},
		NativeToolRegistry: mustTestNativeToolRegistry(t, "read_project_context"),
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.AssistantText, "没有收到可展示") {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if out.Metadata["empty_content_fallback"] != true || out.Metadata["empty_content_without_reasoning"] != true {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
}

func TestLatestUserTextFromMessagesUsesLastUserText(t *testing.T) {
	messages := []db.AgentMessage{
		{Role: "user", MessageType: "text", Content: mustUserContent(t, uimessage.UserMessageInput{Text: "first"})},
		{Role: "assistant", MessageType: "text", Content: mustAssistantContent(t, uimessage.AssistantMessageInput{Text: "reply"})},
		{Role: "user", MessageType: "text", Content: mustUserContent(t, uimessage.UserMessageInput{Text: "second"})},
	}

	got := latestUserTextFromMessages(messages)

	if got != "second" {
		t.Fatalf("latest user text = %q, want second", got)
	}
}

func TestAgentMessageTextIncludesAttachmentSummary(t *testing.T) {
	got := agentMessageText(mustUserContent(t, uimessage.UserMessageInput{
		Text: "看这个素材",
		Attachments: []uimessage.Attachment{
			{Kind: "text", Name: "brief.txt"},
			{Kind: "image", Name: "hero.png"},
		},
	}))

	if !strings.Contains(got, "看这个素材") {
		t.Fatalf("message text = %q", got)
	}
	if !strings.Contains(got, "text: brief.txt") || !strings.Contains(got, "image: hero.png") {
		t.Fatalf("message text = %q", got)
	}
}

type fakeContextLoader struct {
	context ProducerContext
	err     error
}

func nativeToolCallOutput(id string, name string, arguments string) ProducerTurnOutput {
	return ProducerTurnOutput{ModelMessage: nativeToolCallMessage(id, name, arguments)}
}

func nativeToolCallMessage(id string, name string, arguments string) *schema.Message {
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{
				ID:   id,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      name,
					Arguments: arguments,
				},
			},
		},
	}
}

func mustTestNativeToolRegistry(t *testing.T, names ...string) *agenttools.NativeRegistry {
	t.Helper()
	tools := make([]agenttools.NativeTool, 0, len(names))
	for _, name := range names {
		tools = append(tools, &testNativeTool{name: name})
	}
	return mustTestNativeToolRegistryWithTools(t, tools...)
}

func mustTestNativeToolRegistryWithTools(t *testing.T, tools ...agenttools.NativeTool) *agenttools.NativeRegistry {
	t.Helper()
	registry, err := agenttools.NewNativeRegistry(tools...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func (f fakeContextLoader) LoadProducerContext(_ context.Context, input ProducerTurnInput) (ProducerContext, error) {
	context := f.context
	context.Input = input
	return context, f.err
}

type sequenceResponder struct {
	outputs []ProducerTurnOutput
	index   int
}

func (s *sequenceResponder) Respond(context.Context, ProducerContext) (ProducerTurnOutput, error) {
	if s.index >= len(s.outputs) {
		return ProducerTurnOutput{AssistantText: "done"}, nil
	}
	out := s.outputs[s.index]
	s.index++
	return out, nil
}

type recordingResponder struct {
	outputs  []ProducerTurnOutput
	index    int
	contexts []ProducerContext
}

func (r *recordingResponder) Respond(_ context.Context, context ProducerContext) (ProducerTurnOutput, error) {
	r.contexts = append(r.contexts, context)
	if r.index >= len(r.outputs) {
		return ProducerTurnOutput{AssistantText: "done"}, nil
	}
	out := r.outputs[r.index]
	r.index++
	return out, nil
}

type testNativeTool struct {
	name       string
	interrupt  bool
	calledName string
	calledID   string
}

func (t *testNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name,
		Desc: "Native test tool.",
	}, nil
}

func (t *testNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	if wasInterrupted, _, state := einotool.GetInterruptState[map[string]any](ctx); wasInterrupted {
		if isResumeTarget, hasData, data := einotool.GetResumeContext[map[string]any](ctx); isResumeTarget {
			if hasData {
				raw, _ := json.Marshal(data)
				return "已收到用户决策：" + string(raw), nil
			}
			return "已收到用户决策。", nil
		}
		return "", einotool.StatefulInterrupt(ctx, map[string]any{"tool_name": t.name}, state)
	}
	runtime, _ := agenttools.NativeRuntimeFromContext(ctx)
	t.calledName = t.name
	t.calledID = runtime.ToolCallID
	if t.interrupt {
		return "", einotool.StatefulInterrupt(ctx, map[string]any{
			"tool_name":    t.name,
			"tool_call_id": runtime.ToolCallID,
		}, map[string]any{
			"tool_name":    t.name,
			"tool_call_id": runtime.ToolCallID,
			"arguments":    argumentsInJSON,
		})
	}
	return `{"ok":true}`, nil
}

type memoryCheckpointStore struct {
	values map[string][]byte
}

func newMemoryCheckpointStore() *memoryCheckpointStore {
	return &memoryCheckpointStore{values: map[string][]byte{}}
}

func (s *memoryCheckpointStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, ok := s.values[key]
	return value, ok, nil
}

func (s *memoryCheckpointStore) Set(_ context.Context, key string, value []byte) error {
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func graphInfoHasEdge(edges map[string][]string, from string, to string) bool {
	for _, value := range edges[from] {
		if value == to {
			return true
		}
	}
	return false
}

func graphInfoHasBranchTarget(branches map[string][]compose.GraphBranch, from string, to string) bool {
	for _, branch := range branches[from] {
		if branch.GetEndNode()[to] {
			return true
		}
	}
	return false
}
