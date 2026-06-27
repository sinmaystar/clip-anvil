package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentpss "github.com/sinmaystar/clip-anvil/internal/agent/pss"
	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestContextLoaderBuildsPreviewReviewContext(t *testing.T) {
	store := &fakeReviewContextStore{
		shot:    db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", Title: "开场", Status: "preview_ready"},
		node:    db.MediaNode{ID: uuidWithByte(3), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), Title: "shot-01 preview", NodeType: "image", CurrentVersionID: uuidWithByte(4)},
		version: db.ArtifactVersion{ID: uuidWithByte(4), WorkspaceID: uuidWithByte(1), NodeID: uuidWithByte(3), AssetID: uuidWithByte(5), VersionNo: 1, Status: db.JobStatusSucceeded},
		job:     db.GenerationJob{ID: uuidWithByte(6), WorkspaceID: uuidWithByte(1), TargetNodeID: uuidWithByte(3), RenderedPrompt: "bright product shot", Status: db.JobStatusSucceeded},
		asset:   db.MediaAsset{ID: uuidWithByte(5), WorkspaceID: uuidWithByte(1), Type: db.AssetTypeImage, Mime: "image/png", StorageUrl: pgtype.Text{String: "minio://internal/product.png", Valid: true}},
		reviews: []db.ReviewRecord{
			{ID: uuidWithByte(7), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), NodeID: uuidWithByte(3), ArtifactVersionID: uuidWithByte(4), TargetPhase: "preview_image", Status: "rejected", Critique: "商品太小"},
		},
	}
	loader := ContextLoader{
		Store: store,
		Runtime: fakeReviewerMessageRuntime{messages: []db.AgentMessage{
			{ID: uuidWithByte(10), Role: "assistant", MessageType: "text", Content: []byte(`{"schema":"clipanvil.agent.message.v1","blocks":[{"type":"markdown","text":"上一轮 Reviewer 已提醒产品偏小"}]}`)},
		}},
		ImageReader: fakeReviewImageReader{
			data: []byte{137, 80, 78, 71},
			ref:  storage.ObjectRef{MIME: "image/png"},
		},
		PSSBuilder: fakeReviewPSSBuilder{text: "当前项目\n- Workspace: test"},
	}

	out, err := loader.Load(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		TaskID:      uuidWithByte(9),
		Task: TaskInput{
			TargetPhase:       "preview_image",
			ShotID:            uuidString(uuidWithByte(2)),
			NodeID:            uuidString(uuidWithByte(3)),
			ArtifactVersionID: uuidString(uuidWithByte(4)),
			GenerationJobID:   uuidString(uuidWithByte(6)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Shot.ClientKey != "shot-01" || out.Node.Title != "shot-01 preview" {
		t.Fatalf("context = %#v", out)
	}
	if !strings.HasPrefix(out.AssetURL, "data:image/png;base64,") {
		t.Fatalf("asset url = %q", out.AssetURL)
	}
	if len(out.Messages) != 1 || !strings.Contains(string(out.Messages[0].Content), "产品偏小") {
		t.Fatalf("messages = %#v", out.Messages)
	}
	for _, want := range []string{"shot-01", "bright product shot", "商品太小", "当前项目"} {
		if !strings.Contains(out.Text, want) {
			t.Fatalf("context text missing %q: %s", want, out.Text)
		}
	}
}

type fakeReviewContextStore struct {
	shot    db.Shot
	node    db.MediaNode
	version db.ArtifactVersion
	job     db.GenerationJob
	asset   db.MediaAsset
	reviews []db.ReviewRecord
}

func (f *fakeReviewContextStore) GetShotByID(context.Context, pgtype.UUID) (db.Shot, error) {
	return f.shot, nil
}

func (f *fakeReviewContextStore) GetMediaNodeByID(context.Context, pgtype.UUID) (db.MediaNode, error) {
	return f.node, nil
}

func (f *fakeReviewContextStore) GetArtifactVersionByID(context.Context, pgtype.UUID) (db.ArtifactVersion, error) {
	return f.version, nil
}

func (f *fakeReviewContextStore) GetGenerationJobByID(context.Context, pgtype.UUID) (db.GenerationJob, error) {
	return f.job, nil
}

func (f *fakeReviewContextStore) GetMediaAssetByID(context.Context, pgtype.UUID) (db.MediaAsset, error) {
	return f.asset, nil
}

func (f *fakeReviewContextStore) ListReviewRecordsByShotPhase(context.Context, db.ListReviewRecordsByShotPhaseParams) ([]db.ReviewRecord, error) {
	return f.reviews, nil
}

type fakeReviewImageReader struct {
	data []byte
	ref  storage.ObjectRef
}

func (f fakeReviewImageReader) ReadObject(context.Context, pgtype.UUID, string, int64) ([]byte, storage.ObjectRef, error) {
	return f.data, f.ref, nil
}

type fakeReviewPSSBuilder struct {
	text string
}

func (f fakeReviewPSSBuilder) BuildProducerPSS(context.Context, pgtype.UUID) (agentpss.ProducerPSS, error) {
	return agentpss.ProducerPSS{Text: f.text}, nil
}

type fakeReviewerMessageRuntime struct {
	messages []db.AgentMessage
}

func (f fakeReviewerMessageRuntime) ListMessages(context.Context, pgtype.UUID, int64, int32) ([]db.AgentMessage, error) {
	return f.messages, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}
