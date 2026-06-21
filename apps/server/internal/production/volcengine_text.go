package production

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type arkChatStreamer interface {
	Stream(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.StreamReader[*schema.Message], error)
}

type arkChatModelFactory func(ctx context.Context, config *ark.ChatModelConfig) (arkChatStreamer, error)

type VolcengineTextRuntime struct {
	cfg     VolcengineProviderConfig
	factory arkChatModelFactory
}

func NewVolcengineTextRuntime(cfg VolcengineProviderConfig) VolcengineTextRuntime {
	return VolcengineTextRuntime{
		cfg: cfg,
		factory: func(ctx context.Context, config *ark.ChatModelConfig) (arkChatStreamer, error) {
			return ark.NewChatModel(ctx, config)
		},
	}
}

func newVolcengineTextRuntimeForTest(cfg VolcengineProviderConfig, model arkChatStreamer) VolcengineTextRuntime {
	return VolcengineTextRuntime{
		cfg: cfg,
		factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return model, nil
		},
	}
}

func (r VolcengineTextRuntime) Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error) {
	if strings.TrimSpace(r.cfg.APIKey) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for provider volcengine", ErrProviderConfig)
	}
	modelID := strings.TrimSpace(intent.Model.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(r.cfg.TextModel)
	}
	if modelID == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL is required", ErrProviderConfig)
	}
	model, err := r.factory(ctx, &ark.ChatModelConfig{
		APIKey:      r.cfg.APIKey,
		BaseURL:     strings.TrimSpace(r.cfg.BaseURL),
		Region:      strings.TrimSpace(r.cfg.Region),
		Model:       modelID,
		MaxTokens:   intParamPtr(intent.Params, "max_tokens"),
		Temperature: float32ParamPtr(intent.Params, "temperature"),
		Timeout:     durationPtr(10 * time.Minute),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create ark chat model: %v", ErrProviderUnavailable, err)
	}
	events := make(chan ProductionEvent, 16)
	go r.stream(ctx, model, job, intent, events)
	return events, nil
}

func (r VolcengineTextRuntime) stream(ctx context.Context, model arkChatStreamer, job ProductionJob, intent GenerationIntent, events chan<- ProductionEvent) {
	defer close(events)
	rendered := strings.TrimSpace(intent.EffectivePrompt())
	if rendered == "" {
		rendered = "empty prompt"
	}
	stream, err := model.Stream(ctx, []*schema.Message{
		schema.SystemMessage("You are ClipAnvil's production text generation engine. Return concise production-ready text."),
		schema.UserMessage(rendered),
	})
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: stream ark chat model: %v", ErrProviderExecution, err)}
		return
	}
	defer stream.Close()

	chunks := []*schema.Message{}
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: receive ark chat stream: %v", ErrProviderExecution, err)}
			return
		}
		if chunk == nil {
			continue
		}
		chunks = append(chunks, chunk)
		if chunk.Content != "" {
			events <- ProductionEvent{
				JobID:        job.ID,
				WorkspaceID:  job.WorkspaceID,
				TargetNodeID: job.TargetNodeID,
				Type:         ProductionEventModelStreamDelta,
				Progress:     50,
				Payload:      map[string]any{"delta": chunk.Content},
			}
		}
	}
	final, err := schema.ConcatMessages(chunks)
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: concatenate ark chat stream: %v", ErrProviderExecution, err)}
		return
	}
	response := map[string]any{
		"provider": "volcengine",
		"model_id": intent.Model.ModelID,
	}
	if requestID := ark.GetArkRequestID(final); requestID != "" {
		response["request_id"] = requestID
	}
	if final.ResponseMeta != nil {
		response["finish_reason"] = final.ResponseMeta.FinishReason
		if final.ResponseMeta.Usage != nil {
			response["usage"] = final.ResponseMeta.Usage
		}
	}
	events <- ProductionEvent{
		JobID:        job.ID,
		WorkspaceID:  job.WorkspaceID,
		TargetNodeID: job.TargetNodeID,
		Type:         ProductionEventJobSucceeded,
		Progress:     100,
		Output: ProductionOutput{
			RenderedPrompt: rendered,
			TextContent:    final.Content,
			RequestSummary: map[string]any{
				"provider":       "volcengine",
				"model_id":       intent.Model.ModelID,
				"operation_type": intent.OperationType,
				"prompt":         rendered,
				"params":         intent.Params,
			},
			ResponseSummary: response,
		},
	}
}

func intParamPtr(params map[string]any, key string) *int {
	value, ok := numericParam(params, key)
	if !ok {
		return nil
	}
	out := int(value)
	return &out
}

func float32ParamPtr(params map[string]any, key string) *float32 {
	value, ok := numericParam(params, key)
	if !ok {
		return nil
	}
	out := float32(value)
	return &out
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}
