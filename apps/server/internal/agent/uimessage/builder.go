package uimessage

import (
	"encoding/json"
	"strings"
)

type UserMessageInput struct {
	Text            string
	ClientMessageID string
	Attachments     []Attachment
}

type AssistantMessageInput struct {
	Text             string
	ReasoningContent string
	IncludeThinking  bool
	DefaultCollapsed bool
}

type SystemReminderInput struct {
	Text string
}

type ToolStatusInput struct {
	ToolCallID   string
	ToolName     string
	Label        string
	Status       string
	Summary      string
	ErrorMessage string
	Arguments    map[string]any
	Result       map[string]any
}

type DecisionCardInput struct {
	DecisionID    string
	Title         string
	Message       string
	Options       []DecisionOption
	AllowFreeText bool
	Status        string
}

func BuildUserMessageContent(input UserMessageInput) ([]byte, error) {
	blocks := []Block{
		MarkdownBlock{
			BaseBlock: NewBaseBlock("blk_text", "markdown"),
			Text:      strings.TrimSpace(input.Text),
		},
	}
	if len(input.Attachments) > 0 {
		blocks = append(blocks, AttachmentBlock{
			BaseBlock:   NewBaseBlock("blk_attachments", "attachment"),
			Attachments: input.Attachments,
		})
	}
	metadata := map[string]any{}
	if strings.TrimSpace(input.ClientMessageID) != "" {
		metadata["client_message_id"] = strings.TrimSpace(input.ClientMessageID)
	}
	return marshalEnvelope(blocks, metadata)
}

func BuildAssistantMessageContent(input AssistantMessageInput) ([]byte, error) {
	blocks := []Block{}
	reasoning := strings.TrimSpace(input.ReasoningContent)
	if input.IncludeThinking && reasoning != "" {
		blocks = append(blocks, ThinkingBlock{
			BaseBlock:        NewBaseBlock("blk_thinking", "thinking"),
			Text:             reasoning,
			Status:           "done",
			DefaultCollapsed: input.DefaultCollapsed,
		})
	}
	text := strings.TrimSpace(input.Text)
	if text != "" {
		blocks = append(blocks, MarkdownBlock{
			BaseBlock: NewBaseBlock("blk_answer", "markdown"),
			Text:      text,
		})
	}
	return marshalEnvelope(blocks, nil)
}

func BuildSystemReminderMessageContent(input SystemReminderInput) ([]byte, error) {
	return marshalEnvelope([]Block{
		SystemReminderBlock{
			BaseBlock: NewBaseBlock("blk_system_reminder", "system_reminder"),
			Text:      strings.TrimSpace(input.Text),
		},
	}, nil)
}

func BuildToolStatusMessageContent(input ToolStatusInput) ([]byte, error) {
	return marshalEnvelope([]Block{
		ToolStatusBlock{
			BaseBlock:    NewBaseBlock("blk_tool_status", "tool_status"),
			ToolCallID:   strings.TrimSpace(input.ToolCallID),
			ToolName:     strings.TrimSpace(input.ToolName),
			Label:        strings.TrimSpace(input.Label),
			Status:       strings.TrimSpace(input.Status),
			Summary:      strings.TrimSpace(input.Summary),
			ErrorMessage: strings.TrimSpace(input.ErrorMessage),
			Arguments:    input.Arguments,
			Result:       input.Result,
		},
	}, nil)
}

func BuildDecisionCardMessageContent(input DecisionCardInput) ([]byte, error) {
	return marshalEnvelope([]Block{
		DecisionCardBlock{
			BaseBlock:     NewBaseBlock("blk_decision", "decision_card"),
			DecisionID:    strings.TrimSpace(input.DecisionID),
			Title:         strings.TrimSpace(input.Title),
			Message:       strings.TrimSpace(input.Message),
			Options:       input.Options,
			AllowFreeText: input.AllowFreeText,
			Status:        strings.TrimSpace(input.Status),
		},
	}, nil)
}

func marshalEnvelope(blocks []Block, metadata map[string]any) ([]byte, error) {
	if len(metadata) == 0 {
		metadata = nil
	}
	return json.Marshal(Envelope{
		Schema:   SchemaV1,
		Blocks:   blocks,
		Metadata: metadata,
	})
}
