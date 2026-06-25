package producer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
)

var ErrInvalidGraphConfig = errors.New("invalid producer graph config")

type ContextLoader interface {
	LoadProducerContext(ctx context.Context, input ProducerTurnInput) (ProducerContext, error)
}

type GraphConfig struct {
	Mode               ProducerGraphMode
	Loader             ContextLoader
	Responder          Responder
	NativeToolRegistry *agenttools.NativeRegistry
	CheckPointStore    compose.CheckPointStore
	CompileCallbacks   []compose.GraphCompileCallback
}

type Graph struct {
	runnable compose.Runnable[ProducerTurnInput, ProducerTurnOutput]
}

type ProducerGraphMode string

const (
	ProducerGraphModeInlineDraft      ProducerGraphMode = "inline_draft"
	ProducerGraphModeExplicitToolLoop ProducerGraphMode = "explicit_tool_loop"
	defaultProducerMaxToolCalls                         = 1000
	producerGraphMaxRunSteps                            = 10000
)

type ProducerLoopState struct {
	Context              ProducerContext
	LastOutput           ProducerTurnOutput
	LastAssistantMessage *schema.Message
	LastToolCalls        []schema.ToolCall
	LastToolResults      []*schema.Message
	ToolIterations       int
}

func NewGraph(config GraphConfig) (*Graph, error) {
	if config.Loader == nil || config.Responder == nil {
		return nil, ErrInvalidGraphConfig
	}
	if config.Mode == ProducerGraphModeExplicitToolLoop {
		return newExplicitToolLoopGraph(config)
	}
	return newInlineDraftGraph(config)
}

func newInlineDraftGraph(config GraphConfig) (*Graph, error) {
	g := compose.NewGraph[ProducerTurnInput, ProducerTurnOutput]()
	if err := g.AddLambdaNode("load_context", compose.InvokableLambda(func(ctx context.Context, input ProducerTurnInput) (ProducerContext, error) {
		return config.Loader.LoadProducerContext(ctx, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("draft_response", compose.InvokableLambda(func(ctx context.Context, input ProducerContext) (ProducerTurnOutput, error) {
		return config.Responder.Respond(ctx, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("finalize_response", compose.InvokableLambda(func(_ context.Context, input ProducerTurnOutput) (ProducerTurnOutput, error) {
		return finalizeProducerOutput(input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddEdge(compose.START, "load_context"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("load_context", "draft_response"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("draft_response", "finalize_response"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("finalize_response", compose.END); err != nil {
		return nil, err
	}

	return compileProducerGraph(g, config)
}

func newExplicitToolLoopGraph(config GraphConfig) (*Graph, error) {
	if config.NativeToolRegistry == nil {
		return nil, NewAgentError("agent_native_tool_registry_missing", "producer native tool registry is not configured")
	}
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
		appendNativeSameTurnMessages(&state.Context, state.LastOutput, state.LastAssistantMessage, state.LastToolResults)
		state.ToolIterations++
		state.LastToolResults = nil
		return state, nil
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
	if err := g.AddEdge("prepare_turn_state", "call_model"); err != nil {
		return nil, err
	}
	if err := g.AddBranch("call_model", compose.NewGraphBranch(routeProducerModelOutput(), map[string]bool{
		"prepare_tool_message": true,
		"finalize_response":    true,
		"fail_turn":            true,
	})); err != nil {
		return nil, err
	}
	if err := g.AddEdge("prepare_tool_message", "execute_tools"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("execute_tools", "append_tool_results"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("append_tool_results", "call_model"); err != nil {
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
		return "finalize_response", nil
	}
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
