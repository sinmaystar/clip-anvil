package reviewer

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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	agentprompt "github.com/sinmaystar/clip-anvil/internal/agent/prompt"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type arkChatStreamer interface {
	Stream(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.StreamReader[*schema.Message], error)
}

type arkChatModelFactory func(ctx context.Context, config *ark.ChatModelConfig) (arkChatStreamer, error)

type VolcengineModelResponderConfig struct {
	APIKey           string
	BaseURL          string
	Region           string
	Model            string
	MaxTokens        int
	Temperature      float32
	Factory          arkChatModelFactory
	ContextCompactor contextcompact.Middleware
}

type VolcengineModelResponder struct {
	cfg     VolcengineModelResponderConfig
	factory arkChatModelFactory
}

func NewVolcengineModelResponder(cfg VolcengineModelResponderConfig) VolcengineModelResponder {
	factory := cfg.Factory
	if factory == nil {
		factory = func(ctx context.Context, config *ark.ChatModelConfig) (arkChatStreamer, error) {
			return ark.NewChatModel(ctx, config)
		}
	}
	return VolcengineModelResponder{cfg: cfg, factory: factory}
}

func (r VolcengineModelResponder) Respond(ctx context.Context, reviewContext Context) (ReviewerTurnOutput, error) {
	apiKey := strings.TrimSpace(r.cfg.APIKey)
	if apiKey == "" {
		return ReviewerTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for Reviewer model")
	}
	modelID := strings.TrimSpace(r.cfg.Model)
	if modelID == "" {
		return ReviewerTurnOutput{}, fmt.Errorf("reviewer model is required")
	}
	config := &ark.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: strings.TrimSpace(r.cfg.BaseURL),
		Region:  strings.TrimSpace(r.cfg.Region),
		Model:   modelID,
		Timeout: durationPtr(10 * time.Minute),
	}
	if r.cfg.MaxTokens > 0 {
		config.MaxTokens = &r.cfg.MaxTokens
	}
	if r.cfg.Temperature > 0 {
		config.Temperature = &r.cfg.Temperature
	}
	model, err := r.factory(ctx, config)
	if err != nil {
		return ReviewerTurnOutput{}, fmt.Errorf("create reviewer ark chat model: %w", err)
	}
	streamer := model
	if len(reviewContext.ToolInfos) > 0 {
		toolCallingModel, ok := model.(einoModel.ToolCallingChatModel)
		if !ok {
			return ReviewerTurnOutput{}, fmt.Errorf("selected Reviewer model does not support tool calling")
		}
		boundModel, err := toolCallingModel.WithTools(reviewContext.ToolInfos)
		if err != nil {
			return ReviewerTurnOutput{}, fmt.Errorf("bind reviewer tools: %w", err)
		}
		streamer = boundModel
	}
	prompt := reviewToolPromptMessagesWithBoundaries(reviewContext)
	messages := prompt.Messages
	facts, mediaCards := reviewerContextCompactionFacts(reviewContext)
	var compacted contextcompact.ProjectionOutput
	if r.cfg.ContextCompactor != nil {
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       reviewContext.Input.WorkspaceID,
			ThreadID:          reviewContext.Input.ThreadID,
			TaskID:            reviewContext.Input.TaskID,
			Role:              "reviewer",
			ModelID:           modelID,
			Messages:          messages,
			MessageRefs:       prompt.MessageRefs,
			ToolInfos:         reviewContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "reviewer_before_model",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
		})
		if err != nil {
			return ReviewerTurnOutput{}, fmt.Errorf("compact reviewer context: %w", err)
		}
		messages = compacted.Messages
	}
	retriedContextOverflow := false
	stream, err := streamer.Stream(ctx, messages)
	if err != nil && contextcompact.IsContextOverflowError(err) && r.cfg.ContextCompactor != nil {
		retriedContextOverflow = true
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       reviewContext.Input.WorkspaceID,
			ThreadID:          reviewContext.Input.ThreadID,
			TaskID:            reviewContext.Input.TaskID,
			Role:              "reviewer",
			ModelID:           modelID,
			Messages:          prompt.Messages,
			MessageRefs:       prompt.MessageRefs,
			ToolInfos:         reviewContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "model_error_context_overflow",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
			ForceFullCompact:  true,
		})
		if err != nil {
			return ReviewerTurnOutput{}, fmt.Errorf("compact reviewer context after overflow: %w", err)
		}
		messages = compacted.Messages
		stream, err = streamer.Stream(ctx, messages)
	}
	if err != nil {
		return ReviewerTurnOutput{}, fmt.Errorf("stream reviewer ark chat model: %w", err)
	}
	defer stream.Close()

	chunks := []*schema.Message{}
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ReviewerTurnOutput{}, fmt.Errorf("receive reviewer ark chat stream: %w", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	final, err := schema.ConcatMessages(chunks)
	if err != nil {
		return ReviewerTurnOutput{}, fmt.Errorf("concatenate reviewer ark chat stream: %w", err)
	}
	metadata := map[string]any{
		"provider":               "volcengine",
		"model_id":               modelID,
		"native_tool_call_count": len(final.ToolCalls),
	}
	enrichReviewerContextCompactionMetadata(metadata, compacted)
	if retriedContextOverflow {
		metadata["context_compaction_retry"] = true
	}
	return ReviewerTurnOutput{
		AssistantText: strings.TrimSpace(final.Content),
		Metadata:      metadata,
		ModelMessage:  final,
	}, nil
}

type reviewPromptBoundary struct {
	Messages          []*schema.Message
	MessageRefs       []contextcompact.SourceMessageRef
	SameTurnFromIndex int
	PendingFromIndex  int
}

func reviewToolPromptMessages(reviewContext Context) []*schema.Message {
	return reviewToolPromptMessagesWithBoundaries(reviewContext).Messages
}

func reviewToolPromptMessagesWithBoundaries(reviewContext Context) reviewPromptBoundary {
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: SystemPrompt(),
		},
	}
	messageRefs := []contextcompact.SourceMessageRef{}
	for _, source := range reviewContext.Messages {
		for _, message := range agentprompt.HistoryMessages([]db.AgentMessage{source}) {
			messages = append(messages, message)
			messageRefs = appendReviewPromptMessageRef(messageRefs, len(messages)-1, source.ID)
		}
	}
	messages = append(messages, reviewUserMessage(reviewContext))
	sameTurnFromIndex := contextcompact.CurrentToolLoopFromIndex(len(messages), len(reviewContext.SameTurnMessages))
	for _, message := range reviewContext.SameTurnMessages {
		switch message.Role {
		case "assistant":
			messages = append(messages, &schema.Message{
				Role:    schema.Assistant,
				Content: message.Content,
				ToolCalls: []schema.ToolCall{{
					ID:   message.ToolCallID,
					Type: "function",
					Function: schema.FunctionCall{
						Name:      message.ToolName,
						Arguments: string(mustJSON(message.ToolArguments)),
					},
				}},
			})
		case "tool":
			messages = append(messages, &schema.Message{
				Role:       schema.Tool,
				Content:    message.Content,
				ToolCallID: message.ToolCallID,
				ToolName:   message.ToolName,
			})
		}
	}
	pendingFromIndex := contextcompact.PendingReminderTargetIndex(messages, reviewContext.PendingReminders)
	messages = agentprompt.AppendPendingReminders(messages, reviewContext.PendingReminders)
	return reviewPromptBoundary{
		Messages:          messages,
		MessageRefs:       messageRefs,
		SameTurnFromIndex: sameTurnFromIndex,
		PendingFromIndex:  pendingFromIndex,
	}
}

func appendReviewPromptMessageRef(refs []contextcompact.SourceMessageRef, index int, id pgtype.UUID) []contextcompact.SourceMessageRef {
	if !id.Valid {
		return refs
	}
	return append(refs, contextcompact.SourceMessageRef{MessageIndex: index, MessageID: id})
}

func enrichReviewerContextCompactionMetadata(metadata map[string]any, output contextcompact.ProjectionOutput) {
	if len(output.Applied) == 0 {
		return
	}
	metadata["context_compaction_applied"] = true
	metadata["context_compaction_mode"] = output.CompactionMode
	metadata["context_compaction_count"] = len(output.Applied)
	metadata["context_compaction_refs"] = output.CompactionRefs
	metadata["context_compaction_detail_files"] = output.DetailFiles
}

func reviewUserMessage(reviewContext Context) *schema.Message {
	text := strings.TrimSpace(reviewContext.Text)
	if text == "" {
		text = "Review the attached generated artifact."
	}
	if strings.TrimSpace(reviewContext.AssetURL) == "" {
		return schema.UserMessage(text)
	}
	url := strings.TrimSpace(reviewContext.AssetURL)
	mime := strings.TrimSpace(reviewContext.AssetMime)
	if mime == "" && strings.HasPrefix(url, "data:image/") {
		mime = strings.TrimPrefix(strings.SplitN(strings.TrimPrefix(url, "data:"), ";", 2)[0], "data:")
	}
	if mime != "" && !strings.HasPrefix(mime, "image/") {
		return schema.UserMessage(strings.TrimSpace(text + "\n\nArtifact URL: " + url + "\nArtifact MIME: " + mime))
	}
	parts := []schema.MessageInputPart{
		{Type: schema.ChatMessagePartTypeText, Text: text},
		{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{MessagePartCommon: schema.MessagePartCommon{
				URL:      &url,
				MIMEType: mime,
			}},
		},
	}
	return &schema.Message{
		Role:                  schema.User,
		UserInputMultiContent: parts,
	}
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}
