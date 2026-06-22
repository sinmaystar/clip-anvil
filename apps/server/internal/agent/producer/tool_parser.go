package producer

import (
	"encoding/json"
	"errors"
	"strings"
)

var ErrInvalidToolCall = errors.New("invalid tool call")

const (
	functionCallBegin = "<|FunctionCallBegin|>"
	functionCallEnd   = "<|FunctionCallEnd|>"
)

type ParsedToolCall struct {
	HasToolCall bool
	ToolCall    ToolCall
	Text        string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

func ParseToolCall(text string) (ParsedToolCall, error) {
	trimmed := strings.TrimSpace(text)
	if candidate, ok := extractFunctionCallCandidate(trimmed); ok {
		return parseFunctionCallCandidate(candidate, trimmed)
	}
	candidate := extractJSONCandidate(trimmed)
	var envelope struct {
		ToolCall *struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"tool_call"`
	}
	if err := json.Unmarshal([]byte(candidate), &envelope); err != nil {
		return ParsedToolCall{Text: trimmed}, nil
	}
	if envelope.ToolCall == nil {
		return ParsedToolCall{Text: trimmed}, nil
	}
	if strings.TrimSpace(envelope.ToolCall.Name) == "" || envelope.ToolCall.Arguments == nil {
		return ParsedToolCall{}, ErrInvalidToolCall
	}
	return ParsedToolCall{
		HasToolCall: true,
		ToolCall: ToolCall{
			Name:      strings.TrimSpace(envelope.ToolCall.Name),
			Arguments: envelope.ToolCall.Arguments,
		},
	}, nil
}

func parseFunctionCallCandidate(candidate string, original string) (ParsedToolCall, error) {
	var calls []struct {
		Name       string         `json:"name"`
		Parameters map[string]any `json:"parameters"`
		Arguments  map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(candidate), &calls); err != nil {
		return ParsedToolCall{Text: original}, nil
	}
	if len(calls) == 0 {
		return ParsedToolCall{}, ErrInvalidToolCall
	}
	call := calls[0]
	args := call.Parameters
	if args == nil {
		args = call.Arguments
	}
	if strings.TrimSpace(call.Name) == "" || args == nil {
		return ParsedToolCall{}, ErrInvalidToolCall
	}
	return ParsedToolCall{
		HasToolCall: true,
		ToolCall: ToolCall{
			Name:      strings.TrimSpace(call.Name),
			Arguments: args,
		},
	}, nil
}

func extractFunctionCallCandidate(text string) (string, bool) {
	start := strings.Index(text, functionCallBegin)
	if start < 0 {
		return "", false
	}
	rest := text[start+len(functionCallBegin):]
	end := strings.Index(rest, functionCallEnd)
	if end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest), true
}

func extractJSONCandidate(text string) string {
	start := strings.Index(text, "```")
	if start < 0 {
		return text
	}
	rest := text[start+3:]
	rest = strings.TrimPrefix(rest, "json")
	end := strings.Index(rest, "```")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}
