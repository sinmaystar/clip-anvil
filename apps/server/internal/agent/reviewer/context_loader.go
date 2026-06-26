package reviewer

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	agentpss "github.com/sinmaystar/clip-anvil/internal/agent/pss"
	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const maxReviewImageBytes = 10 << 20

type ContextStore interface {
	GetShotByID(ctx context.Context, id pgtype.UUID) (db.Shot, error)
	GetMediaNodeByID(ctx context.Context, id pgtype.UUID) (db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	GetGenerationJobByID(ctx context.Context, id pgtype.UUID) (db.GenerationJob, error)
	GetMediaAssetByID(ctx context.Context, id pgtype.UUID) (db.MediaAsset, error)
	ListReviewRecordsByShotPhase(ctx context.Context, params db.ListReviewRecordsByShotPhaseParams) ([]db.ReviewRecord, error)
}

type ImageObjectReader interface {
	ReadObject(ctx context.Context, workspaceID pgtype.UUID, key string, maxBytes int64) ([]byte, storage.ObjectRef, error)
}

type PSSBuilder interface {
	BuildProducerPSS(ctx context.Context, workspaceID pgtype.UUID) (agentpss.ProducerPSS, error)
}

type MessageRuntime interface {
	ListMessages(ctx context.Context, threadID pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error)
}

type ContextLoader struct {
	Store       ContextStore
	Runtime     MessageRuntime
	ImageReader ImageObjectReader
	PSSBuilder  PSSBuilder
}

const reviewerContextMessageLimit int32 = 1000

func (l ContextLoader) Load(ctx context.Context, input GraphInput) (Context, error) {
	if l.Store == nil || !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid {
		return Context{}, ErrInvalidInput
	}
	if input.Task.TargetPhase != TargetPhasePreviewImage && input.Task.TargetPhase != TargetPhaseShotVideo {
		return Context{}, fmt.Errorf("%w: unsupported review phase %q", ErrInvalidInput, input.Task.TargetPhase)
	}
	shotID, ok := pgUUIDFromString(input.Task.ShotID)
	if !ok {
		return Context{}, fmt.Errorf("%w: shot_id is required", ErrInvalidInput)
	}
	nodeID, ok := pgUUIDFromString(input.Task.NodeID)
	if !ok {
		return Context{}, fmt.Errorf("%w: node_id is required", ErrInvalidInput)
	}
	versionID, ok := pgUUIDFromString(input.Task.ArtifactVersionID)
	if !ok {
		return Context{}, fmt.Errorf("%w: artifact_version_id is required", ErrInvalidInput)
	}
	shot, err := l.Store.GetShotByID(ctx, shotID)
	if err != nil {
		return Context{}, err
	}
	node, err := l.Store.GetMediaNodeByID(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}
	version, err := l.Store.GetArtifactVersionByID(ctx, versionID)
	if err != nil {
		return Context{}, err
	}
	if shot.WorkspaceID != input.WorkspaceID || node.WorkspaceID != input.WorkspaceID || version.WorkspaceID != input.WorkspaceID {
		return Context{}, ErrInvalidInput
	}
	if node.ShotID.Valid && node.ShotID != shot.ID {
		return Context{}, ErrInvalidInput
	}
	if version.NodeID != node.ID {
		return Context{}, ErrInvalidInput
	}
	messages := []db.AgentMessage{}
	if l.Runtime != nil {
		messages, err = l.Runtime.ListMessages(ctx, input.ThreadID, 0, reviewerContextMessageLimit)
		if err != nil {
			return Context{}, err
		}
	}
	job := db.GenerationJob{}
	if jobID, ok := pgUUIDFromString(input.Task.GenerationJobID); ok {
		job, err = l.Store.GetGenerationJobByID(ctx, jobID)
		if err != nil {
			return Context{}, err
		}
	}
	assetURL, assetMime := "", ""
	if version.AssetID.Valid {
		asset, err := l.Store.GetMediaAssetByID(ctx, version.AssetID)
		if err != nil {
			return Context{}, err
		}
		assetURL, assetMime = l.modelAssetReference(ctx, asset)
	}
	priorReviews, err := l.Store.ListReviewRecordsByShotPhase(ctx, db.ListReviewRecordsByShotPhaseParams{
		WorkspaceID: input.WorkspaceID,
		ShotID:      shot.ID,
		TargetPhase: input.Task.TargetPhase,
	})
	if err != nil {
		return Context{}, err
	}
	productionText := ""
	if l.PSSBuilder != nil {
		pss, err := l.PSSBuilder.BuildProducerPSS(ctx, input.WorkspaceID)
		if err != nil {
			return Context{}, err
		}
		productionText = pss.Text
	}
	out := Context{
		Input:          input,
		Shot:           shot,
		Node:           node,
		Version:        version,
		GenerationJob:  job,
		Messages:       messages,
		PriorReviews:   priorReviews,
		ProductionText: productionText,
		AssetURL:       assetURL,
		AssetMime:      assetMime,
	}
	out.Text = buildReviewContextText(out)
	return out, nil
}

func (l ContextLoader) modelAssetReference(ctx context.Context, asset db.MediaAsset) (string, string) {
	rawURL := strings.TrimSpace(asset.StorageUrl.String)
	mime := strings.TrimSpace(asset.Mime)
	if rawURL == "" {
		return "", mime
	}
	if isModelAssetReference(rawURL) {
		return rawURL, mime
	}
	if l.ImageReader == nil {
		return "", mime
	}
	key, err := storage.KeyFromStorageURL(asset.WorkspaceID, rawURL)
	if err != nil {
		key = strings.TrimPrefix(rawURL, "minio://")
	}
	data, ref, err := l.ImageReader.ReadObject(ctx, asset.WorkspaceID, key, maxReviewImageBytes)
	if err != nil || len(data) == 0 {
		return "", mime
	}
	if mime == "" {
		mime = strings.TrimSpace(ref.MIME)
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), mime
}

func buildReviewContextText(reviewContext Context) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Review Target\n")
	fmt.Fprintf(&b, "- phase: %s\n", reviewContext.Input.Task.TargetPhase)
	fmt.Fprintf(&b, "- shot: %s %s status=%s\n", reviewContext.Shot.ClientKey, reviewContext.Shot.Title, reviewContext.Shot.Status)
	fmt.Fprintf(&b, "- node: %s type=%s status=%s\n", reviewContext.Node.Title, reviewContext.Node.NodeType, reviewContext.Node.Status)
	fmt.Fprintf(&b, "- version: v%d status=%s\n", reviewContext.Version.VersionNo, reviewContext.Version.Status)
	if strings.TrimSpace(reviewContext.GenerationJob.RenderedPrompt) != "" {
		fmt.Fprintf(&b, "- prompt: %s\n", strings.TrimSpace(reviewContext.GenerationJob.RenderedPrompt))
	}
	if len(reviewContext.PriorReviews) > 0 {
		fmt.Fprintf(&b, "\nPrior Reviews\n")
		for _, record := range reviewContext.PriorReviews {
			fmt.Fprintf(&b, "- %s score=%s critique=%s\n", record.Status, floatText(record.OverallScore), record.Critique)
		}
	}
	if strings.TrimSpace(reviewContext.ProductionText) != "" {
		fmt.Fprintf(&b, "\nProduction State\n%s\n", strings.TrimSpace(reviewContext.ProductionText))
	}
	return b.String()
}

func isModelAssetReference(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "data:image/")
}

func pgUUIDFromString(value string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func floatText(value pgtype.Float4) string {
	if !value.Valid {
		return "-"
	}
	return fmt.Sprintf("%.2f", value.Float32)
}
