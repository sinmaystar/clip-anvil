package production

import (
	"context"
	"errors"
	"io"
	"testing"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeArkChatStreamer struct {
	chunks []*schema.Message
	err    error
}

func (f fakeArkChatStreamer) Stream(context.Context, []*schema.Message, ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
	if f.err != nil {
		return nil, f.err
	}
	return schema.StreamReaderFromArray(f.chunks), nil
}

func TestVolcengineTextRuntimeStreamsDeltasAndFinalText(t *testing.T) {
	runtime := newVolcengineTextRuntimeForTest(
		VolcengineProviderConfig{
			APIKey:    "test-key",
			BaseURL:   "https://example.invalid/api/v3",
			Region:    "cn-beijing",
			TextModel: "doubao-seed-2-0-mini-260428",
		},
		fakeArkChatStreamer{chunks: []*schema.Message{
			schema.AssistantMessage("A quiet", nil),
			schema.AssistantMessage(" studio glows.", nil),
		}},
	)

	stream, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{
		OperationType:  "text_generation",
		PromptTemplate: "Write one short sentence about a quiet studio.",
		Model:          ModelSpec{Provider: "volcengine", ModelID: "doubao-seed-2-0-mini-260428"},
		Params:         map[string]any{"temperature": 0.2, "max_tokens": 64},
	})
	if err != nil {
		t.Fatal(err)
	}

	deltas := []string{}
	var final ProductionOutput
	for event := range stream {
		switch event.Type {
		case ProductionEventModelStreamDelta:
			deltas = append(deltas, event.Payload["delta"].(string))
		case ProductionEventJobSucceeded:
			final = event.Output
		case ProductionEventJobFailed:
			t.Fatalf("unexpected failed event: %v", event.Err)
		}
	}

	if len(deltas) != 2 || deltas[0] != "A quiet" || deltas[1] != " studio glows." {
		t.Fatalf("deltas = %#v", deltas)
	}
	if final.TextContent != "A quiet studio glows." {
		t.Fatalf("final text = %q", final.TextContent)
	}
	if final.RequestSummary["provider"] != "volcengine" {
		t.Fatalf("request summary = %#v", final.RequestSummary)
	}
	if final.RequestSummary["prompt"] != "Write one short sentence about a quiet studio." {
		t.Fatalf("request summary prompt = %#v", final.RequestSummary["prompt"])
	}
}

func TestVolcengineTextRuntimeFailsWithoutAPIKey(t *testing.T) {
	runtime := NewVolcengineTextRuntime(VolcengineProviderConfig{
		TextModel: "doubao-seed-2-0-mini-260428",
	})
	_, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{
		OperationType:  "text_generation",
		PromptTemplate: "write a short ad",
		Model:          ModelSpec{Provider: "volcengine", ModelID: "doubao-seed-2-0-mini-260428"},
		Params:         map[string]any{},
	})
	if !errors.Is(err, ErrProviderConfig) {
		t.Fatalf("error = %v, want ErrProviderConfig", err)
	}
}

func TestVolcengineTextRuntimeEmitsFailureEventOnStreamError(t *testing.T) {
	runtime := newVolcengineTextRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", TextModel: "doubao-seed-2-0-mini-260428"},
		fakeArkChatStreamer{err: io.ErrUnexpectedEOF},
	)
	stream, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{
		OperationType:  "text_generation",
		PromptTemplate: "write a short ad",
		Model:          ModelSpec{Provider: "volcengine", ModelID: "doubao-seed-2-0-mini-260428"},
		Params:         map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	event := <-stream
	if event.Type != ProductionEventJobFailed || !errors.Is(event.Err, ErrProviderExecution) {
		t.Fatalf("event = %#v", event)
	}
}
