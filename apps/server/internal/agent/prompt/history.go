package prompt

import (
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func HistoryMessages(messages []db.AgentMessage) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			text := AgentMessageText(msg.Content)
			if text == "" {
				continue
			}
			out = append(out, schema.UserMessage(text))
		case "assistant":
			switch msg.MessageType {
			case "text":
				text := AgentMessageText(msg.Content)
				if text == "" {
					continue
				}
				out = append(out, schema.AssistantMessage(text, nil))
			case "tool_call":
				if toolMessage := HistoricalToolCallMessage(msg); toolMessage != nil {
					out = append(out, toolMessage)
				}
			}
		case "tool":
			if msg.MessageType == "tool_result" {
				if toolMessage := HistoricalToolResultMessage(msg); toolMessage != nil {
					out = append(out, toolMessage)
				}
			}
		}
	}
	return out
}

func HistoricalToolCallMessage(msg db.AgentMessage) *schema.Message {
	raw := jsonObject(msg.RawMessage)
	toolCallID := stringFromAny(raw["tool_call_id"])
	toolName := firstNonEmpty(stringFromAny(raw["tool_name"]), stringFromAny(raw["name"]))
	if toolCallID == "" || toolName == "" {
		return nil
	}
	arguments := "{}"
	if rawArgs, ok := raw["arguments"]; ok {
		if encoded, err := json.Marshal(rawArgs); err == nil {
			arguments = string(encoded)
		}
	}
	return schema.AssistantMessage(historicalToolText(msg), []schema.ToolCall{{
		ID:   toolCallID,
		Type: "function",
		Function: schema.FunctionCall{
			Name:      toolName,
			Arguments: arguments,
		},
	}})
}

func HistoricalToolResultMessage(msg db.AgentMessage) *schema.Message {
	raw := jsonObject(msg.RawMessage)
	toolCallID := stringFromAny(raw["tool_call_id"])
	toolName := firstNonEmpty(stringFromAny(raw["tool_name"]), stringFromAny(raw["name"]))
	content := firstNonEmpty(stringFromAny(raw["result_text"]), historicalToolText(msg))
	if content == "" {
		if rawResult, ok := raw["result"]; ok {
			if encoded, err := json.Marshal(rawResult); err == nil {
				content = string(encoded)
			}
		}
	}
	if content == "" {
		return nil
	}
	if toolCallID == "" {
		return schema.UserMessage("工具返回：" + content)
	}
	return schema.ToolMessage(content, toolCallID, schema.WithToolName(toolName))
}

func AgentMessageText(raw []byte) string {
	text := strings.TrimSpace(strings.Join(uimessage.ExtractMarkdownTexts(raw), "\n\n"))
	attachments := uimessage.ExtractAttachments(raw)
	if len(attachments) == 0 {
		return text
	}
	lines := []string{text, "用户附加素材："}
	for _, attachment := range attachments {
		kind := strings.TrimSpace(attachment.Kind)
		name := strings.TrimSpace(attachment.Name)
		if kind == "" || name == "" {
			continue
		}
		lines = append(lines, "- "+kind+": "+name)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func historicalToolText(msg db.AgentMessage) string {
	if text := strings.TrimSpace(AgentMessageText(msg.Content)); text != "" {
		return text
	}
	content := jsonObject(msg.Content)
	return strings.TrimSpace(stringFromAny(content["text"]))
}

func jsonObject(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
