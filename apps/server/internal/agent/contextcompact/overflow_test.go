package contextcompact

import (
	"errors"
	"testing"
)

func TestIsContextOverflowErrorClassifiesProviderMessages(t *testing.T) {
	for _, msg := range []string{
		"context length exceeded",
		"maximum context length",
		"tokens exceed context window",
		"prompt is too long",
	} {
		if !IsContextOverflowError(errors.New(msg)) {
			t.Fatalf("expected overflow for %q", msg)
		}
	}
	for _, msg := range []string{"network timeout", "rate limit exceeded", "invalid api key"} {
		if IsContextOverflowError(errors.New(msg)) {
			t.Fatalf("expected non-overflow for %q", msg)
		}
	}
}
