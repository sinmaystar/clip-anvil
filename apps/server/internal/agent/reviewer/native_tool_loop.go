package reviewer

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

	"github.com/sinmaystar/clip-anvil/internal/agent/toolloop"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const (
	reviewerLoopStateArgumentKey = "_clipanvil_reviewer_loop_state_key"
	defaultReviewerMaxToolCalls  = 1000
	reviewerGraphMaxRunSteps     = 10000
)

type reviewerLoopState struct {
	Context              Context
	LastOutput           ReviewerTurnOutput
	LastAssistantMessage *schema.Message
	LastToolCalls        []schema.ToolCall
	ToolIterations       int
	ReminderCooldowns    map[string]int
	Submitted            bool
	SubmittedRecordID    pgtype.UUID
	SubmittedRecordKey   string
	SubmittedVerdict     string
}

type reviewerLoopToolStateStore struct {
	mu     sync.Mutex
	byKey  map[string]reviewerLoopState
	byCall map[string]string
}

func newReviewerLoopToolStateStore() *reviewerLoopToolStateStore {
	return &reviewerLoopToolStateStore{
		byKey:  map[string]reviewerLoopState{},
		byCall: map[string]string{},
	}
}

func (s *reviewerLoopToolStateStore) rememberKey(key string, state reviewerLoopState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey[key] = state
}

func (s *reviewerLoopToolStateStore) rememberCallWithKey(callID string, key string, state reviewerLoopState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey[key] = state
	s.byCall[callID] = key
}

func (s *reviewerLoopToolStateStore) stateByKey(key string) (reviewerLoopState, bool) {
	if s == nil {
		return reviewerLoopState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.byKey[key]
	return state, ok
}

func (s *reviewerLoopToolStateStore) stateByCall(callID string) (reviewerLoopState, bool) {
	if s == nil {
		return reviewerLoopState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byCall[callID]
	if !ok {
		return reviewerLoopState{}, false
	}
	state, ok := s.byKey[key]
	return state, ok
}

func newNativeToolLoopGraph(config GraphConfig) (*Graph, error) {
	stateStore := newReviewerLoopToolStateStore()
	toolInfos, err := config.NativeToolRegistry.ToolInfos(context.Background())
	if err != nil {
		return nil, err
	}
	toolNode, err := compose.NewToolNode(context.Background(), &compose.ToolsNodeConfig{
		Tools:               config.NativeToolRegistry.BaseTools(),
		ExecuteSequentially: true,
		ToolCallMiddlewares: []compose.ToolMiddleware{nativeReviewerToolRuntimeMiddleware(stateStore)},
	})
	if err != nil {
		return nil, err
	}
	g := compose.NewGraph[GraphInput, GraphOutput]()
	if err := g.AddLambdaNode("load_context", compose.InvokableLambda(func(ctx context.Context, input GraphInput) (Context, error) {
		reviewContext, err := config.Loader.Load(ctx, input)
		if err != nil {
			return Context{}, err
		}
		reviewContext.ToolInfos = toolInfos
		return reviewContext, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("prepare_turn_state", compose.InvokableLambda(func(_ context.Context, input Context) (reviewerLoopState, error) {
		return reviewerLoopState{Context: input}, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("before_model", compose.InvokableLambda(func(_ context.Context, state reviewerLoopState) (reviewerLoopState, error) {
		state = applyReviewerBeforeModel(state)
		return state, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("call_model", compose.InvokableLambda(func(ctx context.Context, state reviewerLoopState) (reviewerLoopState, error) {
		out, err := config.ToolResponder.Respond(ctx, state.Context)
		if err != nil {
			return reviewerLoopState{}, err
		}
		state.LastOutput = out
		state.LastAssistantMessage = out.ModelMessage
		state.LastToolCalls = append([]schema.ToolCall(nil), nativeToolCalls(out.ModelMessage)...)
		return state, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("prepare_tool_message", compose.InvokableLambda(func(_ context.Context, state reviewerLoopState) (*schema.Message, error) {
		return prepareReviewerToolMessage(stateStore, state)
	})); err != nil {
		return nil, err
	}
	if err := g.AddToolsNode("execute_tools", toolNode); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("append_tool_results", compose.InvokableLambda(func(_ context.Context, toolResults []*schema.Message) (reviewerLoopState, error) {
		state, err := reviewerLoopStateForToolResults(stateStore, toolResults)
		if err != nil {
			return reviewerLoopState{}, err
		}
		appendReviewerSameTurnMessages(&state.Context, state.LastAssistantMessage, toolResults)
		state.ToolIterations++
		for _, message := range toolResults {
			if message != nil && message.ToolName == toolSubmitReviewResultName() && strings.Contains(message.Content, "已提交 Reviewer 评审结果") {
				state.Submitted = true
				if id, ok := reviewerToolResultUUID(message.Content, "review_record"); ok {
					state.SubmittedRecordID = id
				}
				if id, ok := reviewerToolResultUUID(message.Content, "review_record_id"); ok {
					state.SubmittedRecordID = id
				}
				if key := reviewerToolResultObjectKey(message.Content, "review_record_ref", "review_record"); key != "" {
					state.SubmittedRecordKey = key
				}
				if verdict := reviewerToolResultValue(message.Content, "verdict"); verdict != "" {
					state.SubmittedVerdict = verdict
				}
			}
		}
		return state, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("finalize_response", compose.InvokableLambda(func(_ context.Context, state reviewerLoopState) (GraphOutput, error) {
		if !state.Submitted && len(nativeToolCalls(state.LastAssistantMessage)) == 0 {
			return GraphOutput{}, errors.New("reviewer finished without submit_review_result")
		}
		status := state.SubmittedVerdict
		if status == "" {
			status = ReviewStatusAccepted
		}
		return GraphOutput{
			Record:           db.ReviewRecord{ID: state.SubmittedRecordID, SemanticKey: state.SubmittedRecordKey},
			Decision:         ReviewDecision{Status: status, ShouldRetry: status == ReviewStatusRejected},
			Result:           ReviewResult{Critique: state.LastOutput.AssistantText},
			SameTurnMessages: append([]ReviewerSameTurnMessage(nil), state.Context.SameTurnMessages...),
		}, nil
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("fail_turn", compose.InvokableLambda(func(_ context.Context, _ reviewerLoopState) (GraphOutput, error) {
		return GraphOutput{}, errors.New("reviewer exceeded max tool calls")
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
	if err := g.AddBranch("call_model", compose.NewGraphBranch(routeReviewerModelOutput(), map[string]bool{
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
		compose.WithGraphName("reviewer_gate"),
		compose.WithMaxRunSteps(reviewerGraphMaxRunSteps),
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

func applyReviewerBeforeModel(state reviewerLoopState) reviewerLoopState {
	reminderState := toolloop.BeforeModel(reviewerLoopMessages(state.Context), state.ToolIterations, state.ReminderCooldowns, toolloop.DefaultConfig())
	state.ReminderCooldowns = reminderState.Cooldowns
	state.Context.PendingReminders = reminderState.PendingReminders
	return state
}

func reviewerLoopMessages(context Context) []toolloop.Message {
	out := make([]toolloop.Message, 0, len(context.Messages)+len(context.SameTurnMessages)+1)
	for _, message := range context.Messages {
		out = append(out, toolloop.Message{
			Role:        strings.TrimSpace(message.Role),
			MessageType: strings.TrimSpace(message.MessageType),
			ToolName:    toolNameFromRaw(message.RawMessage),
		})
	}
	if strings.TrimSpace(context.Text) != "" {
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

func routeReviewerModelOutput() compose.GraphBranchCondition[reviewerLoopState] {
	return func(_ context.Context, state reviewerLoopState) (string, error) {
		if len(nativeToolCalls(state.LastAssistantMessage)) > 0 {
			if state.ToolIterations >= maxReviewerToolCalls() {
				return "fail_turn", nil
			}
			return "prepare_tool_message", nil
		}
		return "finalize_response", nil
	}
}

func maxReviewerToolCalls() int {
	return defaultReviewerMaxToolCalls
}

func prepareReviewerToolMessage(stateStore *reviewerLoopToolStateStore, state reviewerLoopState) (*schema.Message, error) {
	assistantMessage := state.LastAssistantMessage
	if assistantMessage == nil && len(state.LastToolCalls) > 0 {
		assistantMessage = &schema.Message{Role: schema.Assistant, ToolCalls: append([]schema.ToolCall(nil), state.LastToolCalls...)}
	}
	if assistantMessage == nil {
		return nil, errors.New("reviewer model returned no tool call message")
	}
	cleanMessage := cloneToolCallMessage(assistantMessage)
	state.LastAssistantMessage = cloneToolCallMessage(cleanMessage)
	state.LastToolCalls = append([]schema.ToolCall(nil), cleanMessage.ToolCalls...)

	stateKey := uuid.NewString()
	stateStore.rememberKey(stateKey, state)
	for _, call := range cleanMessage.ToolCalls {
		stateStore.rememberCallWithKey(call.ID, stateKey, state)
	}
	messageForToolNode := cloneToolCallMessage(cleanMessage)
	for i := range messageForToolNode.ToolCalls {
		args := toolCallArgumentsMap(messageForToolNode.ToolCalls[i])
		args[reviewerLoopStateArgumentKey] = stateKey
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		messageForToolNode.ToolCalls[i].Function.Arguments = string(raw)
	}
	return messageForToolNode, nil
}

func reviewerLoopStateForToolResults(stateStore *reviewerLoopToolStateStore, toolResults []*schema.Message) (reviewerLoopState, error) {
	for _, message := range toolResults {
		if message == nil {
			continue
		}
		if state, ok := stateStore.stateByCall(message.ToolCallID); ok {
			return state, nil
		}
	}
	return reviewerLoopState{}, errors.New("reviewer tool result missing loop state")
}

func appendReviewerSameTurnMessages(reviewContext *Context, assistant *schema.Message, toolResults []*schema.Message) {
	if reviewContext == nil || assistant == nil {
		return
	}
	for _, call := range assistant.ToolCalls {
		args := map[string]any{}
		_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
		reviewContext.SameTurnMessages = append(reviewContext.SameTurnMessages, ReviewerSameTurnMessage{
			Role:          "assistant",
			MessageType:   "tool_call",
			Content:       assistant.Content,
			ToolCallID:    call.ID,
			ToolName:      call.Function.Name,
			ToolArguments: args,
		})
	}
	for _, result := range toolResults {
		if result == nil {
			continue
		}
		reviewContext.SameTurnMessages = append(reviewContext.SameTurnMessages, ReviewerSameTurnMessage{
			Role:        "tool",
			MessageType: "tool_result",
			Content:     result.Content,
			ToolCallID:  result.ToolCallID,
			ToolName:    result.ToolName,
		})
	}
}

func nativeReviewerToolRuntimeMiddleware(stateStore *reviewerLoopToolStateStore) compose.ToolMiddleware {
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
				if stateKey, _ := args[reviewerLoopStateArgumentKey].(string); stateKey != "" && stateStore != nil {
					delete(args, reviewerLoopStateArgumentKey)
					if raw, err := json.Marshal(args); err == nil {
						input.Arguments = string(raw)
					}
					if state, ok := stateStore.stateByKey(stateKey); ok {
						stateStore.rememberCallWithKey(input.CallID, stateKey, state)
						ctx = agenttools.WithNativeRuntimeContext(ctx, agenttools.NativeRuntimeContext{
							WorkspaceID:                state.Context.Input.WorkspaceID,
							ThreadID:                   state.Context.Input.ThreadID,
							TaskID:                     state.Context.Input.TaskID,
							TaskType:                   "reviewer_turn",
							ToolCallID:                 input.CallID,
							ReviewTask:                 state.Context.Input.Task.ReviewTask,
							ReviewShotID:               state.Context.Input.Task.Target.ShotID,
							ReviewShotKey:              state.Context.Input.Task.Target.ShotRef.Key,
							ReviewNodeID:               state.Context.Input.Task.Target.NodeID,
							ReviewNodeKey:              state.Context.Input.Task.Target.NodeRef.Key,
							ReviewVersionID:            state.Context.Input.Task.Target.ArtifactVersionID,
							ReviewVersionKey:           state.Context.Input.Task.Target.ArtifactVersionRef.Key,
							ReviewJobID:                state.Context.Input.Task.Target.GenerationJobID,
							ReviewRenderPlanID:         state.Context.Input.Task.Target.RenderPlanID,
							ReviewRenderPlanKey:        state.Context.Input.Task.Target.RenderPlanRef.Key,
							ReviewParentReviewRecordID: state.Context.Input.Task.Target.ParentReviewRecordID,
							ReviewParentReviewKey:      state.Context.Input.Task.Target.ParentReviewRef.Key,
							ReviewAttemptNo:            state.Context.Input.Task.AttemptNo,
							ReviewMaxAttempts:          state.Context.Input.Task.MaxAttempts,
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

func nativeToolCalls(message *schema.Message) []schema.ToolCall {
	if message == nil {
		return nil
	}
	return message.ToolCalls
}

func cloneToolCallMessage(message *schema.Message) *schema.Message {
	if message == nil {
		return nil
	}
	clone := *message
	clone.Role = schema.Assistant
	clone.ToolCalls = append([]schema.ToolCall(nil), message.ToolCalls...)
	for i := range clone.ToolCalls {
		if strings.TrimSpace(clone.ToolCalls[i].ID) == "" {
			clone.ToolCalls[i].ID = uuid.NewString()
		}
		if strings.TrimSpace(clone.ToolCalls[i].Type) == "" {
			clone.ToolCalls[i].Type = "function"
		}
	}
	return &clone
}

func toolNameFromRaw(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	text, _ := payload["tool_name"].(string)
	return strings.TrimSpace(text)
}

func toolSubmitReviewResultName() string {
	return "submit_review_result"
}

func toolCallArgumentsMap(call schema.ToolCall) map[string]any {
	args := map[string]any{}
	if strings.TrimSpace(call.Function.Arguments) == "" {
		return args
	}
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	return args
}

func reviewerToolResultUUID(content string, label string) (pgtype.UUID, bool) {
	value := reviewerToolResultValue(content, label)
	if value == "" {
		return pgtype.UUID{}, false
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}

func reviewerToolResultValue(content string, label string) string {
	prefix := "- " + label + "："
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func reviewerToolResultObjectKey(content string, label string, objectType string) string {
	value := reviewerToolResultValue(content, label)
	value = strings.TrimSpace(value)
	prefix := objectType + "/"
	if strings.HasPrefix(value, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(value, prefix))
	}
	return ""
}
