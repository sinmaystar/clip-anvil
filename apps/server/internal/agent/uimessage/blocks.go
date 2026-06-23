package uimessage

import (
	"encoding/json"
	"strings"
)

const SchemaV1 = "clipanvil.agent.message.v1"

type Envelope struct {
	Schema   string         `json:"schema"`
	Blocks   []Block        `json:"blocks"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Block interface {
	UIBlockType() string
}

type BaseBlock struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	CreatedAt  string `json:"created_at,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

type MarkdownBlock struct {
	BaseBlock
	Text string `json:"text"`
}

func (MarkdownBlock) UIBlockType() string { return "markdown" }

func (b MarkdownBlock) MarshalJSON() ([]byte, error) {
	type alias MarkdownBlock
	next := alias(b)
	next.Type = b.UIBlockType()
	return json.Marshal(next)
}

type ThinkingBlock struct {
	BaseBlock
	Text             string `json:"text"`
	Status           string `json:"status"`
	DefaultCollapsed bool   `json:"default_collapsed"`
}

func (ThinkingBlock) UIBlockType() string { return "thinking" }

func (b ThinkingBlock) MarshalJSON() ([]byte, error) {
	type alias ThinkingBlock
	next := alias(b)
	next.Type = b.UIBlockType()
	return json.Marshal(next)
}

type DecisionOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type DecisionCardBlock struct {
	BaseBlock
	DecisionID       string           `json:"decision_id"`
	Title            string           `json:"title"`
	Message          string           `json:"message"`
	Options          []DecisionOption `json:"options"`
	AllowFreeText    bool             `json:"allow_free_text"`
	Status           string           `json:"status"`
	SelectedOptionID string           `json:"selected_option_id,omitempty"`
	FreeText         string           `json:"free_text,omitempty"`
}

func (DecisionCardBlock) UIBlockType() string { return "decision_card" }

func (b DecisionCardBlock) MarshalJSON() ([]byte, error) {
	type alias DecisionCardBlock
	next := alias(b)
	next.Type = b.UIBlockType()
	return json.Marshal(next)
}

type ToolStatusBlock struct {
	BaseBlock
	ToolCallID   string         `json:"tool_call_id"`
	ToolName     string         `json:"tool_name"`
	Label        string         `json:"label"`
	Status       string         `json:"status"`
	Summary      string         `json:"summary,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Arguments    map[string]any `json:"arguments,omitempty"`
	Result       map[string]any `json:"result,omitempty"`
}

func (ToolStatusBlock) UIBlockType() string { return "tool_status" }

func (b ToolStatusBlock) MarshalJSON() ([]byte, error) {
	type alias ToolStatusBlock
	next := alias(b)
	next.Type = b.UIBlockType()
	return json.Marshal(next)
}

type Attachment struct {
	AssetID      string `json:"asset_id"`
	NodeID       string `json:"node_id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Mime         string `json:"mime"`
	SizeBytes    int64  `json:"size_bytes"`
	URL          string `json:"url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

type AttachmentBlock struct {
	BaseBlock
	Attachments []Attachment `json:"attachments"`
}

func (AttachmentBlock) UIBlockType() string { return "attachment" }

func (b AttachmentBlock) MarshalJSON() ([]byte, error) {
	type alias AttachmentBlock
	next := alias(b)
	next.Type = b.UIBlockType()
	return json.Marshal(next)
}

type MediaBlock struct {
	BaseBlock
	AssetID      string `json:"asset_id"`
	NodeID       string `json:"node_id,omitempty"`
	Kind         string `json:"kind"`
	Title        string `json:"title,omitempty"`
	URL          string `json:"url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Mime         string `json:"mime,omitempty"`
}

func (MediaBlock) UIBlockType() string { return "media" }

func (b MediaBlock) MarshalJSON() ([]byte, error) {
	type alias MediaBlock
	next := alias(b)
	next.Type = b.UIBlockType()
	return json.Marshal(next)
}

type ErrorBlock struct {
	BaseBlock
	Title     string `json:"title"`
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

func (ErrorBlock) UIBlockType() string { return "error" }

func (b ErrorBlock) MarshalJSON() ([]byte, error) {
	type alias ErrorBlock
	next := alias(b)
	next.Type = b.UIBlockType()
	return json.Marshal(next)
}

func NewBaseBlock(id string, blockType string) BaseBlock {
	return BaseBlock{ID: strings.TrimSpace(id), Type: strings.TrimSpace(blockType)}
}

func ExtractMarkdownTexts(raw []byte) []string {
	var envelope struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Schema != SchemaV1 {
		return nil
	}
	out := make([]string, 0, len(envelope.Blocks))
	for _, block := range envelope.Blocks {
		if block.Type != "markdown" {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func ExtractAttachments(raw []byte) []Attachment {
	var envelope struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type        string       `json:"type"`
			Attachments []Attachment `json:"attachments"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Schema != SchemaV1 {
		return nil
	}
	out := []Attachment{}
	for _, block := range envelope.Blocks {
		if block.Type == "attachment" {
			out = append(out, block.Attachments...)
		}
	}
	return out
}
