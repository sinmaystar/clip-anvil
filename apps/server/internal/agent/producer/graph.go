package producer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
)

var ErrInvalidGraphConfig = errors.New("invalid producer graph config")

type ContextLoader interface {
	LoadProducerContext(ctx context.Context, input ProducerTurnInput) (ProducerContext, error)
}

type GraphConfig struct {
	Loader                         ContextLoader
	Responder                      Responder
	ToolExecutor                   ToolExecutor
	ToolRegistry                   *agenttools.Registry
	EnableLegacyToolParserFallback bool
}

type Graph struct {
	runnable compose.Runnable[ProducerTurnInput, ProducerTurnOutput]
}

func NewGraph(config GraphConfig) (*Graph, error) {
	if config.Loader == nil || config.Responder == nil {
		return nil, ErrInvalidGraphConfig
	}

	g := compose.NewGraph[ProducerTurnInput, ProducerTurnOutput]()
	if err := g.AddLambdaNode("load_context", compose.InvokableLambda(func(ctx context.Context, input ProducerTurnInput) (ProducerContext, error) {
		return config.Loader.LoadProducerContext(ctx, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("draft_response", compose.InvokableLambda(func(ctx context.Context, input ProducerContext) (ProducerTurnOutput, error) {
		return runProducerLoop(ctx, config.Responder, config.ToolRegistry, config.ToolExecutor, input, config.EnableLegacyToolParserFallback)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("finalize_response", compose.InvokableLambda(func(_ context.Context, input ProducerTurnOutput) (ProducerTurnOutput, error) {
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
			return ProducerTurnOutput{}, errors.New("producer returned empty response")
		}
		if input.Metadata == nil {
			input.Metadata = map[string]any{}
		}
		return input, nil
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

	runnable, err := g.Compile(context.Background(), compose.WithGraphName("producer_turn"))
	if err != nil {
		return nil, err
	}
	return &Graph{runnable: runnable}, nil
}

func stringFromMap(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func runProducerLoop(ctx context.Context, responder Responder, registry *agenttools.Registry, toolExecutor ToolExecutor, input ProducerContext, enableLegacyToolParserFallback bool) (ProducerTurnOutput, error) {
	maxToolCalls := input.Input.MaxToolCalls
	if maxToolCalls <= 0 {
		maxToolCalls = 50
	}
	var toolNode *compose.ToolsNode
	var toolRunState *einoToolRunState
	if registry != nil && toolExecutor != nil {
		var err error
		toolNode, toolRunState, err = newEinoProducerToolNode(ctx, input, registry, toolExecutor)
		if err != nil {
			return ProducerTurnOutput{}, err
		}
		input.ToolInfos = toolRunState.toolInfos
	}
	usedLegacyParser := false
	for toolCalls := 0; ; toolCalls++ {
		out, err := responder.Respond(ctx, input)
		if err != nil {
			return ProducerTurnOutput{}, err
		}
		if nativeToolCalls := nativeToolCalls(out.ModelMessage); len(nativeToolCalls) > 0 {
			if toolCalls >= maxToolCalls {
				return ProducerTurnOutput{}, NewAgentError("agent_tool_loop_exhausted", "producer exceeded max tool calls")
			}
			if toolNode == nil {
				return ProducerTurnOutput{}, NewAgentError("agent_tool_executor_missing", "producer tool executor is not configured")
			}
			assistantMessage := cloneToolCallMessage(out.ModelMessage)
			toolResults, err := toolNode.Invoke(ctx, assistantMessage)
			if err != nil {
				return ProducerTurnOutput{}, err
			}
			appendNativeSameTurnMessages(&input, out, assistantMessage, toolResults)
			if result, interrupted := toolRunState.anyInterrupted(); interrupted {
				return ProducerTurnOutput{
					AssistantText: "等待你的选择。",
					Metadata: map[string]any{
						"interrupted":  true,
						"tool_name":    result.ToolName,
						"tool_call_id": result.ToolCallID,
					},
				}, nil
			}
			continue
		}
		if !enableLegacyToolParserFallback {
			out.UsedLegacyToolParser = usedLegacyParser
			return out, nil
		}
		parsed, err := ParseToolCall(out.AssistantText)
		if err != nil {
			return ProducerTurnOutput{}, err
		}
		if !parsed.HasToolCall {
			out.UsedLegacyToolParser = usedLegacyParser
			return out, nil
		}
		usedLegacyParser = true
		if toolCalls >= maxToolCalls {
			return ProducerTurnOutput{}, NewAgentError("agent_tool_loop_exhausted", "producer exceeded max tool calls")
		}
		if toolExecutor == nil {
			return ProducerTurnOutput{}, NewAgentError("agent_tool_executor_missing", "producer tool executor is not configured")
		}
		if parsed.ToolCall.ID == "" {
			parsed.ToolCall.ID = uuid.NewString()
		}
		result, err := toolExecutor.ExecuteProducerTool(ctx, input, parsed.ToolCall)
		if err != nil {
			return ProducerTurnOutput{}, err
		}
		if result.Interrupted {
			return ProducerTurnOutput{
				AssistantText: "等待你的选择。",
				Metadata: map[string]any{
					"interrupted": true,
					"tool_name":   parsed.ToolCall.Name,
				},
				UsedLegacyToolParser: true,
			}, nil
		}
		input.SameTurnMessages = append(input.SameTurnMessages,
			ProducerSameTurnMessage{
				Role:             "assistant",
				MessageType:      "tool_call",
				Content:          out.AssistantText,
				ReasoningContent: stringFromMap(out.Metadata, "reasoning_content"),
				ToolCallID:       result.ToolCallID,
				ToolName:         parsed.ToolCall.Name,
				ToolArguments:    parsed.ToolCall.Arguments,
			},
			ProducerSameTurnMessage{
				Role:        "tool",
				MessageType: "tool_result",
				Content:     string(mustJSON(result.Result)),
				ToolCallID:  result.ToolCallID,
				ToolName:    result.ToolName,
			},
		)
	}
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

func (g *Graph) Run(ctx context.Context, input ProducerTurnInput) (ProducerTurnOutput, error) {
	return g.runnable.Invoke(ctx, input)
}
