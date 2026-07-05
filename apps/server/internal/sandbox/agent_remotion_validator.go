package sandbox

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type AgentRemotionValidationIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

type AgentRemotionValidationResult struct {
	Passed     bool                           `json:"passed"`
	Errors     []AgentRemotionValidationIssue `json:"errors"`
	Warnings   []AgentRemotionValidationIssue `json:"warnings"`
	SourceHash string                         `json:"source_hash"`
	PropsHash  string                         `json:"props_hash"`
}

var agentRemotionImportPattern = regexp.MustCompile(`\bimport(?:\s+[^"']+\s+from\s+|\s*)["']([^"']+)["']`)

func ValidateAgentRemotionSnapshot(snapshot AgentRemotionSnapshot) AgentRemotionValidationResult {
	result := AgentRemotionValidationResult{
		SourceHash: snapshot.SourceHash,
		PropsHash:  snapshot.PropsHash,
	}
	if err := validateAgentRemotionProps(snapshot.PropsJSON); err != nil {
		result.Errors = append(result.Errors, AgentRemotionValidationIssue{
			Severity: "error",
			Code:     "invalid_props_json",
			File:     AgentRemotionPropsFile,
			Message:  err.Error(),
		})
	}
	hasGeneratedComposition := false
	for _, file := range snapshot.Files {
		if file.Path == "GeneratedComposition.tsx" {
			hasGeneratedComposition = true
		}
		result.Errors = append(result.Errors, validateAgentRemotionSourceFile(file)...)
	}
	if !hasGeneratedComposition {
		result.Errors = append(result.Errors, AgentRemotionValidationIssue{
			Severity: "error",
			Code:     "missing_generated_composition",
			Message:  "GeneratedComposition.tsx is required",
		})
	}
	result.Passed = len(result.Errors) == 0
	return result
}

func validateAgentRemotionProps(data []byte) error {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	if _, ok := root["output"].(map[string]any); !ok {
		return fmt.Errorf("props_json.output object is required")
	}
	return nil
}

func validateAgentRemotionSourceFile(file AgentRemotionFile) []AgentRemotionValidationIssue {
	var issues []AgentRemotionValidationIssue
	lines := strings.Split(file.Content, "\n")
	for i, line := range lines {
		lineNo := i + 1
		for _, match := range agentRemotionImportPattern.FindAllStringSubmatch(line, -1) {
			if len(match) > 1 && !isAllowedAgentRemotionImport(match[1]) {
				issues = append(issues, AgentRemotionValidationIssue{
					Severity: "error",
					Code:     "forbidden_import",
					File:     file.Path,
					Line:     lineNo,
					Message:  "import " + match[1] + " is not allowed in agent Remotion renderer",
				})
			}
		}
		issues = append(issues, validateAgentRemotionForbiddenLine(file.Path, lineNo, line)...)
	}
	return issues
}

func isAllowedAgentRemotionImport(module string) bool {
	module = strings.TrimSpace(module)
	if module == "react" || module == "remotion" || module == "../runtime/safe" {
		return true
	}
	if strings.HasPrefix(module, "./") {
		return true
	}
	return false
}

func validateAgentRemotionForbiddenLine(filePath string, lineNo int, line string) []AgentRemotionValidationIssue {
	checks := []struct {
		code    string
		token   string
		message string
	}{
		{"dynamic_import", "import(", "dynamic import is not allowed"},
		{"require_call", "require(", "require is not allowed"},
		{"network_api", "fetch(", "network APIs are not allowed"},
		{"network_api", "XMLHttpRequest", "network APIs are not allowed"},
		{"network_api", "WebSocket", "network APIs are not allowed"},
		{"eval_call", "eval(", "eval is not allowed"},
		{"function_constructor", "Function(", "Function constructor is not allowed"},
		{"function_constructor", "new Function", "Function constructor is not allowed"},
		{"external_url", "http://", "external URLs are not allowed"},
		{"external_url", "https://", "external URLs are not allowed"},
	}
	compact := strings.ReplaceAll(line, " ", "")
	var issues []AgentRemotionValidationIssue
	for _, check := range checks {
		haystack := line
		needle := check.token
		if check.token == "import(" || check.token == "require(" || check.token == "fetch(" || check.token == "eval(" || check.token == "Function(" {
			haystack = compact
			needle = strings.ReplaceAll(check.token, " ", "")
		}
		if strings.Contains(haystack, needle) {
			issues = append(issues, AgentRemotionValidationIssue{
				Severity: "error",
				Code:     check.code,
				File:     filePath,
				Line:     lineNo,
				Message:  check.message,
			})
		}
	}
	return issues
}
