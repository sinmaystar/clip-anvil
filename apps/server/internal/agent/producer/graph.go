package producer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/toolloop"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var ErrInvalidGraphConfig = errors.New("invalid producer graph config")

type ContextLoader interface {
	LoadProducerContext(ctx context.Context, input ProducerTurnInput) (ProducerContext, error)
}

type GraphConfig struct {
	Loader             ContextLoader
	Responder          Responder
	NativeToolRegistry *agenttools.NativeRegistry
	SignalRuntime      ProducerSignalRuntime
	CheckPointStore    compose.CheckPointStore
	CompileCallbacks   []compose.GraphCompileCallback
}

type ProducerSignalRuntime interface {
	ClaimProducerPendingSignals(ctx context.Context, params agentruntime.ClaimProducerPendingSignalsParams) ([]db.ProducerPendingSignal, error)
	ListClaimedProducerSignalsByTask(ctx context.Context, workspaceID, producerThreadID, taskID pgtype.UUID) ([]db.ProducerPendingSignal, error)
}

type Graph struct {
	runnable compose.Runnable[ProducerTurnInput, ProducerTurnOutput]
}

const (
	defaultProducerMaxToolCalls = 1000
	producerGraphMaxRunSteps    = 10000
)

type ProducerLoopState struct {
	Context                    ProducerContext
	LastOutput                 ProducerTurnOutput
	LastAssistantMessage       *schema.Message
	LastToolCalls              []schema.ToolCall
	LastToolResults            []*schema.Message
	ToolIterations             int
	ReminderCooldowns          map[string]int
	NewlyClaimedSignals        int
	SignalReminderCount        int
	SignalReminderKey          string
	FinalizeAfterAsyncDispatch bool
}

func NewGraph(config GraphConfig) (*Graph, error) {
	if config.Loader == nil || config.Responder == nil || config.NativeToolRegistry == nil {
		return nil, ErrInvalidGraphConfig
	}
	return newExplicitToolLoopGraph(config)
}

func newExplicitToolLoopGraph(config GraphConfig) (*Graph, error) {
	stateStore := newProducerLoopToolStateStore()
	toolNode, toolInfos, err := producerToolNodeForConfig(context.Background(), config, stateStore)
	if err != nil {
		return nil, err
	}
	g := compose.NewGraph[ProducerTurnInput, ProducerTurnOutput]()
	if err := g.AddLambdaNode("load_context", compose.InvokableLambda(func(ctx context.Context, input ProducerTurnInput) (ProducerContext, error) {
		return config.Loader.LoadProducerContext(ctx, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("prepare_turn_state", compose.InvokableLambda(func(_ context.Context, input ProducerContext) (ProducerLoopState, error) {
		input.ToolInfos = toolInfos
		return ProducerLoopState{Context: input}, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("before_model", compose.InvokableLambda(func(ctx context.Context, state ProducerLoopState) (ProducerLoopState, error) {
		return applyProducerBeforeModel(ctx, config.SignalRuntime, state)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("call_model", compose.InvokableLambda(func(ctx context.Context, state ProducerLoopState) (ProducerLoopState, error) {
		out, err := config.Responder.Respond(ctx, state.Context)
		if err != nil {
			return ProducerLoopState{}, err
		}
		state.LastOutput = out
		state.LastAssistantMessage = out.ModelMessage
		state.LastToolCalls = append([]schema.ToolCall(nil), nativeToolCalls(out.ModelMessage)...)
		state.LastToolResults = nil
		return state, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("prepare_tool_message", compose.InvokableLambda(func(_ context.Context, state ProducerLoopState) (*schema.Message, error) {
		return prepareProducerToolMessage(stateStore, state)
	})); err != nil {
		return nil, err
	}
	if err := g.AddToolsNode("execute_tools", toolNode); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("append_tool_results", compose.InvokableLambda(func(_ context.Context, toolResults []*schema.Message) (ProducerLoopState, error) {
		state, err := producerLoopStateForToolResults(stateStore, toolResults)
		if err != nil {
			return ProducerLoopState{}, err
		}
		if hasAsyncCraftsmanDispatchResult(state.LastToolResults) {
			state.FinalizeAfterAsyncDispatch = true
			state.LastOutput = asyncCraftsmanDispatchOutput(state.LastOutput)
		}
		appendNativeSameTurnMessages(&state.Context, state.LastOutput, state.LastAssistantMessage, state.LastToolResults)
		state.ToolIterations++
		state.LastToolResults = nil
		return state, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("check_signals_before_finalize", compose.InvokableLambda(func(ctx context.Context, state ProducerLoopState) (ProducerLoopState, error) {
		return applyProducerSignalCheck(ctx, config.SignalRuntime, state)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("finalize_response", compose.InvokableLambda(func(_ context.Context, state ProducerLoopState) (ProducerTurnOutput, error) {
		out, err := finalizeProducerOutput(state.LastOutput)
		if err != nil {
			return ProducerTurnOutput{}, err
		}
		out.SameTurnMessages = append([]ProducerSameTurnMessage(nil), state.Context.SameTurnMessages...)
		return out, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("fail_turn", compose.InvokableLambda(func(_ context.Context, _ ProducerLoopState) (ProducerTurnOutput, error) {
		return ProducerTurnOutput{}, NewAgentError("agent_tool_loop_exhausted", "producer exceeded max tool calls")
	})); err != nil {
		return nil, err
	}
	if err := g.AddEdge(compose.START, "load_context"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("load_context", "prepare_turn_state"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("prepare_turn_state", "before_model"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("before_model", "call_model"); err != nil {
		return nil, err
	}
	if err := g.AddBranch("call_model", compose.NewGraphBranch(routeProducerModelOutput(), map[string]bool{
		"prepare_tool_message":          true,
		"check_signals_before_finalize": true,
		"fail_turn":                     true,
	})); err != nil {
		return nil, err
	}
	if err := g.AddEdge("prepare_tool_message", "execute_tools"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("execute_tools", "append_tool_results"); err != nil {
		return nil, err
	}
	if err := g.AddBranch("append_tool_results", compose.NewGraphBranch(routeProducerAfterToolResults(), map[string]bool{
		"before_model":      true,
		"finalize_response": true,
	})); err != nil {
		return nil, err
	}
	if err := g.AddBranch("check_signals_before_finalize", compose.NewGraphBranch(routeProducerFinalizeCheck(), map[string]bool{
		"call_model":        true,
		"finalize_response": true,
	})); err != nil {
		return nil, err
	}
	if err := g.AddEdge("finalize_response", compose.END); err != nil {
		return nil, err
	}
	if err := g.AddEdge("fail_turn", compose.END); err != nil {
		return nil, err
	}

	return compileProducerGraph(g, config)
}

func applyProducerBeforeModel(ctx context.Context, signalRuntime ProducerSignalRuntime, state ProducerLoopState) (ProducerLoopState, error) {
	reminderState := toolloop.BeforeModel(producerLoopMessages(state.Context), state.ToolIterations, state.ReminderCooldowns, toolloop.DefaultConfig())
	state.ReminderCooldowns = reminderState.Cooldowns
	state.Context.PendingReminders = reminderState.PendingReminders
	if state.ToolIterations > 0 {
		return state, nil
	}
	signalReminders, newlyClaimed, err := producerSignalReminders(ctx, signalRuntime, state.Context.Input)
	if err != nil {
		return ProducerLoopState{}, err
	}
	state.NewlyClaimedSignals = newlyClaimed
	state.SignalReminderCount = newlyClaimed
	state.SignalReminderKey = signalReminderKey(signalReminders)
	state.Context.PendingReminders = append(state.Context.PendingReminders, signalReminders...)
	return state, nil
}

func applyProducerSignalCheck(ctx context.Context, signalRuntime ProducerSignalRuntime, state ProducerLoopState) (ProducerLoopState, error) {
	signalReminders, newlyClaimed, err := producerSignalReminders(ctx, signalRuntime, state.Context.Input)
	if err != nil {
		return ProducerLoopState{}, err
	}
	state.NewlyClaimedSignals = newlyClaimed
	state.SignalReminderCount = newlyClaimed
	state.SignalReminderKey = signalReminderKey(signalReminders)
	state.Context.PendingReminders = signalReminders
	return state, nil
}

func producerSignalReminders(ctx context.Context, runtime ProducerSignalRuntime, input ProducerTurnInput) ([]string, int, error) {
	if runtime == nil || !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid {
		return nil, 0, nil
	}
	signals, err := runtime.ClaimProducerPendingSignals(ctx, agentruntime.ClaimProducerPendingSignalsParams{
		WorkspaceID:       input.WorkspaceID,
		ProducerThreadID:  input.ThreadID,
		ClaimedByTaskID:   input.TaskID,
		Limit:             20,
		StaleAfterSeconds: 600,
	})
	if err != nil {
		return nil, 0, err
	}
	if len(signals) == 0 {
		return nil, 0, nil
	}
	return []string{formatProducerSignalReminder(signals)}, len(signals), nil
}

func signalReminderKey(reminders []string) string {
	return strings.Join(reminders, "\n")
}

func formatProducerSignalReminder(signals []db.ProducerPendingSignal) string {
	lines := []string{
		"<system-reminder>",
		fmt.Sprintf("你有 %d 个待处理 Producer signal。", len(signals)),
		"这些 signal 是工程事件队列，不是普通用户需求；请读取项目上下文，然后按业务优先级处理。",
		"处理 craftsman_render_plan_ready 时，应针对每条 signal 指定的 render_plan_id 调用 decide_render_plan；如果有多条 RenderPlan，请使用 decisions 批量参数一次提交每条的 accept/reject，或先派 Reviewer；不要只处理列表中的第一条。",
		"处理 worker_generation_completed 时，应读取项目上下文确认 RenderPlan、generation_job、artifact_version 和画布产物状态；这是信息型完成通知，通常不需要 accept/reject。",
		"处理 review_completed 时，应读取项目上下文确认 review_record、artifact_issue 和 retry_recommendation；这是 Reviewer 结果通知，Producer 需要决定接受、修复、重生成或请求用户确认。",
	}
	for i, signal := range signals {
		lines = append(lines, fmt.Sprintf("%d. %s: scope=%s/%s render_plan_id=%s target_phase=%s status=%s review_record_id=%s verdict=%s generation_job_id=%s artifact_version_id=%s source_task=%s",
			i+1,
			strings.TrimSpace(signal.SignalType),
			strings.TrimSpace(signal.ScopeType),
			uuidString(signal.ScopeID),
			uuidString(signal.RenderPlanID),
			signalPayloadString(signal.Payload, "target_phase"),
			signalPayloadString(signal.Payload, "render_plan_status"),
			signalPayloadString(signal.Payload, "review_record_id"),
			signalPayloadString(signal.Payload, "verdict"),
			signalPayloadString(signal.Payload, "generation_job_id"),
			signalPayloadString(signal.Payload, "artifact_version_id"),
			uuidString(signal.SourceTaskID),
		))
	}
	lines = append(lines, "</system-reminder>")
	return strings.Join(lines, "\n")
}

func signalPayloadString(raw []byte, key string) string {
	if len(raw) == 0 || strings.TrimSpace(key) == "" {
		return ""
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return stringFromAny(payload[key])
}

func producerLoopMessages(context ProducerContext) []toolloop.Message {
	out := make([]toolloop.Message, 0, len(context.Messages)+len(context.SameTurnMessages))
	for _, message := range context.Messages {
		out = append(out, toolloop.Message{
			Role:        strings.TrimSpace(message.Role),
			MessageType: strings.TrimSpace(message.MessageType),
			ToolName:    toolNameFromRaw(message.RawMessage),
		})
	}
	for _, message := range context.SameTurnMessages {
		out = append(out, toolloop.Message{
			Role:        strings.TrimSpace(message.Role),
			MessageType: strings.TrimSpace(message.MessageType),
			ToolName:    strings.TrimSpace(message.ToolName),
		})
	}
	return out
}

func compileProducerGraph(g *compose.Graph[ProducerTurnInput, ProducerTurnOutput], config GraphConfig) (*Graph, error) {
	compileOptions := []compose.GraphCompileOption{
		compose.WithGraphName("producer_turn"),
		compose.WithMaxRunSteps(producerGraphMaxRunSteps),
	}
	if config.CheckPointStore != nil {
		compileOptions = append(compileOptions, compose.WithCheckPointStore(config.CheckPointStore))
	}
	if len(config.CompileCallbacks) > 0 {
		compileOptions = append(compileOptions, compose.WithGraphCompileCallbacks(config.CompileCallbacks...))
	}
	runnable, err := g.Compile(context.Background(), compileOptions...)
	if err != nil {
		return nil, err
	}
	return &Graph{runnable: runnable}, nil
}

func routeProducerModelOutput() compose.GraphBranchCondition[ProducerLoopState] {
	return func(_ context.Context, state ProducerLoopState) (string, error) {
		if len(nativeToolCalls(state.LastAssistantMessage)) > 0 {
			if state.ToolIterations >= maxProducerToolCalls(state.Context.Input.MaxToolCalls) {
				return "fail_turn", nil
			}
			return "prepare_tool_message", nil
		}
		return "check_signals_before_finalize", nil
	}
}

func routeProducerAfterToolResults() compose.GraphBranchCondition[ProducerLoopState] {
	return func(_ context.Context, state ProducerLoopState) (string, error) {
		if state.FinalizeAfterAsyncDispatch {
			return "finalize_response", nil
		}
		return "before_model", nil
	}
}

func routeProducerFinalizeCheck() compose.GraphBranchCondition[ProducerLoopState] {
	return func(_ context.Context, state ProducerLoopState) (string, error) {
		if state.NewlyClaimedSignals > 0 {
			return "call_model", nil
		}
		return "finalize_response", nil
	}
}

func hasAsyncCraftsmanDispatchResult(toolResults []*schema.Message) bool {
	for _, result := range toolResults {
		if result == nil || strings.TrimSpace(result.ToolName) != "dispatch_craftsman" {
			continue
		}
		content := strings.TrimSpace(result.Content)
		if strings.Contains(content, "任务已排队") ||
			strings.Contains(content, "任务加入队列") ||
			strings.Contains(content, "已存在同一 target_phase 的 Craftsman 任务正在排队或运行") {
			return true
		}
	}
	return false
}

func asyncCraftsmanDispatchOutput(input ProducerTurnOutput) ProducerTurnOutput {
	input.AssistantText = "已派发 Craftsman 生成任务，后续会由工程事件在 RenderPlan、预览图或视频状态变化后继续通知我处理。当前这一步不需要继续同步轮询，请在画布中观察生产进度，或稍后追问指定分镜状态。"
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}
	input.Metadata["finalized_after_async_craftsman_dispatch"] = true
	return input
}

func maxProducerToolCalls(value int) int {
	if value <= 0 {
		return defaultProducerMaxToolCalls
	}
	return value
}

func finalizeProducerOutput(input ProducerTurnOutput) (ProducerTurnOutput, error) {
	input.AssistantText = strings.TrimSpace(input.AssistantText)
	if input.AssistantText == "" {
		if input.Metadata == nil {
			input.Metadata = map[string]any{}
		}
		if strings.TrimSpace(stringFromMap(input.Metadata, "reasoning_content")) != "" {
			input.AssistantText = "ClipAnvil 已完成思考，但模型没有返回可展示的回复。请切换到较低思考深度，或简化需求后重试。"
			input.Metadata["empty_content_fallback"] = true
			return input, nil
		}
		input.AssistantText = "ClipAnvil 这次没有收到可展示的模型回复。我会保留当前项目状态，你可以直接继续追问进展，或指定要查看/调整的分镜、RenderPlan、预览图或视频。"
		input.Metadata["empty_content_fallback"] = true
		input.Metadata["empty_content_without_reasoning"] = true
		return input, nil
	}
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}
	return input, nil
}

func prepareProducerToolMessage(stateStore *producerLoopToolStateStore, state ProducerLoopState) (*schema.Message, error) {
	assistantMessage := state.LastAssistantMessage
	if assistantMessage == nil && len(state.LastToolCalls) > 0 {
		assistantMessage = &schema.Message{Role: schema.Assistant, ToolCalls: append([]schema.ToolCall(nil), state.LastToolCalls...)}
	}
	if assistantMessage == nil {
		return nil, NewAgentError("agent_tool_call_missing", "producer model returned no tool call message")
	}
	cleanMessage := cloneToolCallMessage(assistantMessage)
	state.LastAssistantMessage = cloneToolCallMessage(cleanMessage)
	state.LastToolCalls = append([]schema.ToolCall(nil), cleanMessage.ToolCalls...)
	state.LastToolResults = nil

	stateKey := uuid.NewString()
	stateStore.rememberKey(stateKey, state)
	for _, call := range cleanMessage.ToolCalls {
		stateStore.rememberCall(call.ID, state)
	}

	messageForToolNode := cloneToolCallMessage(cleanMessage)
	for i := range messageForToolNode.ToolCalls {
		args := toolCallArgumentsMap(messageForToolNode.ToolCalls[i])
		args[producerLoopStateArgumentKey] = stateKey
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		messageForToolNode.ToolCalls[i].Function.Arguments = string(raw)
	}
	return messageForToolNode, nil
}

func producerLoopStateForToolResults(stateStore *producerLoopToolStateStore, toolResults []*schema.Message) (ProducerLoopState, error) {
	for _, result := range toolResults {
		if result == nil {
			continue
		}
		if state, ok := stateStore.stateByCall(result.ToolCallID); ok {
			state.LastToolResults = toolResults
			for _, completed := range toolResults {
				if completed != nil {
					stateStore.forgetCall(completed.ToolCallID)
				}
			}
			return state, nil
		}
	}
	return ProducerLoopState{}, NewAgentError("agent_tool_state_missing", "producer tool result state is missing")
}

func stringFromMap(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func toolNameFromRaw(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return stringFromAny(payload["tool_name"])
}

func nativeToolCalls(message *schema.Message) []schema.ToolCall {
	if message == nil {
		return nil
	}
	return message.ToolCalls
}

func cloneToolCallMessage(message *schema.Message) *schema.Message {
	cloned := *message
	cloned.Role = schema.Assistant
	cloned.ToolCalls = append([]schema.ToolCall(nil), message.ToolCalls...)
	for i := range cloned.ToolCalls {
		if strings.TrimSpace(cloned.ToolCalls[i].ID) == "" {
			cloned.ToolCalls[i].ID = uuid.NewString()
		}
		if strings.TrimSpace(cloned.ToolCalls[i].Type) == "" {
			cloned.ToolCalls[i].Type = "function"
		}
	}
	return &cloned
}

func appendNativeSameTurnMessages(input *ProducerContext, out ProducerTurnOutput, assistant *schema.Message, toolResults []*schema.Message) {
	for _, call := range assistant.ToolCalls {
		input.SameTurnMessages = append(input.SameTurnMessages, ProducerSameTurnMessage{
			Role:             "assistant",
			MessageType:      "tool_call",
			Content:          out.AssistantText,
			ReasoningContent: stringFromMap(out.Metadata, "reasoning_content"),
			ToolCallID:       call.ID,
			ToolName:         call.Function.Name,
			ToolArguments:    toolCallArgumentsMap(call),
		})
	}
	for _, result := range toolResults {
		if result == nil {
			continue
		}
		input.SameTurnMessages = append(input.SameTurnMessages, ProducerSameTurnMessage{
			Role:        "tool",
			MessageType: "tool_result",
			Content:     result.Content,
			ToolCallID:  result.ToolCallID,
			ToolName:    result.ToolName,
		})
	}
}

func toolCallArgumentsMap(call schema.ToolCall) map[string]any {
	args := map[string]any{}
	if strings.TrimSpace(call.Function.Arguments) == "" {
		return args
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return map[string]any{"_raw": call.Function.Arguments}
	}
	return args
}

func (g *Graph) Run(ctx context.Context, input ProducerTurnInput, options ...agenteino.RunOptions) (ProducerTurnOutput, error) {
	runOptions := agenteino.RunOptions{}
	if len(options) > 0 {
		runOptions = options[0]
	}
	ctx, callOptions := agenteino.ApplyRunOptions(ctx, runOptions)
	return g.runnable.Invoke(ctx, input, callOptions...)
}
