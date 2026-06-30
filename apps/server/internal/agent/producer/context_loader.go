package producer

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/agent/modelselection"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const maxAgentVisionImageBytes = 10 << 20
const minAgentVisionImageDimension = 14
const producerContextMessageLimit int32 = 1000

type ImageObjectReader interface {
	ReadObject(ctx context.Context, workspaceID pgtype.UUID, key string, maxBytes int64) ([]byte, storage.ObjectRef, error)
}

type ProducerMessageRuntime interface {
	ListMessages(ctx context.Context, threadID pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error)
}

type ProducerFactsProvider interface {
	LoadProducerFacts(ctx context.Context, workspaceID pgtype.UUID) ([]contextcompact.FullSummaryFact, []contextcompact.MediaCard, error)
}

type RuntimeContextLoader struct {
	Runtime        ProducerMessageRuntime
	Queries        *db.Queries
	ImageReader    ImageObjectReader
	Facts          ProducerFactsProvider
	ModelSelection interface {
		ResolveProducerModel(ctx context.Context, workspace db.Workspace) (modelselection.Option, error)
	}
}

func (l RuntimeContextLoader) LoadProducerContext(ctx context.Context, input ProducerTurnInput) (ProducerContext, error) {
	messages, err := l.Runtime.ListMessages(ctx, input.ThreadID, producerMessageWindowAfterSeq(input.TriggerMessageSeq, int64(producerContextMessageLimit)), producerContextMessageLimit)
	if err != nil {
		return ProducerContext{}, err
	}
	model, err := l.loadModel(ctx, input.WorkspaceID)
	if err != nil {
		return ProducerContext{}, err
	}
	imageAttachments := l.loadImageAttachments(ctx, messages)
	projectFacts, projectMediaCards, err := l.loadProjectFacts(ctx, input.WorkspaceID)
	if err != nil {
		return ProducerContext{}, err
	}
	return ProducerContext{
		Input:              input,
		Messages:           messages,
		LatestUserText:     latestUserTextFromMessages(messages),
		RuntimeTriggerText: strings.TrimSpace(input.RuntimeTriggerText),
		Model:              model,
		ImageAttachments:   imageAttachments,
		ProjectFacts:       projectFacts,
		ProjectMediaCards:  projectMediaCards,
		EmitDelta:          input.EmitDelta,
	}, nil
}

func producerMessageWindowAfterSeq(triggerSeq int64, limit int64) int64 {
	if triggerSeq <= 0 || limit <= 0 {
		return 0
	}
	afterSeq := triggerSeq - limit
	if afterSeq < 0 {
		return 0
	}
	return afterSeq
}

func (l RuntimeContextLoader) loadModel(ctx context.Context, workspaceID pgtype.UUID) (ProducerModelSelection, error) {
	if l.Queries == nil || l.ModelSelection == nil {
		return ProducerModelSelection{}, nil
	}
	workspace, err := l.Queries.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return ProducerModelSelection{}, err
	}
	option, err := l.ModelSelection.ResolveProducerModel(ctx, workspace)
	if err != nil {
		return ProducerModelSelection{}, AgentError{Code: "agent_model_unavailable", Message: "resolve Producer model", Cause: err}
	}
	return ProducerModelSelection{
		ProviderID:          option.ProviderID,
		ModelID:             option.ModelID,
		DisplayName:         option.DisplayName,
		ReasoningEffort:     option.DefaultReasoningEffort,
		SupportsThinking:    option.SupportsThinking,
		MaxCompletionTokens: option.MaxCompletionTokens,
	}, nil
}

func (l RuntimeContextLoader) loadProjectFacts(ctx context.Context, workspaceID pgtype.UUID) ([]contextcompact.FullSummaryFact, []contextcompact.MediaCard, error) {
	if l.Facts == nil || !workspaceID.Valid {
		return nil, nil, nil
	}
	return l.Facts.LoadProducerFacts(ctx, workspaceID)
}

func (l RuntimeContextLoader) loadImageAttachments(ctx context.Context, messages []db.AgentMessage) map[string]ProducerImageAttachment {
	out := map[string]ProducerImageAttachment{}
	if l.Queries == nil {
		return out
	}
	for _, msg := range messages {
		for _, attachment := range uimessage.ExtractAttachments(msg.Content) {
			if strings.TrimSpace(attachment.Kind) != "image" {
				continue
			}
			assetID := strings.TrimSpace(attachment.AssetID)
			if assetID == "" {
				continue
			}
			assetUUID, ok := pgUUIDFromString(assetID)
			if !ok {
				continue
			}
			asset, err := l.Queries.GetMediaAssetByID(ctx, assetUUID)
			if err != nil || !asset.StorageUrl.Valid || strings.TrimSpace(asset.StorageUrl.String) == "" {
				continue
			}
			imageURL, mime, ok := l.modelImageReference(ctx, asset)
			if !ok {
				continue
			}
			out[assetID] = ProducerImageAttachment{
				AssetID: assetID,
				NodeID:  strings.TrimSpace(attachment.NodeID),
				Name:    strings.TrimSpace(attachment.Name),
				URL:     imageURL,
				Mime:    mime,
			}
		}
	}
	return out
}

func (l RuntimeContextLoader) modelImageReference(ctx context.Context, asset db.MediaAsset) (string, string, bool) {
	rawURL := strings.TrimSpace(asset.StorageUrl.String)
	mime := strings.TrimSpace(asset.Mime)
	if isModelImageReference(rawURL) {
		return rawURL, mime, true
	}
	if l.ImageReader == nil {
		return "", "", false
	}
	key, err := storage.KeyFromStorageURL(asset.WorkspaceID, rawURL)
	if err != nil {
		return "", "", false
	}
	data, ref, err := l.ImageReader.ReadObject(ctx, asset.WorkspaceID, key, maxAgentVisionImageBytes)
	if err != nil {
		return "", "", false
	}
	if !agentVisionImageDimensionsAllowed(data) {
		return "", "", false
	}
	if mime == "" {
		mime = strings.TrimSpace(ref.MIME)
	}
	return imageDataURL(mime, data), mime, true
}

func isModelImageReference(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "data:image/")
}

func imageDataURL(mime string, data []byte) string {
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func agentVisionImageDimensionsAllowed(data []byte) bool {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return false
	}
	return config.Width >= minAgentVisionImageDimension &&
		config.Height >= minAgentVisionImageDimension
}

func latestUserTextFromMessages(messages []db.AgentMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		texts := uimessage.ExtractMarkdownTexts(messages[i].Content)
		if len(texts) > 0 && strings.TrimSpace(texts[len(texts)-1]) != "" {
			return strings.TrimSpace(texts[len(texts)-1])
		}
	}
	return ""
}

func pgUUIDFromString(value string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}
