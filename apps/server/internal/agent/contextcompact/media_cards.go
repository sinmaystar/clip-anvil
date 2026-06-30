package contextcompact

import (
	"fmt"
	"strings"
)

type MediaCard struct {
	Ref         string
	Kind        string
	Role        string
	Status      string
	Summary     string
	DurationSec float64
	SandboxPath string
	SourceRef   string
	TokenWeight int
}

func MediaCardPromptText(card MediaCard) string {
	kind := strings.TrimSpace(card.Kind)
	summary := strings.TrimSpace(card.Summary)
	if summary == "" {
		summary = mediaSummaryFallback(kind)
	}
	lines := []string{
		"media_ref=" + strings.TrimSpace(card.Ref),
		"kind=" + kind,
		"role=" + strings.TrimSpace(card.Role),
		"status=" + strings.TrimSpace(card.Status),
		"source=" + strings.TrimSpace(card.SourceRef),
		"summary=" + summary,
	}
	if card.DurationSec > 0 {
		lines = append(lines, fmt.Sprintf("duration_sec=%.2f", card.DurationSec))
	}
	if strings.TrimSpace(card.SandboxPath) != "" {
		lines = append(lines, "sandbox_path="+strings.TrimSpace(card.SandboxPath))
	}
	return strings.Join(lines, "\n")
}

func mediaSummaryFallback(kind string) string {
	switch strings.TrimSpace(kind) {
	case "audio":
		return "未生成音频摘要"
	case "video", "image":
		return "未生成视觉摘要"
	default:
		return "未生成媒体摘要"
	}
}
