package sandbox

import (
	"errors"
	"path"
	"strings"
)

type TextEditInput struct {
	Mode    string
	Content string
	OldText string
	NewText string
}

func ValidateWorkspaceTextPath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("path is required")
	}
	if !strings.HasPrefix(input, DefaultWorkdir+"/") {
		return "", errors.New("path must be inside /workspace")
	}
	clean := path.Clean(input)
	if clean == DefaultWorkdir || !strings.HasPrefix(clean, DefaultWorkdir+"/") {
		return "", errors.New("path must be a file inside /workspace")
	}
	return clean, nil
}

func ApplyTextEdit(existing string, input TextEditInput) (string, error) {
	switch strings.TrimSpace(input.Mode) {
	case "create", "create_or_overwrite":
		return input.Content, nil
	case "append":
		return existing + input.Content, nil
	case "replace":
		if input.OldText == "" {
			return "", errors.New("old_text is required for replace")
		}
		count := strings.Count(existing, input.OldText)
		if count == 0 {
			return "", errors.New("old_text was not found")
		}
		if count > 1 {
			return "", errors.New("old_text must match exactly once")
		}
		return strings.Replace(existing, input.OldText, input.NewText, 1), nil
	default:
		return "", errors.New("mode must be create, create_or_overwrite, append, or replace")
	}
}
