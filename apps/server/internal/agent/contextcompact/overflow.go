package contextcompact

import (
	"strings"
)

func IsContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	if text == "" {
		return false
	}
	for _, phrase := range []string{
		"context length exceeded",
		"maximum context length",
		"context window",
		"prompt is too long",
		"tokens exceed",
		"too many tokens",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
