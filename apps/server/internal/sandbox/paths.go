package sandbox

import (
	"errors"
	"path"
	"strings"
)

func ValidateOutputPath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("path is required")
	}
	if !strings.HasPrefix(input, OutputDir+"/") {
		return "", errors.New("artifact path must be inside /workspace/output")
	}
	clean := path.Clean(input)
	if clean == OutputDir || !strings.HasPrefix(clean, OutputDir+"/") {
		return "", errors.New("artifact path must be a file inside /workspace/output")
	}
	return clean, nil
}
