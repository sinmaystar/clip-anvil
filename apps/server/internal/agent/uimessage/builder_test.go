package uimessage

import (
	"encoding/json"
	"testing"
)

func TestBuildUserMessageContentIncludesMarkdownAndAttachments(t *testing.T) {
	raw, err := BuildUserMessageContent(UserMessageInput{
		Text:            "第一行\n第二行",
		ClientMessageID: "client-1",
		Attachments: []Attachment{{
			AssetID: "asset-1", NodeID: "node-1", Kind: "image", Name: "hero.png", Mime: "image/png", SizeBytes: 123,
		}},
	})
	if err != nil {
		t.Fatalf("BuildUserMessageContent() error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if envelope["schema"] != SchemaV1 {
		t.Fatalf("schema = %#v", envelope["schema"])
	}
	if got := ExtractMarkdownTexts(raw); len(got) != 1 || got[0] != "第一行\n第二行" {
		t.Fatalf("markdown texts = %#v", got)
	}
	if got := ExtractAttachments(raw); len(got) != 1 || got[0].Name != "hero.png" {
		t.Fatalf("attachments = %#v", got)
	}
}

func TestBuildAssistantMessageContentOmitsEmptyThinking(t *testing.T) {
	raw, err := BuildAssistantMessageContent(AssistantMessageInput{
		Text:             "最终回复",
		ReasoningContent: "   ",
		IncludeThinking:  true,
		DefaultCollapsed: true,
	})
	if err != nil {
		t.Fatalf("BuildAssistantMessageContent() error = %v", err)
	}
	if string(raw) == "" {
		t.Fatal("expected content")
	}
	if len(ExtractMarkdownTexts(raw)) != 1 {
		t.Fatalf("expected one markdown block: %s", raw)
	}
	var envelope struct {
		Blocks []struct {
			Type string `json:"type"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, block := range envelope.Blocks {
		if block.Type == "thinking" {
			t.Fatalf("unexpected thinking block: %s", raw)
		}
	}
}

func TestBuildAssistantMessageContentIncludesVisibleThinking(t *testing.T) {
	raw, err := BuildAssistantMessageContent(AssistantMessageInput{
		Text:             "最终回复",
		ReasoningContent: "先分析",
		IncludeThinking:  true,
		DefaultCollapsed: true,
	})
	if err != nil {
		t.Fatalf("BuildAssistantMessageContent() error = %v", err)
	}
	var envelope struct {
		Blocks []struct {
			Type             string `json:"type"`
			Text             string `json:"text"`
			Status           string `json:"status"`
			DefaultCollapsed bool   `json:"default_collapsed"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(envelope.Blocks) != 2 || envelope.Blocks[0].Type != "thinking" || envelope.Blocks[1].Type != "markdown" {
		t.Fatalf("blocks = %#v", envelope.Blocks)
	}
	if envelope.Blocks[0].Text != "先分析" || envelope.Blocks[0].Status != "done" || !envelope.Blocks[0].DefaultCollapsed {
		t.Fatalf("thinking block = %#v", envelope.Blocks[0])
	}
}

func TestBuildToolStatusAndDecisionCardContent(t *testing.T) {
	toolRaw, err := BuildToolStatusMessageContent(ToolStatusInput{
		ToolCallID: "call-1",
		ToolName:   "read_workspace_context",
		Label:      "工具执行完成",
		Status:     "succeeded",
		Arguments:  map[string]any{"include_canvas": true},
		Result:     map[string]any{"ok": true},
	})
	if err != nil {
		t.Fatalf("BuildToolStatusMessageContent() error = %v", err)
	}
	if !containsBlockType(t, toolRaw, "tool_status") {
		t.Fatalf("tool content missing tool_status: %s", toolRaw)
	}
	var toolContent struct {
		Blocks []struct {
			Arguments map[string]any `json:"arguments"`
			Result    map[string]any `json:"result"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(toolRaw, &toolContent); err != nil {
		t.Fatalf("unmarshal tool content: %v", err)
	}
	if toolContent.Blocks[0].Arguments["include_canvas"] != true || toolContent.Blocks[0].Result["ok"] != true {
		t.Fatalf("tool payload = %#v", toolContent.Blocks[0])
	}

	cardRaw, err := BuildDecisionCardMessageContent(DecisionCardInput{
		DecisionID:    "decision-1",
		Title:         "确认方向",
		Message:       "请选择",
		AllowFreeText: true,
		Status:        "pending",
		Options:       []DecisionOption{{ID: "a", Label: "方案 A"}},
	})
	if err != nil {
		t.Fatalf("BuildDecisionCardMessageContent() error = %v", err)
	}
	if !containsBlockType(t, cardRaw, "decision_card") {
		t.Fatalf("decision content missing decision_card: %s", cardRaw)
	}
}

func containsBlockType(t *testing.T, raw []byte, blockType string) bool {
	t.Helper()
	var envelope struct {
		Blocks []struct {
			Type string `json:"type"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, block := range envelope.Blocks {
		if block.Type == blockType {
			return true
		}
	}
	return false
}
