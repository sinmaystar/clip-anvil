package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"github.com/google/uuid"

	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
)

type einoToolCallIDContextKey struct{}

type einoProducerTool struct {
	definition      agenttools.Definition
	producerContext ProducerContext
	executor        ToolExecutor
	runState        *einoToolRunState
}

type einoToolRunState struct {
	mu          sync.Mutex
	results     map[string]ToolExecutionResult
	definitions map[string]agenttools.Definition
	toolInfos   []*schema.ToolInfo
}

func newEinoToolRunState() *einoToolRunState {
	return &einoToolRunState{
		results:     map[string]ToolExecutionResult{},
		definitions: map[string]agenttools.Definition{},
	}
}

func (s *einoToolRunState) record(result ToolExecutionResult) {
	if s == nil || result.ToolCallID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[result.ToolCallID] = result
}

func (s *einoToolRunState) result(toolCallID string) (ToolExecutionResult, bool) {
	if s == nil || toolCallID == "" {
		return ToolExecutionResult{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, ok := s.results[toolCallID]
	return result, ok
}

func (s *einoToolRunState) definition(name string) (agenttools.Definition, bool) {
	if s == nil || name == "" {
		return agenttools.Definition{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	def, ok := s.definitions[name]
	return def, ok
}

func (s *einoToolRunState) anyInterrupted() (ToolExecutionResult, bool) {
	if s == nil {
		return ToolExecutionResult{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, result := range s.results {
		if result.Interrupted {
			return result, true
		}
	}
	return ToolExecutionResult{}, false
}

func producerToolInfo(def agenttools.Definition) (*schema.ToolInfo, error) {
	info := &schema.ToolInfo{
		Name: def.Name,
		Desc: def.Description,
		Extra: map[string]any{
			"read_only":               def.Safety.ReadOnly,
			"requires_hitl":           def.Safety.RequiresHITL,
			"writes_canvas":           def.Safety.WritesCanvas,
			"uses_production_service": def.Safety.UsesProductionService,
			"max_calls_per_turn":      def.Safety.MaxCallsPerTurn,
			"user_label":              def.Visibility.UserLabel,
			"show_call_message":       def.Visibility.ShowCallMessage,
			"show_result_message":     def.Visibility.ShowResultMessage,
		},
	}
	if len(def.Parameters) == 0 {
		return info, nil
	}
	raw, err := json.Marshal(def.Parameters)
	if err != nil {
		return nil, fmt.Errorf("marshal tool parameters: %w", err)
	}
	var js jsonschema.Schema
	if err := json.Unmarshal(raw, &js); err != nil {
		return nil, fmt.Errorf("decode tool parameters: %w", err)
	}
	info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(&js)
	return info, nil
}

func newEinoProducerTools(ctx context.Context, producerContext ProducerContext, registry *agenttools.Registry, executor ToolExecutor) ([]einotool.BaseTool, *einoToolRunState, error) {
	if registry == nil {
		return nil, nil, NewAgentError("agent_tool_registry_missing", "producer tool registry is not configured")
	}
	if executor == nil {
		return nil, nil, NewAgentError("agent_tool_executor_missing", "producer tool executor is not configured")
	}
	runState := newEinoToolRunState()
	executors := registry.Executors()
	tools := make([]einotool.BaseTool, 0, len(executors))
	for _, registeredTool := range executors {
		def := registeredTool.Definition()
		info, err := producerToolInfo(def)
		if err != nil {
			return nil, nil, err
		}
		runState.definitions[def.Name] = def
		runState.toolInfos = append(runState.toolInfos, info)
		tools = append(tools, &einoProducerTool{
			definition:      def,
			producerContext: producerContext,
			executor:        executor,
			runState:        runState,
		})
	}
	_ = ctx
	return tools, runState, nil
}

func newEinoProducerToolNode(ctx context.Context, producerContext ProducerContext, registry *agenttools.Registry, executor ToolExecutor) (*compose.ToolsNode, *einoToolRunState, error) {
	tools, runState, err := newEinoProducerTools(ctx, producerContext, registry, executor)
	if err != nil {
		return nil, nil, err
	}
	node, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools:               tools,
		ExecuteSequentially: true,
		ToolCallMiddlewares: []compose.ToolMiddleware{einoToolCallIDMiddleware()},
	})
	if err != nil {
		return nil, nil, err
	}
	return node, runState, nil
}

func (t *einoProducerTool) Info(context.Context) (*schema.ToolInfo, error) {
	return producerToolInfo(t.definition)
}

func (t *einoProducerTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	if t.executor == nil {
		return "", NewAgentError("agent_tool_executor_missing", "producer tool executor is not configured")
	}
	args := map[string]any{}
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("decode tool arguments: %w", err)
		}
	}
	toolCallID, _ := ctx.Value(einoToolCallIDContextKey{}).(string)
	if toolCallID == "" {
		toolCallID = uuid.NewString()
	}
	result, err := t.executor.ExecuteProducerTool(ctx, t.producerContext, ToolCall{
		ID:        toolCallID,
		Name:      t.definition.Name,
		Arguments: args,
	})
	if err != nil {
		return "", err
	}
	if result.ToolCallID == "" {
		result.ToolCallID = toolCallID
	}
	if result.ToolName == "" {
		result.ToolName = t.definition.Name
	}
	t.runState.record(result)
	raw, err := json.Marshal(result.Result)
	if err != nil {
		return "", fmt.Errorf("encode tool result: %w", err)
	}
	return string(raw), nil
}

func einoToolCallIDMiddleware() compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				if input != nil && input.CallID != "" {
					ctx = context.WithValue(ctx, einoToolCallIDContextKey{}, input.CallID)
				}
				return next(ctx, input)
			}
		},
	}
}
