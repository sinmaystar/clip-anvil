package production

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/sandbox"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type fakeSandboxFrameExtractor struct {
	input sandbox.ExtractFrameInput
	err   error
}

func (f *fakeSandboxFrameExtractor) ExtractFrame(ctx context.Context, input sandbox.ExtractFrameInput) (sandbox.SandboxJobResult, error) {
	f.input = input
	if f.err != nil {
		return sandbox.SandboxJobResult{
			Job: db.SandboxJob{ID: pgtype.UUID{Bytes: [16]byte{0xee}, Valid: true}},
		}, f.err
	}
	return sandbox.SandboxJobResult{
		Job: db.SandboxJob{ID: pgtype.UUID{Bytes: [16]byte{0xdd}, Valid: true}},
		Asset: sandbox.ArtifactObject{
			StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/frame.png",
		},
		MIME: "image/png",
		Size: 123,
	}, nil
}

func TestInternalFFmpegProviderMockExtractSuccess(t *testing.T) {
	provider := NewInternalFFmpegProvider(nil)
	result, err := provider.Run(context.Background(), GenerationIntent{
		OutputType:    "image",
		OperationType: "extract_first_frame",
		Model:         ModelSpec{Provider: "internal_ffmpeg", ModelID: "ffmpeg"},
		Params:        map[string]any{"mock_extract": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AssetMIME != "image/png" || len(result.AssetContent) == 0 {
		t.Fatalf("asset output = %s/%d", result.AssetMIME, len(result.AssetContent))
	}
}

func TestInternalFFmpegProviderUsesSandboxExtractor(t *testing.T) {
	extractor := &fakeSandboxFrameExtractor{}
	provider := NewInternalFFmpegProvider(extractor)
	result, err := provider.Run(context.Background(), GenerationIntent{
		WorkspaceID:    pgtype.UUID{Bytes: [16]byte{0xaa}, Valid: true},
		TargetNodeID:   pgtype.UUID{Bytes: [16]byte{0xbb}, Valid: true},
		OutputType:     "image",
		OperationType:  "extract_last_frame",
		Model:          ModelSpec{Provider: "internal_ffmpeg", ModelID: "ffmpeg"},
		Params:         map[string]any{},
		PromptTemplate: "extract tail",
		InputRefs: []InputRef{{
			Kind:       "dependency",
			NodeType:   "video",
			AssetID:    "asset-video",
			Mime:       "video/mp4",
			StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/video.mp4",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if extractor.input.Mode != sandbox.FrameLast {
		t.Fatalf("mode = %q, want last", extractor.input.Mode)
	}
	if result.AssetStorageURL == "" || result.AssetMIME != "image/png" || result.AssetSizeBytes != 123 {
		t.Fatalf("asset result = %#v", result)
	}
	if result.ProviderResponse["sandbox_job_id"] == "" {
		t.Fatalf("provider response missing sandbox job id: %#v", result.ProviderResponse)
	}
}

func TestInternalFFmpegProviderPersistsReadableFailure(t *testing.T) {
	provider := NewInternalFFmpegProvider(&fakeSandboxFrameExtractor{err: errors.New("ffmpeg failed")})
	_, err := provider.Run(context.Background(), GenerationIntent{
		WorkspaceID:   pgtype.UUID{Bytes: [16]byte{0xaa}, Valid: true},
		TargetNodeID:  pgtype.UUID{Bytes: [16]byte{0xbb}, Valid: true},
		OutputType:    "image",
		OperationType: "extract_last_frame",
		Model:         ModelSpec{Provider: "internal_ffmpeg", ModelID: "ffmpeg"},
		Params:        map[string]any{},
		InputRefs: []InputRef{{
			Kind:       "dependency",
			NodeType:   "video",
			AssetID:    "asset-video",
			Mime:       "video/mp4",
			StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/video.mp4",
		}},
	})
	if !errors.Is(err, ErrProviderExecution) {
		t.Fatalf("error = %v, want ErrProviderExecution", err)
	}
}
