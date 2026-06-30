package composer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	"github.com/sinmaystar/clip-anvil/internal/agent/toolloop"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/production"
)

const (
	composerLoopStateArgumentKey = "_clipanvil_composer_loop_state_key"
	defaultComposerMaxToolCalls  = 1000
	composerGraphMaxRunSteps     = 10000
)

type GraphConfig struct {
	Loader             ContextLoader
	Runtime            Runtime
	Store              Store
	Production         ProductionSubmitter
	ToolResponder      ToolResponder
	NativeToolRegistry *agenttools.NativeRegistry
	Broadcaster        NodeBroadcaster
	CheckPointStore    compose.CheckPointStore
	CompileCallbacks   []compose.GraphCompileCallback
}

type Graph struct {
	runnable compose.Runnable[GraphInput, GraphOutput]
}

type composerLoopState struct {
	Context              Context
	LastOutput           ComposerTurnOutput
	LastAssistantMessage *schema.Message
	LastToolCalls        []schema.ToolCall
	LastToolResults      []*schema.Message
	ToolIterations       int
	ReminderCooldowns    map[string]int
}

type composerLoopToolStateStore struct {
	mu     sync.Mutex
	byKey  map[string]composerLoopState
	byCall map[string]string
}

func NewGraph(config GraphConfig) (*Graph, error) {
	if config.Loader == nil || config.ToolResponder == nil || config.NativeToolRegistry == nil {
		return nil, ErrInvalidConfig
	}
	stateStore := newComposerLoopToolStateStore()
	toolInfos, err := config.NativeToolRegistry.ToolInfos(context.Background())
	if err != nil {
		return nil, err
	}
	toolNode, err := compose.NewToolNode(context.Background(), &compose.ToolsNodeConfig{
		Tools:               config.NativeToolRegistry.BaseTools(),
		ExecuteSequentially: true,
		ToolCallMiddlewares: []compose.ToolMiddleware{nativeComposerToolRuntimeMiddleware(stateStore)},
	})
	if err != nil {
		return nil, err
	}
	g := compose.NewGraph[GraphInput, GraphOutput]()
	if err := g.AddLambdaNode("load_context", compose.InvokableLambda(func(ctx context.Context, input GraphInput) (Context, error) {
		req := Request{
			WorkspaceID:            input.WorkspaceID,
			ThreadID:               input.ThreadID,
			TaskID:                 input.TaskID,
			SourceStoryboardNodeID: sourceStoryboardNodeID(input),
			Instructions:           strings.TrimSpace(input.Input.Instructions),
		}
		composerContext, err := config.Loader.LoadCompositionContext(ctx, req)
		if err != nil {
			return Context{}, err
		}
		composerContext.Input = input
		composerContext.WorkspaceID = input.WorkspaceID
		composerContext.SourceStoryboardNodeID = req.SourceStoryboardNodeID
		composerContext.ToolInfos = toolInfos
		return composerContext, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("prepare_turn_state", compose.InvokableLambda(func(_ context.Context, input Context) (composerLoopState, error) {
		return composerLoopState{Context: input}, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("before_model", compose.InvokableLambda(func(_ context.Context, state composerLoopState) (composerLoopState, error) {
		return applyComposerBeforeModel(state), nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("call_model", compose.InvokableLambda(func(ctx context.Context, state composerLoopState) (composerLoopState, error) {
		out, err := config.ToolResponder.Respond(ctx, state.Context)
		if err != nil {
			return composerLoopState{}, err
		}
		state.LastOutput = out
		state.LastAssistantMessage = out.ModelMessage
		state.LastToolCalls = append([]schema.ToolCall(nil), nativeToolCalls(out.ModelMessage)...)
		state.LastToolResults = nil
		return state, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("prepare_tool_message", compose.InvokableLambda(func(_ context.Context, state composerLoopState) (*schema.Message, error) {
		return prepareComposerToolMessage(stateStore, state)
	})); err != nil {
		return nil, err
	}
	if err := g.AddToolsNode("execute_tools", toolNode); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("append_tool_results", compose.InvokableLambda(func(_ context.Context, toolResults []*schema.Message) (composerLoopState, error) {
		state, err := composerLoopStateForToolResults(stateStore, toolResults)
		if err != nil {
			return composerLoopState{}, err
		}
		appendComposerSameTurnMessages(&state.Context, state.LastOutput, state.LastAssistantMessage, toolResults)
		state.ToolIterations++
		state.LastToolResults = nil
		return state, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("finalize_response", compose.InvokableLambda(func(_ context.Context, state composerLoopState) (GraphOutput, error) {
		out := finalizeComposerOutput(state)
		return GraphOutput{Output: out, CheckpointKey: "composer_timeline", AssistantText: state.LastOutput.AssistantText}, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("fail_turn", compose.InvokableLambda(func(_ context.Context, _ composerLoopState) (GraphOutput, error) {
		return GraphOutput{}, errors.New("composer exceeded max tool calls")
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
	if err := g.AddBranch("call_model", compose.NewGraphBranch(routeComposerModelOutput(), map[string]bool{
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
	if err := g.AddEdge("append_tool_results", "before_model"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("finalize_response", compose.END); err != nil {
		return nil, err
	}
	if err := g.AddEdge("fail_turn", compose.END); err != nil {
		return nil, err
	}
	compileOptions := []compose.GraphCompileOption{
		compose.WithGraphName("composer_timeline"),
		compose.WithMaxRunSteps(composerGraphMaxRunSteps),
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

func (g *Graph) Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error) {
	if g == nil || g.runnable == nil {
		return GraphOutput{}, ErrInvalidConfig
	}
	runOptions := agenteino.RunOptions{}
	if len(options) > 0 {
		runOptions = options[0]
	}
	ctx, callOptions := agenteino.ApplyRunOptions(ctx, runOptions)
	return g.runnable.Invoke(ctx, input, callOptions...)
}

func newComposerLoopToolStateStore() *composerLoopToolStateStore {
	return &composerLoopToolStateStore{byKey: map[string]composerLoopState{}, byCall: map[string]string{}}
}

func (s *composerLoopToolStateStore) rememberKey(key string, state composerLoopState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey[key] = state
}

func (s *composerLoopToolStateStore) rememberCallWithKey(callID string, key string, state composerLoopState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey[key] = state
	s.byCall[callID] = key
}

func (s *composerLoopToolStateStore) stateByKey(key string) (composerLoopState, bool) {
	if s == nil {
		return composerLoopState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.byKey[key]
	return state, ok
}

func (s *composerLoopToolStateStore) stateByCall(callID string) (composerLoopState, bool) {
	if s == nil {
		return composerLoopState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byCall[callID]
	if !ok {
		return composerLoopState{}, false
	}
	state, ok := s.byKey[key]
	return state, ok
}

func (s *composerLoopToolStateStore) forgetCall(callID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byCall, callID)
}

func applyComposerBeforeModel(state composerLoopState) composerLoopState {
	reminderState := toolloop.BeforeModel(composerLoopMessages(state.Context), state.ToolIterations, state.ReminderCooldowns, toolloop.DefaultConfig())
	state.ReminderCooldowns = reminderState.Cooldowns
	state.Context.PendingReminders = reminderState.PendingReminders
	return state
}

func composerLoopMessages(context Context) []toolloop.Message {
	out := make([]toolloop.Message, 0, len(context.SameTurnMessages)+1)
	if strings.TrimSpace(context.Input.Input.Instructions) != "" {
		out = append(out, toolloop.Message{Role: "user", MessageType: "text"})
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

func routeComposerModelOutput() compose.GraphBranchCondition[composerLoopState] {
	return func(_ context.Context, state composerLoopState) (string, error) {
		if len(nativeToolCalls(state.LastAssistantMessage)) > 0 {
			if state.ToolIterations >= defaultComposerMaxToolCalls {
				return "fail_turn", nil
			}
			return "prepare_tool_message", nil
		}
		return "finalize_response", nil
	}
}

func prepareComposerToolMessage(stateStore *composerLoopToolStateStore, state composerLoopState) (*schema.Message, error) {
	assistantMessage := state.LastAssistantMessage
	if assistantMessage == nil && len(state.LastToolCalls) > 0 {
		assistantMessage = &schema.Message{Role: schema.Assistant, ToolCalls: append([]schema.ToolCall(nil), state.LastToolCalls...)}
	}
	if assistantMessage == nil {
		return nil, errors.New("composer model returned no tool call message")
	}
	cleanMessage := cloneToolCallMessage(assistantMessage)
	state.LastAssistantMessage = cloneToolCallMessage(cleanMessage)
	state.LastToolCalls = append([]schema.ToolCall(nil), cleanMessage.ToolCalls...)
	state.LastToolResults = nil

	stateKey := uuid.NewString()
	stateStore.rememberKey(stateKey, state)
	messageForToolNode := cloneToolCallMessage(cleanMessage)
	for i := range messageForToolNode.ToolCalls {
		args, ok := toolCallArgumentsObject(messageForToolNode.ToolCalls[i].Function.Arguments)
		if ok {
			args[composerLoopStateArgumentKey] = stateKey
			raw, err := json.Marshal(args)
			if err != nil {
				return nil, err
			}
			messageForToolNode.ToolCalls[i].Function.Arguments = string(raw)
		}
		stateStore.rememberCallWithKey(messageForToolNode.ToolCalls[i].ID, stateKey, state)
	}
	return messageForToolNode, nil
}

func composerLoopStateForToolResults(stateStore *composerLoopToolStateStore, toolResults []*schema.Message) (composerLoopState, error) {
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
	return composerLoopState{}, errors.New("composer tool result state is missing")
}

func nativeComposerToolRuntimeMiddleware(stateStore *composerLoopToolStateStore) compose.ToolMiddleware {
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
				if stateKey, _ := args[composerLoopStateArgumentKey].(string); stateKey != "" && stateStore != nil {
					delete(args, composerLoopStateArgumentKey)
					if raw, err := json.Marshal(args); err == nil {
						input.Arguments = string(raw)
					}
					if state, ok := stateStore.stateByKey(stateKey); ok {
						stateStore.rememberCallWithKey(input.CallID, stateKey, state)
						ctx = agenttools.WithNativeRuntimeContext(ctx, composerNativeRuntime(input.CallID, state))
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

func composerNativeRuntime(callID string, state composerLoopState) agenttools.NativeRuntimeContext {
	scopeID := state.Context.SourceStoryboardNodeID
	if !scopeID.Valid {
		scopeID = sourceStoryboardNodeID(state.Context.Input)
	}
	return agenttools.NativeRuntimeContext{
		WorkspaceID: state.Context.Input.WorkspaceID,
		ThreadID:    state.Context.Input.ThreadID,
		TaskID:      state.Context.Input.TaskID,
		TaskType:    "composer_turn",
		ToolCallID:  callID,
		ScopeType:   "final_output",
		ScopeID:     scopeID,
		TargetPhase: "final_video",
	}
}

func finalizeComposerOutput(state composerLoopState) CompositionOutput {
	out := state.LastOutput.Result
	out = applySubmitArtifactToolResult(out, state.Context.SameTurnMessages)
	if strings.TrimSpace(out.Status) == "" {
		out.Status = "blocked"
	}
	if strings.TrimSpace(out.OperationType) == "" {
		out.OperationType = "compose_final_video"
	}
	out.SameTurnMessages = append([]ComposerSameTurnMessage(nil), state.Context.SameTurnMessages...)
	return out
}

func applySubmitArtifactToolResult(out CompositionOutput, messages []ComposerSameTurnMessage) CompositionOutput {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if strings.TrimSpace(message.ToolName) != "submit_composition_artifact" || strings.TrimSpace(message.Content) == "" {
			continue
		}
		var result struct {
			OutputNodeID       string `json:"output_node_id"`
			GenerationJobID    string `json:"generation_job_id"`
			ArtifactVersionID  string `json:"artifact_version_id"`
			TimelinePlanID     string `json:"timeline_plan_id"`
			SandboxJobID       string `json:"sandbox_job_id"`
			GenerationJobState string `json:"generation_job_state"`
		}
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		if strings.TrimSpace(result.OutputNodeID) == "" ||
			strings.TrimSpace(result.GenerationJobID) == "" ||
			strings.TrimSpace(result.ArtifactVersionID) == "" {
			continue
		}
		out.Status = "completed"
		out.NodeID = firstNonEmpty(out.NodeID, result.OutputNodeID)
		out.GenerationJobID = firstNonEmpty(out.GenerationJobID, result.GenerationJobID)
		out.ArtifactVersionID = firstNonEmpty(out.ArtifactVersionID, result.ArtifactVersionID)
		out.TimelinePlanID = firstNonEmpty(out.TimelinePlanID, result.TimelinePlanID)
		out.SandboxJobID = firstNonEmpty(out.SandboxJobID, result.SandboxJobID)
		break
	}
	return out
}

func firstNonEmpty(current, fallback string) string {
	if strings.TrimSpace(current) != "" {
		return current
	}
	return strings.TrimSpace(fallback)
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

func toolCallArgumentsObject(raw string) (map[string]any, bool) {
	args := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return args, true
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, false
	}
	return args, true
}

func appendComposerSameTurnMessages(input *Context, out ComposerTurnOutput, assistant *schema.Message, toolResults []*schema.Message) {
	if input == nil || assistant == nil {
		return
	}
	for _, call := range assistant.ToolCalls {
		args, _ := toolCallArgumentsObject(call.Function.Arguments)
		input.SameTurnMessages = append(input.SameTurnMessages, ComposerSameTurnMessage{
			Role:          "assistant",
			MessageType:   "tool_call",
			Content:       out.AssistantText,
			ToolCallID:    call.ID,
			ToolName:      call.Function.Name,
			ToolArguments: args,
		})
	}
	for _, result := range toolResults {
		if result == nil {
			continue
		}
		input.SameTurnMessages = append(input.SameTurnMessages, ComposerSameTurnMessage{
			Role:        "tool",
			MessageType: "tool_result",
			Content:     result.Content,
			ToolCallID:  result.ToolCallID,
			ToolName:    result.ToolName,
		})
	}
}

func sourceStoryboardNodeID(input GraphInput) pgtype.UUID {
	if id, ok := pgUUIDFromString(input.Input.SourceStoryboardNodeID); ok {
		return id
	}
	return pgtype.UUID{}
}

var _ = production.GenerationIntent{}
