package craftsman

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
)

const (
	craftsmanLoopStateArgumentKey = "_clipanvil_craftsman_loop_state_key"
	defaultCraftsmanMaxToolCalls  = 1000
	craftsmanGraphMaxRunSteps     = 10000
)

type craftsmanLoopState struct {
	Context              Context
	LastOutput           CraftsmanTurnOutput
	LastAssistantMessage *schema.Message
	LastToolCalls        []schema.ToolCall
	LastToolResults      []*schema.Message
	ToolIterations       int
}

type craftsmanLoopToolStateStore struct {
	mu     sync.Mutex
	byKey  map[string]craftsmanLoopState
	byCall map[string]string
}

func newCraftsmanLoopToolStateStore() *craftsmanLoopToolStateStore {
	return &craftsmanLoopToolStateStore{
		byKey:  map[string]craftsmanLoopState{},
		byCall: map[string]string{},
	}
}

func (s *craftsmanLoopToolStateStore) rememberKey(key string, state craftsmanLoopState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey[key] = state
}

func (s *craftsmanLoopToolStateStore) rememberCall(callID string, state craftsmanLoopState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := uuid.NewString()
	s.byKey[key] = state
	s.byCall[callID] = key
}

func (s *craftsmanLoopToolStateStore) rememberCallWithKey(callID string, key string, state craftsmanLoopState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey[key] = state
	s.byCall[callID] = key
}

func (s *craftsmanLoopToolStateStore) stateByKey(key string) (craftsmanLoopState, bool) {
	if s == nil {
		return craftsmanLoopState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.byKey[key]
	return state, ok
}

func (s *craftsmanLoopToolStateStore) stateByCall(callID string) (craftsmanLoopState, bool) {
	if s == nil {
		return craftsmanLoopState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byCall[callID]
	if !ok {
		return craftsmanLoopState{}, false
	}
	state, ok := s.byKey[key]
	return state, ok
}

func (s *craftsmanLoopToolStateStore) forgetCall(callID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byCall, callID)
}

func newNativeToolLoopGraph(config GraphConfig) (*Graph, error) {
	stateStore := newCraftsmanLoopToolStateStore()
	toolInfos, err := config.NativeToolRegistry.ToolInfos(context.Background())
	if err != nil {
		return nil, err
	}
	toolNode, err := compose.NewToolNode(context.Background(), &compose.ToolsNodeConfig{
		Tools:               config.NativeToolRegistry.BaseTools(),
		ExecuteSequentially: true,
		ToolCallMiddlewares: []compose.ToolMiddleware{nativeCraftsmanToolRuntimeMiddleware(stateStore)},
	})
	if err != nil {
		return nil, err
	}
	g := compose.NewGraph[GraphInput, GraphOutput]()
	if err := g.AddLambdaNode("load_context", compose.InvokableLambda(func(ctx context.Context, input GraphInput) (Context, error) {
		craftsmanContext, err := config.Loader.Load(ctx, input)
		if err != nil {
			return Context{}, err
		}
		craftsmanContext.ToolInfos = toolInfos
		return craftsmanContext, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("prepare_turn_state", compose.InvokableLambda(func(_ context.Context, input Context) (craftsmanLoopState, error) {
		return craftsmanLoopState{Context: input}, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("call_model", compose.InvokableLambda(func(ctx context.Context, state craftsmanLoopState) (craftsmanLoopState, error) {
		out, err := config.ToolResponder.Respond(ctx, state.Context)
		if err != nil {
			return craftsmanLoopState{}, err
		}
		state.LastOutput = out
		state.LastAssistantMessage = out.ModelMessage
		state.LastToolCalls = append([]schema.ToolCall(nil), nativeToolCalls(out.ModelMessage)...)
		state.LastToolResults = nil
		return state, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("prepare_tool_message", compose.InvokableLambda(func(_ context.Context, state craftsmanLoopState) (*schema.Message, error) {
		return prepareCraftsmanToolMessage(stateStore, state)
	})); err != nil {
		return nil, err
	}
	if err := g.AddToolsNode("execute_tools", toolNode); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("append_tool_results", compose.InvokableLambda(func(_ context.Context, toolResults []*schema.Message) (craftsmanLoopState, error) {
		state, err := craftsmanLoopStateForToolResults(stateStore, toolResults)
		if err != nil {
			return craftsmanLoopState{}, err
		}
		appendCraftsmanSameTurnMessages(&state.Context, state.LastOutput, state.LastAssistantMessage, toolResults)
		state.ToolIterations++
		state.LastToolResults = nil
		return state, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("finalize_response", compose.InvokableLambda(func(_ context.Context, state craftsmanLoopState) (GraphOutput, error) {
		out, err := finalizeCraftsmanOutput(state.LastOutput)
		if err != nil {
			return GraphOutput{}, err
		}
		out.SameTurnMessages = append([]CraftsmanSameTurnMessage(nil), state.Context.SameTurnMessages...)
		return out, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("fail_turn", compose.InvokableLambda(func(_ context.Context, _ craftsmanLoopState) (GraphOutput, error) {
		return GraphOutput{}, errors.New("craftsman exceeded max tool calls")
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
	if err := g.AddBranch("call_model", compose.NewGraphBranch(routeCraftsmanModelOutput(), map[string]bool{
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
	return compileCraftsmanGraph(g, config)
}

func compileCraftsmanGraph(g *compose.Graph[GraphInput, GraphOutput], config GraphConfig) (*Graph, error) {
	compileOptions := []compose.GraphCompileOption{
		compose.WithGraphName("craftsman_render_plan"),
		compose.WithMaxRunSteps(craftsmanGraphMaxRunSteps),
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

func routeCraftsmanModelOutput() compose.GraphBranchCondition[craftsmanLoopState] {
	return func(_ context.Context, state craftsmanLoopState) (string, error) {
		if len(nativeToolCalls(state.LastAssistantMessage)) > 0 {
			if state.ToolIterations >= maxCraftsmanToolCalls() {
				return "fail_turn", nil
			}
			return "prepare_tool_message", nil
		}
		return "finalize_response", nil
	}
}

func maxCraftsmanToolCalls() int {
	return defaultCraftsmanMaxToolCalls
}

func prepareCraftsmanToolMessage(stateStore *craftsmanLoopToolStateStore, state craftsmanLoopState) (*schema.Message, error) {
	assistantMessage := state.LastAssistantMessage
	if assistantMessage == nil && len(state.LastToolCalls) > 0 {
		assistantMessage = &schema.Message{Role: schema.Assistant, ToolCalls: append([]schema.ToolCall(nil), state.LastToolCalls...)}
	}
	if assistantMessage == nil {
		return nil, errors.New("craftsman model returned no tool call message")
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
		args[craftsmanLoopStateArgumentKey] = stateKey
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		messageForToolNode.ToolCalls[i].Function.Arguments = string(raw)
	}
	return messageForToolNode, nil
}

func craftsmanLoopStateForToolResults(stateStore *craftsmanLoopToolStateStore, toolResults []*schema.Message) (craftsmanLoopState, error) {
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
	return craftsmanLoopState{}, errors.New("craftsman tool result state is missing")
}

func nativeCraftsmanToolRuntimeMiddleware(stateStore *craftsmanLoopToolStateStore) compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				if input == nil {
					return next(ctx, input)
				}
				args := map[string]any{}
				if input.Arguments != "" {
					_ = json.Unmarshal([]byte(input.Arguments), &args)
				}
				if stateKey, _ := args[craftsmanLoopStateArgumentKey].(string); stateKey != "" && stateStore != nil {
					delete(args, craftsmanLoopStateArgumentKey)
					if raw, err := json.Marshal(args); err == nil {
						input.Arguments = string(raw)
					}
					if state, ok := stateStore.stateByKey(stateKey); ok {
						stateStore.rememberCallWithKey(input.CallID, stateKey, state)
						ctx = agenttools.WithNativeRuntimeContext(ctx, agenttools.NativeRuntimeContext{
							WorkspaceID:     state.Context.Input.WorkspaceID,
							ThreadID:        state.Context.Input.ThreadID,
							TaskID:          state.Context.Input.TaskID,
							ToolCallID:      input.CallID,
							ExecutionPolicy: state.Context.Input.ExecutionPolicy,
						})
					}
				}
				runtime, _ := agenttools.NativeRuntimeFromContext(ctx)
				if strings.TrimSpace(runtime.ToolCallID) == "" {
					runtime.ToolCallID = input.CallID
				}
				if sink, ok := agenttools.NativeToolTraceSinkFromContext(ctx); ok && sink != nil {
					if err := sink.NativeToolCallStarted(ctx, runtime, agenttools.NativeToolTrace{
						ToolName:  input.Name,
						Arguments: args,
					}); err != nil {
						return nil, err
					}
				}
				out, err := next(ctx, input)
				trace := agenttools.NativeToolTrace{ToolName: input.Name}
				if out != nil {
					trace.Result = out.Result
				}
				if err != nil {
					trace.Error = err.Error()
				}
				if _, interrupted := compose.IsInterruptRerunError(err); interrupted {
					return out, err
				}
				if _, interrupted := compose.ExtractInterruptInfo(err); interrupted {
					return out, err
				}
				if sink, ok := agenttools.NativeToolTraceSinkFromContext(ctx); ok && sink != nil {
					if traceErr := sink.NativeToolCallCompleted(ctx, runtime, trace); traceErr != nil {
						if err != nil {
							return out, err
						}
						return out, traceErr
					}
				}
				return out, err
			}
		},
	}
}

func finalizeCraftsmanOutput(input CraftsmanTurnOutput) (GraphOutput, error) {
	text := strings.TrimSpace(input.AssistantText)
	if text == "" {
		text = "Craftsman 已完成 RenderPlan 工具调用。"
	}
	metadata := input.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	return GraphOutput{AssistantText: text, Metadata: metadata}, nil
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

func appendCraftsmanSameTurnMessages(input *Context, out CraftsmanTurnOutput, assistant *schema.Message, toolResults []*schema.Message) {
	if input == nil || assistant == nil {
		return
	}
	for _, call := range assistant.ToolCalls {
		input.SameTurnMessages = append(input.SameTurnMessages, CraftsmanSameTurnMessage{
			Role:          "assistant",
			MessageType:   "tool_call",
			Content:       out.AssistantText,
			ToolCallID:    call.ID,
			ToolName:      call.Function.Name,
			ToolArguments: toolCallArgumentsMap(call),
		})
	}
	for _, result := range toolResults {
		if result == nil {
			continue
		}
		input.SameTurnMessages = append(input.SameTurnMessages, CraftsmanSameTurnMessage{
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

var _ = agenteino.CheckpointKey
