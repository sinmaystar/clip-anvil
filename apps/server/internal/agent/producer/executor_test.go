package producer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestExecutorPersistsAssistantMessageOnSuccess(t *testing.T) {
	runtime := &fakeRuntime{}
	broadcaster := &fakeBroadcaster{}
	graph := &fakeGraph{output: ProducerTurnOutput{AssistantText: "assistant reply"}}
	executor := NewExecutor(ExecutorConfig{
		Runtime:     runtime,
		Graph:       graph,
		Broadcaster: broadcaster,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	if runtime.runningTask != uuidWithByte(3) {
		t.Fatal("task was not marked running")
	}
	if runtime.assistantText != "assistant reply" {
		t.Fatalf("assistant text = %q", runtime.assistantText)
	}
	if runtime.succeededTask != uuidWithByte(3) {
		t.Fatal("task was not marked succeeded")
	}
	if broadcaster.messageCount != 1 || broadcaster.taskCount == 0 {
		t.Fatalf("broadcast counts = messages %d tasks %d", broadcaster.messageCount, broadcaster.taskCount)
	}
	wantCheckpoint := "agent:eino:producer_turn:01000000-0000-0000-0000-000000000000:02000000-0000-0000-0000-000000000000:03000000-0000-0000-0000-000000000000"
	if graph.runOptions.CheckPointID != wantCheckpoint {
		t.Fatalf("checkpoint id = %q, want %q", graph.runOptions.CheckPointID, wantCheckpoint)
	}
	if runtime.threadCheckpoint != wantCheckpoint {
		t.Fatalf("thread checkpoint = %q, want %q", runtime.threadCheckpoint, wantCheckpoint)
	}
}

func TestExecutorReleasesUnprocessedClaimedSignalsOnSuccess(t *testing.T) {
	runtime := &fakeRuntime{}
	graph := &fakeGraph{output: ProducerTurnOutput{AssistantText: "只处理了一条 signal，剩余 signal 下轮继续。"}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(runtime.releasedSignals) != 1 {
		t.Fatalf("released signals = %#v", runtime.releasedSignals)
	}
	release := runtime.releasedSignals[0]
	if release.workspaceID != uuidWithByte(1) || release.taskID != uuidWithByte(3) {
		t.Fatalf("release target = %#v", release)
	}
	if !strings.Contains(release.reason, "producer_turn_completed") {
		t.Fatalf("release reason = %q", release.reason)
	}
}

func TestExecutorMarksClaimedInformationalSignalsProcessedOnSuccess(t *testing.T) {
	runtime := &fakeRuntime{
		claimedSignals: []db.ProducerPendingSignal{
			{
				ID:               uuidWithByte(21),
				WorkspaceID:      uuidWithByte(1),
				ProducerThreadID: uuidWithByte(2),
				SignalType:       "worker_generation_completed",
				Status:           "claimed",
				ClaimedByTaskID:  uuidWithByte(3),
			},
			{
				ID:               uuidWithByte(23),
				WorkspaceID:      uuidWithByte(1),
				ProducerThreadID: uuidWithByte(2),
				SignalType:       "review_completed",
				Status:           "claimed",
				ClaimedByTaskID:  uuidWithByte(3),
			},
			{
				ID:               uuidWithByte(22),
				WorkspaceID:      uuidWithByte(1),
				ProducerThreadID: uuidWithByte(2),
				SignalType:       "craftsman_render_plan_ready",
				Status:           "claimed",
				ClaimedByTaskID:  uuidWithByte(3),
			},
		},
	}
	graph := &fakeGraph{output: ProducerTurnOutput{AssistantText: "已感知生成完成，继续等待用户反馈。"}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(runtime.processedSignals) != 2 || runtime.processedSignals[0] != uuidWithByte(21) || runtime.processedSignals[1] != uuidWithByte(23) {
		t.Fatalf("processed signals = %#v", runtime.processedSignals)
	}
	if len(runtime.releasedSignals) != 1 {
		t.Fatalf("released signals = %#v", runtime.releasedSignals)
	}
}

func TestExecutorTranslatesCraftsmanWakeTaskInputToRuntimeTriggerText(t *testing.T) {
	runtime := &fakeRuntime{
		runningTaskInput: []byte(`{
			"trigger":"craftsman_render_plan_ready",
			"craftsman_task_id":"04000000-0000-0000-0000-000000000000",
			"craftsman_thread_id":"03000000-0000-0000-0000-000000000000",
			"scope_type":"shot",
			"scope_id":"02000000-0000-0000-0000-000000000000",
			"shot_id":"02000000-0000-0000-0000-000000000000",
			"target_phase":"preview_image"
		}`),
	}
	graph := &fakeGraph{output: ProducerTurnOutput{AssistantText: "assistant reply"}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(graph.input.RuntimeTriggerText, "Craftsman 已完成 RenderPlan 编译") ||
		!strings.Contains(graph.input.RuntimeTriggerText, "preview_image") ||
		!strings.Contains(graph.input.RuntimeTriggerText, "decide_render_plan") {
		t.Fatalf("runtime trigger text = %q", graph.input.RuntimeTriggerText)
	}
}

func TestExecutorTranslatesReviewCompletedWakeTaskInputToRuntimeTriggerText(t *testing.T) {
	runtime := &fakeRuntime{
		runningTaskInput: []byte(`{
			"trigger":"review_completed",
			"review_record_id":"28000000-0000-0000-0000-000000000000",
			"review_task":"preview_image_review",
			"verdict":"rejected",
			"should_retry":true,
			"scope_type":"shot",
			"scope_id":"02000000-0000-0000-0000-000000000000",
			"target_phase":"preview_image"
		}`),
	}
	graph := &fakeGraph{output: ProducerTurnOutput{AssistantText: "assistant reply"}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(graph.input.RuntimeTriggerText, "Reviewer 已提交评审结果") ||
		!strings.Contains(graph.input.RuntimeTriggerText, "review_record") ||
		!strings.Contains(graph.input.RuntimeTriggerText, "rejected") {
		t.Fatalf("runtime trigger text = %q", graph.input.RuntimeTriggerText)
	}
}

func TestExecutorPersistsNativeToolTraceBeforeFinalAssistantMessage(t *testing.T) {
	runtime := &fakeRuntime{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph: &fakeGraph{output: ProducerTurnOutput{
			AssistantText: "已保存 brief。",
			SameTurnMessages: []ProducerSameTurnMessage{
				{
					Role:          "assistant",
					MessageType:   "tool_call",
					Content:       "调用 create_agent_text_node",
					ToolCallID:    "call-text",
					ToolName:      "create_agent_text_node",
					ToolArguments: map[string]any{"title": "brief", "text": "hello"},
				},
				{
					Role:        "tool",
					MessageType: "tool_result",
					Content:     "工具返回：已保存。",
					ToolCallID:  "call-text",
					ToolName:    "create_agent_text_node",
				},
			},
		}},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	got := runtime.appendedMessageTypes()
	want := []string{"tool_call", "tool_result", "text"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("message types = %#v, want %#v", got, want)
	}
	if runtime.appended[0].Role != "assistant" || runtime.appended[1].Role != "tool" {
		t.Fatalf("persisted roles = %#v", runtime.appended)
	}
	if !bytes.Contains(runtime.appended[0].RawMessage, []byte(`"tool_call_id":"call-text"`)) ||
		!bytes.Contains(runtime.appended[1].RawMessage, []byte(`"tool_call_id":"call-text"`)) {
		t.Fatalf("raw tool messages = %s / %s", runtime.appended[0].RawMessage, runtime.appended[1].RawMessage)
	}
	if len(runtime.updated) != 1 || !bytes.Contains(runtime.updated[0].Content, []byte(`"status":"succeeded"`)) {
		t.Fatalf("updated tool status = %#v", runtime.updated)
	}
}

func TestExecutorDoesNotPersistSystemReminderBeforeFinalAssistantMessage(t *testing.T) {
	runtime := &fakeRuntime{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph: &fakeGraph{output: ProducerTurnOutput{
			AssistantText: "已停止重复读取。",
			SameTurnMessages: []ProducerSameTurnMessage{
				{
					Role:        "system",
					MessageType: "system_reminder",
					Content:     "<system-reminder>你已连续调用 read_project_context 5 次。</system-reminder>",
				},
			},
		}},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	got := runtime.appendedMessageTypes()
	want := []string{"text"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("message types = %#v, want %#v", got, want)
	}
	if runtime.appended[0].Role != "assistant" || bytes.Contains(runtime.appended[0].Content, []byte(`"type":"system_reminder"`)) {
		t.Fatalf("persisted message should be final assistant only: %#v content=%s", runtime.appended[0], runtime.appended[0].Content)
	}
}

func TestExecutorPersistsLiveNativeToolTraceFromGraphContext(t *testing.T) {
	runtime := &fakeRuntime{}
	broadcaster := &fakeBroadcaster{}
	executor := NewExecutor(ExecutorConfig{
		Runtime:     runtime,
		Graph:       &fakeGraph{emitLiveToolTrace: true, output: ProducerTurnOutput{AssistantText: "完成。"}},
		Broadcaster: broadcaster,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	got := runtime.appendedMessageTypes()
	want := []string{"tool_call", "tool_result", "text"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("message types = %#v, want %#v", got, want)
	}
	if len(runtime.updated) != 1 || !bytes.Contains(runtime.updated[0].Content, []byte(`"status":"succeeded"`)) {
		t.Fatalf("updated live tool status = %#v", runtime.updated)
	}
	if broadcaster.messageCount < 2 || broadcaster.messageUpdateCount < 1 {
		t.Fatalf("broadcast counts = messages %d updates %d", broadcaster.messageCount, broadcaster.messageUpdateCount)
	}
}

func TestExecutorDoesNotPersistSystemReminderWithLiveNativeToolTrace(t *testing.T) {
	runtime := &fakeRuntime{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph: &fakeGraph{
			emitLiveToolTrace: true,
			output: ProducerTurnOutput{
				AssistantText: "我会换一种方式继续。",
				SameTurnMessages: []ProducerSameTurnMessage{
					{
						Role:        "system",
						MessageType: "system_reminder",
						Content:     "<system-reminder>你已连续调用 read_project_context 5 次。</system-reminder>",
					},
				},
			},
		},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	got := runtime.appendedMessageTypes()
	want := []string{"tool_call", "tool_result", "text"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("message types = %#v, want %#v", got, want)
	}
	if len(runtime.updated) != 1 {
		t.Fatalf("updated count = %d, want 1; updated=%#v", len(runtime.updated), runtime.updated)
	}
}

func TestExecutorDoesNotPersistGrowingLiveSignalReminder(t *testing.T) {
	runtime := &fakeRuntime{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph: &fakeGraph{
			emitLiveToolTrace: true,
			output: ProducerTurnOutput{
				AssistantText: "已处理 signal。",
				SameTurnMessages: []ProducerSameTurnMessage{
					{
						Role:        "system",
						MessageType: "system_reminder",
						Content:     "<system-reminder>你有 1 个待处理 Producer signal。\n1. craftsman_render_plan_ready: render_plan_id=rp1</system-reminder>",
					},
					{
						Role:        "system",
						MessageType: "system_reminder",
						Content:     "<system-reminder>你有 2 个待处理 Producer signal。\n1. craftsman_render_plan_ready: render_plan_id=rp1\n2. craftsman_render_plan_ready: render_plan_id=rp2</system-reminder>",
					},
				},
			},
		},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	got := runtime.appendedMessageTypes()
	want := []string{"tool_call", "tool_result", "text"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("message types = %#v, want %#v", got, want)
	}
	if len(runtime.updated) != 1 {
		t.Fatalf("updated count = %d, want 1; updated=%#v", len(runtime.updated), runtime.updated)
	}
}

func TestExecutorBroadcastsStreamDeltasBeforeFinalMessage(t *testing.T) {
	runtime := &fakeRuntime{}
	broadcaster := &fakeBroadcaster{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph: &fakeGraph{
			output: ProducerTurnOutput{AssistantText: "streamed reply"},
			deltas: []string{"streamed ", "reply"},
		},
		Broadcaster: broadcaster,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(broadcaster.deltas, ""); got != "streamed reply" {
		t.Fatalf("stream deltas = %q", got)
	}
	if broadcaster.messageCount != 1 {
		t.Fatalf("message broadcasts = %d, want 1", broadcaster.messageCount)
	}
}

func TestExecutorPassesTraceCallbacksToGraph(t *testing.T) {
	runtime := &fakeRuntime{}
	graph := &fakeGraph{output: ProducerTurnOutput{AssistantText: "assistant reply"}}
	traceCallback := callbacks.NewHandlerBuilder().Build()
	executor := NewExecutor(ExecutorConfig{
		Runtime:        runtime,
		Graph:          graph,
		TraceCallbacks: []callbacks.Handler{traceCallback},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(graph.runOptions.Callbacks) != 1 {
		t.Fatalf("callbacks len = %d, want 1", len(graph.runOptions.Callbacks))
	}
	if got := traceAttribute(graph.ctx, "clipanvil.agent.role"); got != "producer" {
		t.Fatalf("trace role = %q, want producer", got)
	}
}

func TestExecutorPersistsSelectedModelMetadata(t *testing.T) {
	runtime := &fakeRuntime{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph: &fakeGraph{output: ProducerTurnOutput{
			AssistantText: "assistant reply",
			Metadata: map[string]any{
				"provider":           "volcengine",
				"model_id":           "workspace-model",
				"model_display_name": "Workspace Model",
			},
		}},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(runtime.assistantRawMessage, &raw); err != nil {
		t.Fatalf("unmarshal raw message: %v", err)
	}
	if raw["model_id"] != "workspace-model" || raw["model_display_name"] != "Workspace Model" {
		t.Fatalf("raw metadata = %#v", raw)
	}
}

func TestExecutorMarksTaskWaitingForNativeInterrupt(t *testing.T) {
	runtime := &fakeRuntime{}
	registry := mustTestNativeToolRegistryWithTools(t, &testNativeTool{name: "request_user_decision", interrupt: true})
	graph, err := NewGraph(GraphConfig{
		Loader: fakeContextLoader{context: ProducerContext{LatestUserText: "需要决策"}},
		Responder: &sequenceResponder{outputs: []ProducerTurnOutput{
			nativeToolCallOutput("call-decision", "request_user_decision", `{"title":"确认","message":"继续吗"}`),
		}},
		NativeToolRegistry: registry,
		CheckPointStore:    fakeEinoCheckpointStore{},
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})

	err = executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.waitingTask != uuidWithByte(3) {
		t.Fatalf("waiting task = %v", runtime.waitingTask)
	}
	if runtime.failedTask.Valid {
		t.Fatalf("task should not fail, failed=%v", runtime.failedTask)
	}
	if got := runtime.appendedMessageTypes(); !slices.Equal(got, []string{"tool_call"}) {
		t.Fatalf("appended message types = %#v, want only running tool_call", got)
	}
	if !containsString(runtime.eventTypes, "graph_interrupted") {
		t.Fatalf("events = %#v", runtime.eventTypes)
	}
}

func TestExecutorUsesAgentModelUnavailableErrorCode(t *testing.T) {
	runtime := &fakeRuntime{}
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph:   &fakeGraph{err: NewAgentError("agent_model_unavailable", "model disabled")},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if runtime.failedCode != "agent_model_unavailable" {
		t.Fatalf("failed code = %q, want agent_model_unavailable", runtime.failedCode)
	}
}

func TestExecutorPersistsErrorMessageOnFailure(t *testing.T) {
	runtime := &fakeRuntime{}
	broadcaster := &fakeBroadcaster{}
	executor := NewExecutor(ExecutorConfig{
		Runtime:     runtime,
		Graph:       &fakeGraph{err: errors.New("model unavailable")},
		Broadcaster: broadcaster,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if runtime.failedTask != uuidWithByte(3) {
		t.Fatal("task was not marked failed")
	}
	if runtime.assistantMessageType != "error" {
		t.Fatalf("assistant message type = %q, want error", runtime.assistantMessageType)
	}
	if strings.Contains(runtime.assistantText, "Producer") {
		t.Fatalf("user visible error leaked internal role: %q", runtime.assistantText)
	}
	if broadcaster.messageCount != 1 || broadcaster.taskCount == 0 {
		t.Fatalf("broadcast counts = messages %d tasks %d", broadcaster.messageCount, broadcaster.taskCount)
	}
}

func TestExecutorLogsTaskFailureContext(t *testing.T) {
	runtime := &fakeRuntime{}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	executor := NewExecutor(ExecutorConfig{
		Runtime: runtime,
		Graph:   &fakeGraph{err: NewAgentError("agent_model_unavailable", "model disabled")},
		Logger:  logger,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	logText := logs.String()
	if !strings.Contains(logText, "producer task failed") ||
		!strings.Contains(logText, `"error_code":"agent_model_unavailable"`) ||
		!strings.Contains(logText, `"workspace_id":"01000000-0000-0000-0000-000000000000"`) ||
		!strings.Contains(logText, `"task_id":"03000000-0000-0000-0000-000000000000"`) {
		t.Fatalf("logs = %s", logText)
	}
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}

type fakeGraph struct {
	output            ProducerTurnOutput
	deltas            []string
	err               error
	emitLiveToolTrace bool
	input             ProducerTurnInput
	runOptions        agenteino.RunOptions
	ctx               context.Context
}

func (f *fakeGraph) Run(ctx context.Context, input ProducerTurnInput, options ...agenteino.RunOptions) (ProducerTurnOutput, error) {
	f.ctx = ctx
	f.input = input
	if len(options) > 0 {
		f.runOptions = options[0]
	}
	for _, delta := range f.deltas {
		if input.EmitDelta != nil {
			if err := input.EmitDelta(ctx, ProducerStreamDelta{Delta: delta}); err != nil {
				return ProducerTurnOutput{}, err
			}
		}
	}
	if f.emitLiveToolTrace {
		sink, ok := agenttools.NativeToolTraceSinkFromContext(ctx)
		if !ok {
			return ProducerTurnOutput{}, errors.New("live native tool trace sink missing")
		}
		runtime := agenttools.NativeRuntimeContext{
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(2),
			TaskID:      uuidWithByte(3),
			ToolCallID:  "call-live",
		}
		if err := sink.NativeToolCallStarted(ctx, runtime, agenttools.NativeToolTrace{
			ToolName:  "upsert_project_brief",
			Arguments: map[string]any{"brief": "实时写入项目 brief。"},
		}); err != nil {
			return ProducerTurnOutput{}, err
		}
		if err := sink.NativeToolCallCompleted(ctx, runtime, agenttools.NativeToolTrace{
			ToolName: "upsert_project_brief",
			Result:   "已写入 CreativeBrief。",
		}); err != nil {
			return ProducerTurnOutput{}, err
		}
	}
	return f.output, f.err
}

func traceAttribute(ctx context.Context, key string) string {
	for _, attr := range cozelooptrace.AttributesFromContext(ctx) {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}

type fakeBroadcaster struct {
	messageCount       int
	messageUpdateCount int
	taskCount          int
	eventCount         int
	deltas             []string
}

func (f *fakeBroadcaster) BroadcastAgentMessage(pgtype.UUID, db.AgentMessage, db.AgentEvent) {
	f.messageCount++
}

func (f *fakeBroadcaster) BroadcastAgentMessageUpdated(pgtype.UUID, db.AgentMessage, db.AgentEvent) {
	f.messageUpdateCount++
}

func (f *fakeBroadcaster) BroadcastAgentTask(pgtype.UUID, db.AgentTask) {
	f.taskCount++
}

func (f *fakeBroadcaster) BroadcastAgentEvent(pgtype.UUID, db.AgentEvent) {
	f.eventCount++
}

func (f *fakeBroadcaster) BroadcastAgentMessageDelta(_ pgtype.UUID, delta ProducerStreamDelta) {
	f.deltas = append(f.deltas, delta.Delta)
}

type fakeRuntime struct {
	runningTask          pgtype.UUID
	waitingTask          pgtype.UUID
	succeededTask        pgtype.UUID
	failedTask           pgtype.UUID
	failedCode           string
	assistantText        string
	assistantMessageType string
	assistantRawMessage  []byte
	threadCheckpoint     string
	eventTypes           []string
	appendMessageSeq     int64
	appended             []db.AgentMessage
	updated              []db.AgentMessage
	runningTaskInput     []byte
	releasedSignals      []releasedSignal
	claimedSignals       []db.ProducerPendingSignal
	processedSignals     []pgtype.UUID
}

type releasedSignal struct {
	workspaceID pgtype.UUID
	taskID      pgtype.UUID
	reason      string
}

func (f *fakeRuntime) MarkTaskRunning(_ context.Context, taskID pgtype.UUID) (db.AgentTask, error) {
	f.runningTask = taskID
	return db.AgentTask{ID: taskID, WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), Role: "producer", TaskType: "producer_turn", Status: "running", Input: f.runningTaskInput}, nil
}

func (f *fakeRuntime) MarkTaskSucceeded(_ context.Context, taskID pgtype.UUID, _ []byte) (db.AgentTask, error) {
	f.succeededTask = taskID
	return db.AgentTask{ID: taskID, WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), Role: "producer", TaskType: "producer_turn", Status: "succeeded"}, nil
}

func (f *fakeRuntime) MarkTaskFailed(_ context.Context, taskID pgtype.UUID, code, _ string) (db.AgentTask, error) {
	f.failedTask = taskID
	f.failedCode = code
	return db.AgentTask{ID: taskID, WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), Role: "producer", TaskType: "producer_turn", Status: "failed"}, nil
}

func (f *fakeRuntime) MarkTaskWaitingForUser(_ context.Context, taskID pgtype.UUID) (db.AgentTask, error) {
	f.waitingTask = taskID
	return db.AgentTask{ID: taskID, WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), Role: "producer", TaskType: "producer_turn", Status: "waiting_for_user"}, nil
}

func (f *fakeRuntime) SetThreadCheckpoint(_ context.Context, _ pgtype.UUID, checkpointKey string) (db.AgentThread, error) {
	f.threadCheckpoint = checkpointKey
	return db.AgentThread{CurrentCheckpointKey: pgtype.Text{String: checkpointKey, Valid: checkpointKey != ""}}, nil
}

func (f *fakeRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	if !fakeValidMessageType(params.MessageType) {
		return db.AgentMessage{}, agentruntime.ErrInvalidRequest
	}
	f.assistantMessageType = params.MessageType
	if texts := uimessage.ExtractMarkdownTexts(params.Content); len(texts) > 0 {
		f.assistantText = strings.Join(texts, "\n\n")
	}
	f.assistantRawMessage = params.RawMessage
	f.appendMessageSeq++
	msg := db.AgentMessage{
		ID:          uuidWithByte(byte(9 + f.appendMessageSeq)),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Seq:         f.appendMessageSeq,
		Role:        params.Role,
		MessageType: params.MessageType,
		Content:     params.Content,
		RawMessage:  params.RawMessage,
		TaskID:      params.TaskID,
	}
	f.appended = append(f.appended, msg)
	return msg, nil
}

func fakeValidMessageType(value string) bool {
	switch value {
	case "", "text", "tool_call", "tool_result", "ui_card", "error", "status":
		return true
	default:
		return false
	}
}

func (f *fakeRuntime) UpdateMessage(_ context.Context, params agentruntime.UpdateMessageParams) (db.AgentMessage, error) {
	for index, msg := range f.appended {
		if msg.ID == params.ID {
			msg.Content = params.Content
			msg.RawMessage = params.RawMessage
			msg.EventID = params.EventID
			f.appended[index] = msg
			f.updated = append(f.updated, msg)
			return msg, nil
		}
	}
	return db.AgentMessage{}, errors.New("message not found")
}

func (f *fakeRuntime) appendedMessageTypes() []string {
	out := make([]string, 0, len(f.appended))
	for _, msg := range f.appended {
		out = append(out, msg.MessageType)
	}
	return out
}

func (f *fakeRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.eventTypes = append(f.eventTypes, params.EventType)
	return db.AgentEvent{
		ID:          uuidWithByte(8),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		TaskID:      params.TaskID,
		EventType:   params.EventType,
		SourceRole:  params.SourceRole,
	}, nil
}

func (f *fakeRuntime) ReleaseProducerPendingSignalsForTask(_ context.Context, workspaceID, taskID pgtype.UUID, reason string) ([]db.ProducerPendingSignal, error) {
	f.releasedSignals = append(f.releasedSignals, releasedSignal{workspaceID: workspaceID, taskID: taskID, reason: reason})
	return []db.ProducerPendingSignal{{WorkspaceID: workspaceID, ClaimedByTaskID: taskID, Status: "pending"}}, nil
}

func (f *fakeRuntime) ListClaimedProducerSignalsByTask(context.Context, pgtype.UUID, pgtype.UUID, pgtype.UUID) ([]db.ProducerPendingSignal, error) {
	return f.claimedSignals, nil
}

func (f *fakeRuntime) MarkProducerPendingSignalProcessed(_ context.Context, signalID, _ pgtype.UUID, _ pgtype.UUID) (db.ProducerPendingSignal, error) {
	f.processedSignals = append(f.processedSignals, signalID)
	return db.ProducerPendingSignal{ID: signalID, Status: "processed"}, nil
}

type fakeEinoCheckpointStore struct{}

func (fakeEinoCheckpointStore) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}

func (fakeEinoCheckpointStore) Set(context.Context, string, []byte) error {
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
