package producer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
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
		Responder:          DeterministicResponder{},
		NativeToolRegistry: mustTestNativeToolRegistry(t, "create_agent_text_node"),
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

func TestProducerGraphRequiresNativeToolLoop(t *testing.T) {
	_, err := NewGraph(GraphConfig{
		Loader:    fakeContextLoader{context: ProducerContext{LatestUserText: "brief"}},
		Responder: DeterministicResponder{},
	})
	if !errors.Is(err, ErrInvalidGraphConfig) {
		t.Fatalf("NewGraph without native tool loop error = %v, want ErrInvalidGraphConfig", err)
	}
}

func TestFormatProducerSignalReminderUsesSemanticRefs(t *testing.T) {
	signal := db.ProducerPendingSignal{
		SemanticKey:  "signal.shot_01.preview_image.r1.worker_generation_completed.abc123",
		SignalType:   "worker_generation_completed",
		ScopeType:    "shot",
		RenderPlanID: uuidWithByte(80),
		Payload: []byte(`{
			"target_phase":"preview_image",
			"render_plan_status":"succeeded",
			"scope_key":"shot_01",
			"render_plan_key":"shot_01.preview_image.r1",
			"generation_job_key":"job.shot_01.preview_image.r1",
			"artifact_version_key":"shot_01.preview_image.r1.artifact.v1",
			"worker_task_id":"28000000-0000-0000-0000-000000000000"
		}`),
	}

	text := formatProducerSignalReminder([]db.ProducerPendingSignal{signal})

	if strings.Contains(text, "semantic_key_missing") {
		t.Fatalf("reminder should not contain semantic_key_missing:\n%s", text)
	}
	if strings.Contains(text, "28000000-0000-0000-0000-000000000000") {
		t.Fatalf("reminder leaked worker task UUID:\n%s", text)
	}
	if !strings.Contains(text, "scope_ref=shot/shot_01") ||
		!strings.Contains(text, "render_plan_ref=render_plan/shot_01.preview_image.r1") ||
		!strings.Contains(text, "generation_job_ref=generation_job/job.shot_01.preview_image.r1") ||
		!strings.Contains(text, "artifact_ref=artifact_version/shot_01.preview_image.r1.artifact.v1") {
		t.Fatalf("reminder missing semantic refs:\n%s", text)
	}
}

func TestProducerGraphCompileCapturesGraphInfo(t *testing.T) {
	registry := agenteino.NewGraphInfoRegistry()
	_, err := NewGraph(GraphConfig{
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
	for _, node := range []string{"load_context", "prepare_turn_state", "before_model", "call_model", "prepare_tool_message", "execute_tools", "append_tool_results", "finalize_response"} {
		if _, ok := info.Nodes[node]; !ok {
			t.Fatalf("node %q missing from graph info", node)
		}
	}
}

func TestProducerGraphExplicitToolLoopCapturesGraphInfo(t *testing.T) {
	registry := agenteino.NewGraphInfoRegistry()
	_, err := NewGraph(GraphConfig{
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
	for _, node := range []string{"load_context", "prepare_turn_state", "before_model", "call_model", "prepare_tool_message", "execute_tools", "append_tool_results", "finalize_response"} {
		if _, ok := info.Nodes[node]; !ok {
			t.Fatalf("node %q missing from graph info", node)
		}
	}
	for _, node := range []string{"execute_legacy_tool", "append_legacy_tool_result"} {
		if _, ok := info.Nodes[node]; ok {
			t.Fatalf("legacy node %q must not exist in producer graph info", node)
		}
	}
	if _, ok := info.Nodes["check_signals_before_finalize"]; ok {
		t.Fatal("check_signals_before_finalize must not exist in producer graph info")
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
	if !graphInfoHasBranchTarget(info.Branches, "append_tool_results", "before_model") {
		t.Fatalf("branch append_tool_results -> before_model missing from graph info: %#v", info.Branches)
	}
	if graphInfoHasBranchTarget(info.Branches, "append_tool_results", "check_signals_before_finalize") {
		t.Fatalf("branch append_tool_results -> check_signals_before_finalize must not exist: %#v", info.Branches)
	}
	if !graphInfoHasBranchTarget(info.Branches, "append_tool_results", "finalize_response") {
		t.Fatalf("branch append_tool_results -> finalize_response missing from graph info: %#v", info.Branches)
	}
	if !graphInfoHasEdge(info.Edges, "before_model", "call_model") {
		t.Fatalf("edge before_model -> call_model missing from graph info: %#v", info.Edges)
	}
}

func TestProducerGraphLeavesSignalsForExecutorWakeAfterModelRound(t *testing.T) {
	signals := make([]db.ProducerPendingSignal, 0, 5)
	for i := byte(0); i < 5; i++ {
		signals = append(signals, db.ProducerPendingSignal{
			ID:               uuidWithByte(80 + i),
			WorkspaceID:      uuidWithByte(1),
			ProducerThreadID: uuidWithByte(2),
			SourceTaskID:     uuidWithByte(30 + i),
			SignalType:       "craftsman_render_plan_ready",
			ScopeType:        "shot",
			ScopeID:          uuidWithByte(40 + i),
			RenderPlanID:     uuidWithByte(50 + i),
			Status:           "pending",
			Payload:          []byte(`{"target_phase":"preview_image"}`),
		})
	}
	signalRuntime := &fakeProducerSignalRuntime{pending: signals}
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		{AssistantText: "本轮准备结束。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "继续"}},
		Responder:          responder,
		NativeToolRegistry: mustTestNativeToolRegistry(t, "read_project_context"),
		SignalRuntime:      signalRuntime,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(9),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "本轮准备结束。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if signalRuntime.claimCalls != 0 {
		t.Fatalf("claim calls = %d, want 0", signalRuntime.claimCalls)
	}
	if len(signalRuntime.claimed) != 0 {
		t.Fatalf("claimed signals = %d, want 0", len(signalRuntime.claimed))
	}
	if len(responder.contexts) != 1 {
		t.Fatalf("model calls = %d, want 1", len(responder.contexts))
	}
	if len(responder.contexts[0].PendingReminders) != 0 {
		t.Fatalf("first model reminders = %#v", responder.contexts[0].PendingReminders)
	}
	if len(out.PersistentTriggerMessages) != 0 {
		t.Fatalf("persistent trigger messages = %#v", out.PersistentTriggerMessages)
	}
}

func TestProducerGraphDoesNotDrainNewSignalsBeforeFinalize(t *testing.T) {
	signalRuntime := &fakeProducerSignalRuntime{
		pendingByClaimCall: [][]db.ProducerPendingSignal{
			{
				{
					ID:               uuidWithByte(88),
					WorkspaceID:      uuidWithByte(1),
					ProducerThreadID: uuidWithByte(2),
					SourceTaskID:     uuidWithByte(31),
					SignalType:       "craftsman_render_plan_ready",
					ScopeType:        "shot",
					ScopeID:          uuidWithByte(41),
					RenderPlanID:     uuidWithByte(51),
					Status:           "pending",
					Payload:          []byte(`{"target_phase":"preview_image"}`),
				},
			},
		},
	}
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		{AssistantText: "本轮准备结束。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "继续"}},
		Responder:          responder,
		NativeToolRegistry: mustTestNativeToolRegistry(t, "read_project_context"),
		SignalRuntime:      signalRuntime,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(9),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "本轮准备结束。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if len(responder.contexts) != 1 {
		t.Fatalf("model calls = %d, want 1", len(responder.contexts))
	}
	if len(responder.contexts[0].PendingReminders) != 0 {
		t.Fatalf("first model reminders = %#v", responder.contexts[0].PendingReminders)
	}
	if len(out.PersistentTriggerMessages) != 0 {
		t.Fatalf("persistent trigger messages = %#v", out.PersistentTriggerMessages)
	}
	if len(signalRuntime.claimed) != 0 || signalRuntime.claimCalls != 0 {
		t.Fatalf("claimCalls=%d claimed=%#v", signalRuntime.claimCalls, signalRuntime.claimed)
	}
}

func TestProducerGraphDoesNotDrainSignalsInsideCurrentTurn(t *testing.T) {
	signals := []db.ProducerPendingSignal{
		{
			ID:               uuidWithByte(81),
			WorkspaceID:      uuidWithByte(1),
			ProducerThreadID: uuidWithByte(2),
			SourceTaskID:     uuidWithByte(31),
			SignalType:       "craftsman_render_plan_ready",
			ScopeType:        "shot",
			ScopeID:          uuidWithByte(41),
			RenderPlanID:     uuidWithByte(51),
			Status:           "claimed",
			ClaimedByTaskID:  uuidWithByte(9),
			Payload:          []byte(`{"target_phase":"preview_image"}`),
		},
		{
			ID:               uuidWithByte(82),
			WorkspaceID:      uuidWithByte(1),
			ProducerThreadID: uuidWithByte(2),
			SourceTaskID:     uuidWithByte(32),
			SignalType:       "craftsman_render_plan_ready",
			ScopeType:        "shot",
			ScopeID:          uuidWithByte(42),
			RenderPlanID:     uuidWithByte(52),
			Status:           "claimed",
			ClaimedByTaskID:  uuidWithByte(9),
			Payload:          []byte(`{"target_phase":"preview_image"}`),
		},
	}
	signalRuntime := &fakeProducerSignalRuntime{
		pending: signals,
	}
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-read", "read_project_context", `{"brief":"先读取项目","scope":{"type":"workspace","id":""}}`),
		{AssistantText: "读取完成。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "继续"}},
		Responder:          responder,
		NativeToolRegistry: mustTestNativeToolRegistry(t, "read_project_context"),
		SignalRuntime:      signalRuntime,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(9),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "读取完成。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if len(responder.contexts) != 2 {
		t.Fatalf("model calls = %d, want 2", len(responder.contexts))
	}
	if len(responder.contexts[0].PendingReminders) != 0 || len(responder.contexts[1].PendingReminders) != 0 {
		t.Fatalf("signal reminder must not be injected into current turn: %#v", responder.contexts)
	}
	if len(out.PersistentTriggerMessages) != 0 {
		t.Fatalf("persistent trigger messages = %#v", out.PersistentTriggerMessages)
	}
	if signalRuntime.claimCalls != 0 || len(signalRuntime.claimed) != 0 {
		t.Fatalf("current turn must not claim pending signals: calls=%d claimed=%#v", signalRuntime.claimCalls, signalRuntime.claimed)
	}
}

func TestProducerGraphExplicitToolLoopExecutesToolWithEinoToolNode(t *testing.T) {
	registry := mustTestNativeToolRegistry(t, "create_agent_text_node")
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-text", "create_agent_text_node", `{"title":"brief","text":"hello"}`),
		{AssistantText: "已保存 brief。"},
	}}
	graph, err := NewGraph(GraphConfig{
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

func TestProducerGraphFinalizesAfterAsyncCraftsmanDispatch(t *testing.T) {
	dispatchTool := &testNativeTool{
		name:   "dispatch_craftsman",
		result: "Craftsman 派发结果\n- 阶段：preview_image\n- 摘要：已将 1 个分镜的预览图 RenderPlan 任务加入队列。Craftsman 编译 RenderPlan 后，工程会自动提交 Worker 生成任务。 当前仅表示 Craftsman 任务已排队，不表示图片已经生成完成。",
	}
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-dispatch", "dispatch_craftsman", `{"brief":"派发预览图","scope":{"type":"shot","id":"shot_01"},"target_phase":"preview_image","execution_policy":"execute_immediately"}`),
		nativeToolCallOutput("call-read", "read_project_context", `{"brief":"不应继续轮询"}`),
	}}
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "生成预览图"}},
		Responder:          responder,
		NativeToolRegistry: mustTestNativeToolRegistryWithTools(t, dispatchTool, &testNativeTool{name: "read_project_context"}),
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(responder.contexts) != 1 {
		t.Fatalf("model calls = %d, want 1; Producer must not poll after async dispatch", len(responder.contexts))
	}
	if !strings.Contains(out.AssistantText, "已派发 Craftsman 生成任务") {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if out.Metadata["finalized_after_async_craftsman_dispatch"] != true {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
	if len(out.SameTurnMessages) != 2 || out.SameTurnMessages[1].ToolName != "dispatch_craftsman" {
		t.Fatalf("same-turn messages = %#v", out.SameTurnMessages)
	}
}

func TestProducerGraphExplicitToolLoopReturnsSameTurnToolTrace(t *testing.T) {
	registry := mustTestNativeToolRegistry(t, "create_agent_text_node")
	graph, err := NewGraph(GraphConfig{
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

func TestProducerGraphAddsPendingReminderAfterContinuousReadToolCalls(t *testing.T) {
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-read-1", "read_project_context", `{"brief":"读取1","scope":{"type":"workspace","id":""}}`),
		nativeToolCallOutput("call-read-2", "read_project_context", `{"brief":"读取2","scope":{"type":"workspace","id":""}}`),
		nativeToolCallOutput("call-read-3", "read_project_context", `{"brief":"读取3","scope":{"type":"workspace","id":""}}`),
		nativeToolCallOutput("call-read-4", "read_project_context", `{"brief":"读取4","scope":{"type":"workspace","id":""}}`),
		nativeToolCallOutput("call-read-5", "read_project_context", `{"brief":"读取5","scope":{"type":"workspace","id":""}}`),
		{AssistantText: "已停止重复读取。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "检查循环"}},
		Responder:          responder,
		NativeToolRegistry: mustTestNativeToolRegistry(t, "read_project_context"),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(responder.contexts) < 6 {
		t.Fatalf("model calls = %d, want at least 6", len(responder.contexts))
	}
	reminders := responder.contexts[5].PendingReminders
	if len(reminders) != 1 {
		t.Fatalf("pending reminders = %#v, want one", reminders)
	}
	if !strings.Contains(reminders[0], "read_project_context") || !strings.Contains(reminders[0], "连续调用") {
		t.Fatalf("unexpected reminder = %q", reminders[0])
	}
}

func TestProducerGraphDoesNotReturnSystemReminderAfterContinuousReadToolCalls(t *testing.T) {
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-read-1", "read_project_context", `{"brief":"读取1","scope":{"type":"workspace","id":""}}`),
		nativeToolCallOutput("call-read-2", "read_project_context", `{"brief":"读取2","scope":{"type":"workspace","id":""}}`),
		nativeToolCallOutput("call-read-3", "read_project_context", `{"brief":"读取3","scope":{"type":"workspace","id":""}}`),
		nativeToolCallOutput("call-read-4", "read_project_context", `{"brief":"读取4","scope":{"type":"workspace","id":""}}`),
		nativeToolCallOutput("call-read-5", "read_project_context", `{"brief":"读取5","scope":{"type":"workspace","id":""}}`),
		{AssistantText: "已停止重复读取。"},
	}}
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "检查循环"}},
		Responder:          responder,
		NativeToolRegistry: mustTestNativeToolRegistry(t, "read_project_context"),
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{MaxToolCalls: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range out.SameTurnMessages {
		if message.MessageType == "system_reminder" && strings.Contains(message.Content, "read_project_context") {
			t.Fatalf("same-turn messages must not persist tool-loop reminder: %#v", out.SameTurnMessages)
		}
	}
}

func TestProducerGraphDoesNotPersistSignalReminderAcrossToolLoop(t *testing.T) {
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-read", "read_project_context", `{"brief":"读取项目","scope":{"type":"workspace","id":""}}`),
		{AssistantText: "已处理 signal。"},
	}}
	signalRuntime := &fakeProducerSignalRuntime{
		pending: []db.ProducerPendingSignal{
			{
				ID:               uuidWithByte(88),
				WorkspaceID:      uuidWithByte(1),
				ProducerThreadID: uuidWithByte(2),
				SourceTaskID:     uuidWithByte(31),
				SignalType:       "craftsman_render_plan_ready",
				ScopeType:        "shot",
				ScopeID:          uuidWithByte(41),
				RenderPlanID:     uuidWithByte(51),
				Payload:          []byte(`{"target_phase":"preview_image","render_plan_status":"waiting_for_approval"}`),
			},
		},
	}
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "继续"}},
		Responder:          responder,
		NativeToolRegistry: mustTestNativeToolRegistry(t, "read_project_context"),
		SignalRuntime:      signalRuntime,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{
		WorkspaceID:  uuidWithByte(1),
		ThreadID:     uuidWithByte(2),
		TaskID:       uuidWithByte(9),
		MaxToolCalls: 50,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, message := range out.SameTurnMessages {
		if message.MessageType == "system_reminder" && strings.Contains(message.Content, "craftsman_render_plan_ready") {
			t.Fatalf("same-turn messages must not persist signal reminder: %#v", out.SameTurnMessages)
		}
	}
}

func TestProducerGraphDoesNotDrainGrowingSignalReminderBeforeFinalize(t *testing.T) {
	responder := &recordingResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-read", "read_project_context", `{"brief":"读取项目","scope":{"type":"workspace","id":""}}`),
		{AssistantText: "已处理最新 signal。"},
	}}
	firstSignal := db.ProducerPendingSignal{
		ID:               uuidWithByte(88),
		WorkspaceID:      uuidWithByte(1),
		ProducerThreadID: uuidWithByte(2),
		SourceTaskID:     uuidWithByte(31),
		SignalType:       "craftsman_render_plan_ready",
		ScopeType:        "shot",
		ScopeID:          uuidWithByte(41),
		RenderPlanID:     uuidWithByte(51),
		Payload:          []byte(`{"target_phase":"preview_image","render_plan_status":"waiting_for_approval"}`),
	}
	secondSignal := db.ProducerPendingSignal{
		ID:               uuidWithByte(89),
		WorkspaceID:      uuidWithByte(1),
		ProducerThreadID: uuidWithByte(2),
		SourceTaskID:     uuidWithByte(32),
		SignalType:       "craftsman_render_plan_ready",
		ScopeType:        "shot",
		ScopeID:          uuidWithByte(42),
		RenderPlanID:     uuidWithByte(52),
		Payload:          []byte(`{"target_phase":"preview_image","render_plan_status":"waiting_for_approval"}`),
	}
	signalRuntime := &fakeProducerSignalRuntime{
		pendingByClaimCall: [][]db.ProducerPendingSignal{
			{firstSignal},
			{secondSignal},
		},
	}
	graph, err := NewGraph(GraphConfig{
		Loader:             fakeContextLoader{context: ProducerContext{LatestUserText: "继续"}},
		Responder:          responder,
		NativeToolRegistry: mustTestNativeToolRegistry(t, "read_project_context"),
		SignalRuntime:      signalRuntime,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{
		WorkspaceID:  uuidWithByte(1),
		ThreadID:     uuidWithByte(2),
		TaskID:       uuidWithByte(9),
		MaxToolCalls: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "已处理最新 signal。" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}

	for _, message := range out.SameTurnMessages {
		if message.MessageType == "system_reminder" && strings.Contains(message.Content, "待处理 Producer signal") {
			t.Fatalf("same-turn messages must not persist signal reminder: %#v", out.SameTurnMessages)
		}
	}
	if len(out.PersistentTriggerMessages) != 0 {
		t.Fatalf("persistent trigger messages = %#v", out.PersistentTriggerMessages)
	}
	if signalRuntime.claimCalls != 0 || len(signalRuntime.claimed) != 0 {
		t.Fatalf("current turn must not claim pending signals: calls=%d claimed=%#v", signalRuntime.claimCalls, signalRuntime.claimed)
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
		NativeToolRegistry: mustTestNativeToolRegistry(t, "create_agent_text_node"),
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

type fakeProducerSignalRuntime struct {
	pending            []db.ProducerPendingSignal
	pendingByClaimCall [][]db.ProducerPendingSignal
	claimedByListCall  [][]db.ProducerPendingSignal
	claimed            []db.ProducerPendingSignal
	claimCalls         int
	listCalls          int
}

func (f *fakeProducerSignalRuntime) ClaimProducerPendingSignals(_ context.Context, params agentruntime.ClaimProducerPendingSignalsParams) ([]db.ProducerPendingSignal, error) {
	f.claimCalls++
	pending := f.pending
	if len(f.pendingByClaimCall) > 0 {
		index := f.claimCalls - 1
		if index >= 0 && index < len(f.pendingByClaimCall) {
			pending = f.pendingByClaimCall[index]
		} else {
			pending = nil
		}
	} else {
		f.pending = nil
	}
	if len(pending) == 0 {
		return nil, nil
	}
	claimed := make([]db.ProducerPendingSignal, 0, len(pending))
	for _, signal := range pending {
		signal.Status = "claimed"
		signal.ClaimedByTaskID = params.ClaimedByTaskID
		signal.ProducerThreadID = params.ProducerThreadID
		claimed = append(claimed, signal)
	}
	f.claimed = append(f.claimed, claimed...)
	return claimed, nil
}

func (f *fakeProducerSignalRuntime) ListClaimedProducerSignalsByTask(_ context.Context, workspaceID, producerThreadID, taskID pgtype.UUID) ([]db.ProducerPendingSignal, error) {
	f.listCalls++
	if len(f.claimedByListCall) > 0 {
		index := f.listCalls - 1
		if index >= 0 && index < len(f.claimedByListCall) {
			return signalsForTask(f.claimedByListCall[index], workspaceID, producerThreadID, taskID), nil
		}
		return nil, nil
	}
	return signalsForTask(f.claimed, workspaceID, producerThreadID, taskID), nil
}

func signalsForTask(signals []db.ProducerPendingSignal, workspaceID, producerThreadID, taskID pgtype.UUID) []db.ProducerPendingSignal {
	out := make([]db.ProducerPendingSignal, 0, len(signals))
	for _, signal := range signals {
		if signal.WorkspaceID == workspaceID && signal.ProducerThreadID == producerThreadID && signal.ClaimedByTaskID == taskID {
			out = append(out, signal)
		}
	}
	return out
}

type testNativeTool struct {
	name       string
	interrupt  bool
	calledName string
	calledID   string
	result     string
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
	if strings.TrimSpace(t.result) != "" {
		return t.result, nil
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
