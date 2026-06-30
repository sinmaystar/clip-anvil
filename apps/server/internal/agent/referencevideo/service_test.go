package referencevideo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestServiceRejectsNonVideoSourceNode(t *testing.T) {
	store := &fakeStore{
		node: db.MediaNode{
			ID:          uuidWithByte(1),
			WorkspaceID: uuidWithByte(2),
			NodeType:    db.NodeTypeImage,
			Source:      "agent",
			AssetID:     uuidWithByte(3),
		},
		asset: db.MediaAsset{ID: uuidWithByte(3), WorkspaceID: uuidWithByte(2), Mime: "image/png"},
	}
	service := NewService(store, &fakeAnalyzer{})
	_, err := service.Analyze(context.Background(), AnalyzeInput{
		WorkspaceID:  uuidWithByte(2),
		SourceNodeID: uuidWithByte(1),
		Brief:        "分析参考视频",
	})
	if !errors.Is(err, ErrInvalidSourceVideo) {
		t.Fatalf("err = %v, want ErrInvalidSourceVideo", err)
	}
}

func TestServicePersistsSucceededAnalysis(t *testing.T) {
	workspaceID := uuidWithByte(2)
	nodeID := uuidWithByte(1)
	assetID := uuidWithByte(3)
	store := &fakeStore{
		node: db.MediaNode{
			ID:          nodeID,
			WorkspaceID: workspaceID,
			NodeType:    db.NodeTypeVideo,
			Source:      "agent",
			AssetID:     assetID,
			Title:       "reference.mp4",
		},
		asset: db.MediaAsset{
			ID:          assetID,
			WorkspaceID: workspaceID,
			Type:        db.AssetTypeVideo,
			Mime:        "video/mp4",
			StorageUrl:  pgtype.Text{String: "workspace/reference.mp4", Valid: true},
		},
	}
	analyzer := &fakeAnalyzer{
		result: AnalysisResult{
			Summary: "快三秒痛点 hook 后展示产品。",
			ReferenceIntent: ReferenceIntent{
				Preserve:       []string{"痛点 hook"},
				MustBeOriginal: []string{"商品外观"},
			},
			Warnings: []string{"不要复制原字幕。"},
		},
	}
	service := NewService(store, analyzer)
	out, err := service.Analyze(context.Background(), AnalyzeInput{
		WorkspaceID:  workspaceID,
		ThreadID:     uuidWithByte(4),
		TaskID:       uuidWithByte(5),
		SourceNodeID: nodeID,
		Brief:        "借鉴脚本和运镜",
		Focus:        []string{"hook", "camera_language"},
		AdaptationTarget: map[string]any{
			"product": "悦行行李箱",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != StatusSucceeded || out.Summary != "快三秒痛点 hook 后展示产品。" {
		t.Fatalf("out = %#v", out)
	}
	if store.created.Brief != "借鉴脚本和运镜" || store.created.Status != StatusPending {
		t.Fatalf("created = %#v", store.created)
	}
	if store.running.ID != store.created.ID {
		t.Fatalf("running id = %s, created id = %s", store.running.ID, store.created.ID)
	}
	if store.succeeded.ID != store.created.ID {
		t.Fatalf("succeeded id = %s, created id = %s", store.succeeded.ID, store.created.ID)
	}
	if !strings.Contains(analyzer.last.FixedProtocol, "must_be_original") {
		t.Fatalf("fixed protocol missing copyright boundary: %s", analyzer.last.FixedProtocol)
	}
	if analyzer.last.Brief != "借鉴脚本和运镜" {
		t.Fatalf("brief = %q", analyzer.last.Brief)
	}
}

func TestServicePersistsFailedAnalysis(t *testing.T) {
	workspaceID := uuidWithByte(2)
	nodeID := uuidWithByte(1)
	assetID := uuidWithByte(3)
	store := &fakeStore{
		node: db.MediaNode{
			ID:          nodeID,
			WorkspaceID: workspaceID,
			NodeType:    db.NodeTypeVideo,
			Source:      "agent",
			AssetID:     assetID,
		},
		asset: db.MediaAsset{
			ID:          assetID,
			WorkspaceID: workspaceID,
			Type:        db.AssetTypeVideo,
			Mime:        "video/mp4",
			StorageUrl:  pgtype.Text{String: "workspace/reference.mp4", Valid: true},
		},
	}
	service := NewService(store, &fakeAnalyzer{err: errors.New("model rejected")})
	_, err := service.Analyze(context.Background(), AnalyzeInput{
		WorkspaceID:  workspaceID,
		SourceNodeID: nodeID,
		Brief:        "分析参考视频",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if store.failed.Status != StatusFailed || store.failed.ErrorCode != ErrorCodeAnalyzerFailed {
		t.Fatalf("failed = %#v", store.failed)
	}
}

type fakeAnalyzer struct {
	result AnalysisResult
	err    error
	last   AnalyzerRequest
}

func (f *fakeAnalyzer) AnalyzeReferenceVideo(_ context.Context, input AnalyzerRequest) (AnalyzerResponse, error) {
	f.last = input
	if f.err != nil {
		return AnalyzerResponse{}, f.err
	}
	return AnalyzerResponse{
		ModelProvider:  "mock",
		ModelID:        "mock-reference-video-analyzer",
		RequestSummary: map[string]any{"brief": input.Brief},
		Result:         f.result,
	}, nil
}

type fakeStore struct {
	node      db.MediaNode
	asset     db.MediaAsset
	created   db.ReferenceVideoAnalysis
	running   db.ReferenceVideoAnalysis
	succeeded db.ReferenceVideoAnalysis
	failed    db.ReferenceVideoAnalysis
}

func (f *fakeStore) GetMediaNodeByID(context.Context, pgtype.UUID) (db.MediaNode, error) {
	return f.node, nil
}

func (f *fakeStore) GetMediaAssetByID(context.Context, pgtype.UUID) (db.MediaAsset, error) {
	return f.asset, nil
}

func (f *fakeStore) CreateReferenceVideoAnalysis(_ context.Context, arg db.CreateReferenceVideoAnalysisParams) (db.ReferenceVideoAnalysis, error) {
	f.created = db.ReferenceVideoAnalysis{
		ID:           uuidWithByte(10),
		WorkspaceID:  arg.WorkspaceID,
		SourceNodeID: arg.SourceNodeID,
		Status:       arg.Status,
		Brief:        arg.Brief,
		Focus:        arg.Focus,
	}
	return f.created, nil
}

func (f *fakeStore) MarkReferenceVideoAnalysisRunning(_ context.Context, arg db.MarkReferenceVideoAnalysisRunningParams) (db.ReferenceVideoAnalysis, error) {
	f.running = f.created
	f.running.ID = arg.ID
	f.running.Status = StatusRunning
	f.running.RequestSummary = arg.RequestSummary
	f.running.ModelProvider = arg.ModelProvider
	f.running.ModelID = arg.ModelID
	return f.running, nil
}

func (f *fakeStore) MarkReferenceVideoAnalysisSucceeded(_ context.Context, arg db.MarkReferenceVideoAnalysisSucceededParams) (db.ReferenceVideoAnalysis, error) {
	f.succeeded = f.running
	f.succeeded.ID = arg.ID
	f.succeeded.Status = StatusSucceeded
	f.succeeded.Result = arg.Result
	f.succeeded.RequestSummary = arg.RequestSummary
	return f.succeeded, nil
}

func (f *fakeStore) MarkReferenceVideoAnalysisFailed(_ context.Context, arg db.MarkReferenceVideoAnalysisFailedParams) (db.ReferenceVideoAnalysis, error) {
	f.failed = f.created
	f.failed.ID = arg.ID
	f.failed.Status = StatusFailed
	f.failed.RequestSummary = arg.RequestSummary
	f.failed.ErrorCode = arg.ErrorCode
	f.failed.ErrorMessage = arg.ErrorMessage
	return f.failed, nil
}

func uuidWithByte(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{value, value, value, value, value, value, value, value, value, value, value, value, value, value, value, value}, Valid: true}
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
