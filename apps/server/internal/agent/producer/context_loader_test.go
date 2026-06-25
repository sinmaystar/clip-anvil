package producer

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestModelImageReferenceConvertsWorkspaceStorageURLToDataURL(t *testing.T) {
	workspaceID := testPgUUID()
	reader := fakeImageReader{
		data: testPNG(t, 32, 32),
		ref:  storage.ObjectRef{MIME: "image/png"},
	}
	loader := RuntimeContextLoader{ImageReader: &reader}

	url, mime, ok := loader.modelImageReference(context.Background(), db.MediaAsset{
		WorkspaceID: workspaceID,
		Mime:        "image/png",
		StorageUrl:  pgtype.Text{String: storage.StorageURL(workspaceID, "uploads/product.png"), Valid: true},
	})

	if !ok {
		t.Fatal("modelImageReference ok = false, want true")
	}
	if mime != "image/png" {
		t.Fatalf("mime = %q, want image/png", mime)
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("url = %q, want data URL", url)
	}
	if reader.key != "uploads/product.png" {
		t.Fatalf("reader key = %q, want uploads/product.png", reader.key)
	}
}

func TestModelImageReferenceKeepsHTTPURL(t *testing.T) {
	loader := RuntimeContextLoader{}

	url, mime, ok := loader.modelImageReference(context.Background(), db.MediaAsset{
		Mime:       "image/jpeg",
		StorageUrl: pgtype.Text{String: "https://assets.example.com/product.jpg", Valid: true},
	})

	if !ok {
		t.Fatal("modelImageReference ok = false, want true")
	}
	if url != "https://assets.example.com/product.jpg" || mime != "image/jpeg" {
		t.Fatalf("url/mime = %q/%q", url, mime)
	}
}

func TestModelImageReferenceRejectsInternalStorageURLWithoutReader(t *testing.T) {
	workspaceID := testPgUUID()
	loader := RuntimeContextLoader{}

	_, _, ok := loader.modelImageReference(context.Background(), db.MediaAsset{
		WorkspaceID: workspaceID,
		Mime:        "image/png",
		StorageUrl:  pgtype.Text{String: storage.StorageURL(workspaceID, "uploads/product.png"), Valid: true},
	})

	if ok {
		t.Fatal("modelImageReference ok = true, want false")
	}
}

func TestModelImageReferenceRejectsTooSmallWorkspaceImage(t *testing.T) {
	workspaceID := testPgUUID()
	reader := fakeImageReader{
		data: testPNG(t, 1, 1),
		ref:  storage.ObjectRef{MIME: "image/png"},
	}
	loader := RuntimeContextLoader{ImageReader: &reader}

	_, _, ok := loader.modelImageReference(context.Background(), db.MediaAsset{
		WorkspaceID: workspaceID,
		Mime:        "image/png",
		StorageUrl:  pgtype.Text{String: storage.StorageURL(workspaceID, "uploads/tiny.png"), Valid: true},
	})

	if ok {
		t.Fatal("modelImageReference ok = true, want false")
	}
}

func TestRuntimeContextLoaderLoadsRecentMessagesThroughTrigger(t *testing.T) {
	threadID := uuidWithByte(2)
	messages := make([]db.AgentMessage, 0, 1001)
	for seq := int64(1); seq <= 1001; seq++ {
		text := "old"
		if seq == 1001 {
			text = "现在什么进展了"
		}
		messages = append(messages, db.AgentMessage{
			ThreadID:    threadID,
			Seq:         seq,
			Role:        "user",
			MessageType: "text",
			Content:     mustUserContent(t, uimessage.UserMessageInput{Text: text}),
		})
	}
	runtime := &fakeProducerContextRuntime{messages: messages}
	loader := RuntimeContextLoader{Runtime: runtime}

	out, err := loader.LoadProducerContext(context.Background(), ProducerTurnInput{
		ThreadID:          threadID,
		TriggerMessageSeq: 1001,
	})
	if err != nil {
		t.Fatal(err)
	}

	if runtime.afterSeq != 1 {
		t.Fatalf("afterSeq = %d, want 1", runtime.afterSeq)
	}
	if runtime.limit != 1000 {
		t.Fatalf("limit = %d, want 1000", runtime.limit)
	}
	if len(out.Messages) != 1000 {
		t.Fatalf("messages len = %d, want 1000", len(out.Messages))
	}
	if out.Messages[0].Seq != 2 || out.Messages[len(out.Messages)-1].Seq != 1001 {
		t.Fatalf("message seq window = %d..%d, want 2..1001", out.Messages[0].Seq, out.Messages[len(out.Messages)-1].Seq)
	}
	if out.LatestUserText != "现在什么进展了" {
		t.Fatalf("latest user text = %q", out.LatestUserText)
	}
}

type fakeProducerContextRuntime struct {
	messages []db.AgentMessage
	afterSeq int64
	limit    int32
}

func (f *fakeProducerContextRuntime) ListMessages(_ context.Context, _ pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error) {
	f.afterSeq = afterSeq
	f.limit = limit
	out := make([]db.AgentMessage, 0, limit)
	for _, message := range f.messages {
		if message.Seq <= afterSeq {
			continue
		}
		out = append(out, message)
		if len(out) >= int(limit) {
			break
		}
	}
	return out, nil
}

type fakeImageReader struct {
	key  string
	data []byte
	ref  storage.ObjectRef
}

func (f *fakeImageReader) ReadObject(_ context.Context, _ pgtype.UUID, key string, _ int64) ([]byte, storage.ObjectRef, error) {
	f.key = key
	return f.data, f.ref, nil
}

func testPgUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true}
}

func testPNG(t *testing.T, width int, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 220, G: 30, B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
