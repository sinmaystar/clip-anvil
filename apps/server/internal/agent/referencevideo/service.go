package referencevideo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Store interface {
	GetMediaNodeByID(ctx context.Context, id pgtype.UUID) (db.MediaNode, error)
	GetMediaAssetByID(ctx context.Context, id pgtype.UUID) (db.MediaAsset, error)
	CreateReferenceVideoAnalysis(ctx context.Context, arg db.CreateReferenceVideoAnalysisParams) (db.ReferenceVideoAnalysis, error)
	MarkReferenceVideoAnalysisRunning(ctx context.Context, arg db.MarkReferenceVideoAnalysisRunningParams) (db.ReferenceVideoAnalysis, error)
	MarkReferenceVideoAnalysisSucceeded(ctx context.Context, arg db.MarkReferenceVideoAnalysisSucceededParams) (db.ReferenceVideoAnalysis, error)
	MarkReferenceVideoAnalysisFailed(ctx context.Context, arg db.MarkReferenceVideoAnalysisFailedParams) (db.ReferenceVideoAnalysis, error)
}

type Analyzer interface {
	AnalyzeReferenceVideo(ctx context.Context, input AnalyzerRequest) (AnalyzerResponse, error)
}

type SourceURLSigner interface {
	PresignedGetURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error)
}

type Service struct {
	store     Store
	analyzer  Analyzer
	urlSigner SourceURLSigner
}

func NewService(store Store, analyzer Analyzer) *Service {
	return &Service{store: store, analyzer: analyzer}
}

func (s *Service) WithSourceURLSigner(signer SourceURLSigner) *Service {
	s.urlSigner = signer
	return s
}

func (s *Service) Analyze(ctx context.Context, input AnalyzeInput) (AnalyzeOutput, error) {
	if s == nil || s.store == nil {
		return AnalyzeOutput{}, fmt.Errorf("reference video analysis service is not configured")
	}
	node, asset, err := s.loadSourceVideo(ctx, input)
	if err != nil {
		return AnalyzeOutput{}, err
	}
	brief := strings.TrimSpace(input.Brief)
	created, err := s.store.CreateReferenceVideoAnalysis(ctx, db.CreateReferenceVideoAnalysisParams{
		WorkspaceID:       input.WorkspaceID,
		SourceNodeID:      node.ID,
		Status:            StatusPending,
		Brief:             brief,
		Focus:             mustMarshalJSON(input.Focus),
		ModelProvider:     "",
		ModelID:           "",
		RequestSummary:    []byte(`{}`),
		Result:            []byte(`{}`),
		CreatedByThreadID: input.ThreadID,
		CreatedByTaskID:   input.TaskID,
	})
	if err != nil {
		return AnalyzeOutput{}, err
	}
	request := AnalyzerRequest{
		FixedProtocol:    FixedAnalysisProtocol,
		Brief:            brief,
		Focus:            input.Focus,
		AdaptationTarget: input.AdaptationTarget,
		Media: MediaEvidence{
			SourceNodeID: uuidString(node.ID),
			Title:        node.Title,
			Mime:         asset.Mime,
			StorageURL:   s.providerReachableURL(ctx, input.WorkspaceID, asset.StorageUrl),
		},
	}
	if s.analyzer == nil {
		err := fmt.Errorf("reference video analyzer is not configured")
		_, _ = s.markFailed(ctx, created.ID, defaultRequestSummary(request), err)
		return AnalyzeOutput{}, err
	}
	response, err := s.analyzer.AnalyzeReferenceVideo(ctx, request)
	requestSummary := response.RequestSummary
	if requestSummary == nil {
		requestSummary = defaultRequestSummary(request)
	}
	_, _ = s.store.MarkReferenceVideoAnalysisRunning(ctx, db.MarkReferenceVideoAnalysisRunningParams{
		ID:             created.ID,
		RequestSummary: mustMarshalJSON(requestSummary),
		ModelProvider:  response.ModelProvider,
		ModelID:        response.ModelID,
	})
	if err != nil {
		_, _ = s.markFailed(ctx, created.ID, requestSummary, err)
		return AnalyzeOutput{}, err
	}
	succeeded, err := s.store.MarkReferenceVideoAnalysisSucceeded(ctx, db.MarkReferenceVideoAnalysisSucceededParams{
		ID:             created.ID,
		RequestSummary: mustMarshalJSON(requestSummary),
		Result:         mustMarshalJSON(response.Result),
	})
	if err != nil {
		return AnalyzeOutput{}, err
	}
	return AnalyzeOutput{
		ID:       uuidString(succeeded.ID),
		Status:   succeeded.Status,
		Summary:  response.Result.Summary,
		Warnings: response.Result.Warnings,
	}, nil
}

func (s *Service) providerReachableURL(ctx context.Context, workspaceID pgtype.UUID, value pgtype.Text) string {
	raw := textString(value)
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || s.urlSigner == nil {
		return raw
	}
	key, err := storage.KeyFromStorageURL(workspaceID, raw)
	if err != nil {
		return raw
	}
	signed, err := s.urlSigner.PresignedGetURL(ctx, workspaceID, key, time.Hour)
	if err != nil || strings.TrimSpace(signed) == "" {
		return raw
	}
	return signed
}

func (s *Service) loadSourceVideo(ctx context.Context, input AnalyzeInput) (db.MediaNode, db.MediaAsset, error) {
	node, err := s.store.GetMediaNodeByID(ctx, input.SourceNodeID)
	if err != nil {
		return db.MediaNode{}, db.MediaAsset{}, err
	}
	if node.WorkspaceID != input.WorkspaceID || node.Source != "agent" || node.NodeType != db.NodeTypeVideo || !node.AssetID.Valid {
		return db.MediaNode{}, db.MediaAsset{}, ErrInvalidSourceVideo
	}
	asset, err := s.store.GetMediaAssetByID(ctx, node.AssetID)
	if err != nil {
		return db.MediaNode{}, db.MediaAsset{}, err
	}
	if asset.WorkspaceID != input.WorkspaceID || !strings.HasPrefix(asset.Mime, "video/") {
		return db.MediaNode{}, db.MediaAsset{}, ErrInvalidSourceVideo
	}
	return node, asset, nil
}

func (s *Service) markFailed(ctx context.Context, id pgtype.UUID, summary map[string]any, err error) (db.ReferenceVideoAnalysis, error) {
	return s.store.MarkReferenceVideoAnalysisFailed(ctx, db.MarkReferenceVideoAnalysisFailedParams{
		ID:             id,
		RequestSummary: mustMarshalJSON(summary),
		ErrorCode:      ErrorCodeAnalyzerFailed,
		ErrorMessage:   err.Error(),
	})
}

func defaultRequestSummary(request AnalyzerRequest) map[string]any {
	return map[string]any{
		"brief":          request.Brief,
		"focus":          request.Focus,
		"source_node_id": request.Media.SourceNodeID,
	}
}

func mustMarshalJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte(`null`)
	}
	return raw
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	id, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return ""
	}
	return id.String()
}

func textString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
