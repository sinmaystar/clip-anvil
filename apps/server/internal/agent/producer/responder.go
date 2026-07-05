package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type Responder interface {
	Respond(ctx context.Context, producerContext ProducerContext) (ProducerTurnOutput, error)
}

type DeterministicResponder struct{}

func (DeterministicResponder) Respond(_ context.Context, producerContext ProducerContext) (ProducerTurnOutput, error) {
	text := strings.TrimSpace(producerContext.LatestUserText)
	if text == "" {
		text = "你的需求"
	}
	if deterministicProducerHasCompositionSignal(producerContext) {
		return ProducerTurnOutput{
			AssistantText: "已收到 Composer 成片状态 signal；当前 mock Producer 不会基于 composition signal 自动重复派发 Composer。请根据最终状态决定修复动态渲染或切换 fallback。",
			Metadata: map[string]any{
				"responder": "deterministic",
				"signal":    "composition_status",
			},
		}, nil
	}
	if deterministicProducerToolResultSeen(producerContext, "dispatch_composer") {
		return ProducerTurnOutput{
			AssistantText: "已将动态 Remotion 成片任务派发给 Composer。请等待 Composer 生成、校验并渲染 agent_remotion_code_v1。",
			Metadata: map[string]any{
				"responder":           "deterministic",
				"composer_dispatched": true,
				"route":               "agent_remotion_code_v1",
			},
		}, nil
	}
	if deterministicProducerWantsAgentRemotion(text) {
		if attachment, ok := deterministicProducerFirstAttachment(producerContext.ImageAttachments); ok && strings.TrimSpace(attachment.NodeID) != "" {
			return deterministicProducerToolCall("dispatch_composer", map[string]any{
				"source_storyboard_node_id": strings.TrimSpace(attachment.NodeID),
				"instructions": strings.TrimSpace(fmt.Sprintf(
					"根据上传素材 %s 和用户诉求生成非固定模板中文营销视频；使用 agent_remotion_code_v1 动态 Remotion renderer attempt，包含字幕、转场、品牌化 CTA，并记录 mock 环境下 Seedream/Seedance/音频的替代策略。",
					firstNonEmpty(attachment.Name, "uploaded product asset"),
				)),
				"template_key": "agent_remotion_code_v1",
			}, map[string]any{
				"responder": "deterministic",
				"route":     "agent_remotion_code_v1",
			}), nil
		}
	}
	return ProducerTurnOutput{
		AssistantText: fmt.Sprintf("我已收到你的需求：「%s」。\n下一步我会先整理创作目标，再在后续阶段拆成分镜和生产任务。", text),
		Metadata: map[string]any{
			"responder": "deterministic",
		},
	}, nil
}

func deterministicProducerWantsAgentRemotion(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(lower, "remotion") &&
		(strings.Contains(text, "动态") || strings.Contains(text, "非固定模板") || strings.Contains(lower, "agent_remotion_code_v1"))
}

func deterministicProducerHasCompositionSignal(producerContext ProducerContext) bool {
	text := strings.Join([]string{
		producerContext.RuntimeTriggerText,
		strings.Join(producerContext.PendingReminders, "\n"),
	}, "\n")
	return strings.Contains(text, "composition_blocked") ||
		strings.Contains(text, "composition_failed") ||
		strings.Contains(text, "composition_completed")
}

func deterministicProducerFirstAttachment(attachments map[string]ProducerImageAttachment) (ProducerImageAttachment, bool) {
	if len(attachments) == 0 {
		return ProducerImageAttachment{}, false
	}
	keys := make([]string, 0, len(attachments))
	for key := range attachments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return attachments[keys[0]], true
}

func deterministicProducerToolResultSeen(producerContext ProducerContext, toolName string) bool {
	for _, message := range producerContext.SameTurnMessages {
		if message.Role == "tool" && strings.TrimSpace(message.ToolName) == toolName {
			return true
		}
	}
	return false
}

func deterministicProducerToolCall(name string, args map[string]any, metadata map[string]any) ProducerTurnOutput {
	raw, _ := json.Marshal(args)
	return ProducerTurnOutput{
		Metadata: metadata,
		ModelMessage: &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   "call_deterministic_" + strings.ReplaceAll(name, "_", "-"),
				Type: "function",
				Function: schema.FunctionCall{
					Name:      name,
					Arguments: string(raw),
				},
			}},
		},
	}
}
