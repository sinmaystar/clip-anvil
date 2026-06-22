package producer

import "fmt"

var ErrAgentModelUnavailable = NewAgentError("agent_model_unavailable", "agent model unavailable")

type AgentError struct {
	Code    string
	Message string
	Cause   error
}

func NewAgentError(code, message string) AgentError {
	return AgentError{Code: code, Message: message}
}

func (e AgentError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e AgentError) Unwrap() error {
	return e.Cause
}

func (e AgentError) Is(target error) bool {
	other, ok := target.(AgentError)
	return ok && e.Code == other.Code
}

func errorCode(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	if agentErr, ok := err.(AgentError); ok && agentErr.Code != "" {
		return agentErr.Code
	}
	return fallback
}
