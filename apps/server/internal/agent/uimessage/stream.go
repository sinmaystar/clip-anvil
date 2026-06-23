package uimessage

import "strings"

const (
	BlockIDAnswer   = "blk_answer"
	BlockIDThinking = "blk_thinking"
)

type StreamDeltaInput struct {
	WorkspaceID string
	ThreadID    string
	TaskID      string
	MessageID   string
	BlockID     string
	BlockType   string
	Delta       string
	Sequence    int64
}

type StreamDeltaPayload struct {
	WorkspaceID string `json:"workspace_id"`
	ThreadID    string `json:"thread_id"`
	TaskID      string `json:"task_id"`
	MessageID   string `json:"message_id,omitempty"`
	BlockID     string `json:"block_id"`
	BlockType   string `json:"block_type"`
	Delta       string `json:"delta"`
	Sequence    int64  `json:"sequence"`
}

func ShouldShowThinking(supportsThinking bool, effort string) bool {
	if !supportsThinking {
		return false
	}
	switch strings.TrimSpace(effort) {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

func NewStreamDelta(input StreamDeltaInput) StreamDeltaPayload {
	return StreamDeltaPayload(input)
}
