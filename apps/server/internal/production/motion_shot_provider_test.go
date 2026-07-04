package production

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/sandbox"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type fakeMotionShotRenderer struct {
	input sandbox.RenderMotionShotInput
	err   error
}

func (f *fakeMotionShotRenderer) RenderMotionShot(_ context.Context, input sandbox.RenderMotionShotInput) (sandbox.SandboxJobResult, error) {
	f.input = input
	if f.err != nil {
		return sandbox.SandboxJobResult{}, f.err
	}
	return sandbox.SandboxJobResult{
		Job:   db.SandboxJob{ID: pgtype.UUID{Bytes: [16]byte{0x44}, Valid: true}},
		Asset: sandbox.ArtifactObject{StorageURL: "minio://workspace/production/motion.mp4"},
		MIME:  "video/mp4",
		Size:  12345,
	}, nil
}

func TestMotionShotProviderUsesSandboxRenderer(t *testing.T) {
	renderer := &fakeMotionShotRenderer{}
	provider := NewMotionShotProvider(renderer)
	result, err := provider.Run(context.Background(), GenerationIntent{
		WorkspaceID:    pgtype.UUID{Bytes: [16]byte{0x11}, Valid: true},
		TargetNodeID:   pgtype.UUID{Bytes: [16]byte{0x22}, Valid: true},
		OutputType:     "video",
		OperationType:  "image_to_motion_video",
		PromptTemplate: "Create motion shot",
		Model:          ModelSpec{Provider: "internal_motion_video", ModelID: "remotion-motion-shot-v1"},
		Params:         map[string]any{"duration_sec": float64(5), "ratio": "9:16", "resolution": "1080p", "fps": float64(30), "motion_style": "premium_product_ad"},
		InputRefs: []InputRef{{
			NodeType:   "image",
			AssetID:    "asset-product",
			StorageURL: "minio://workspace/source/product.png",
			Mime:       "image/png",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if renderer.input.Plan.MotionStyle != "premium_product_ad" || renderer.input.Meta.Width != 1080 || renderer.input.Meta.Height != 1920 {
		t.Fatalf("renderer input = %#v", renderer.input)
	}
	if result.AssetStorageURL == "" || result.AssetMIME != "video/mp4" || result.AssetSizeBytes != 12345 {
		t.Fatalf("result = %#v", result)
	}
	if result.AssetMetadata["provider"] != "internal_motion_video" ||
		result.AssetMetadata["rendering_family"] != "motion_shot_video" ||
		result.AssetMetadata["renderer_engine"] != "remotion" {
		t.Fatalf("metadata = %#v", result.AssetMetadata)
	}
	if result.ProviderRequest["provider"] != "internal_motion_video" ||
		result.ProviderRequest["model_id"] != "remotion-motion-shot-v1" ||
		result.ProviderRequest["asset_count"] != 1 {
		t.Fatalf("request = %#v", result.ProviderRequest)
	}
}

func TestMotionShotProviderRequiresImageInput(t *testing.T) {
	provider := NewMotionShotProvider(&fakeMotionShotRenderer{})
	_, err := provider.Run(context.Background(), GenerationIntent{
		OutputType:    "video",
		OperationType: "image_to_motion_video",
		Model:         ModelSpec{Provider: "internal_motion_video", ModelID: "remotion-motion-shot-v1"},
	})
	if !errors.Is(err, ErrCapabilityMismatch) || !strings.Contains(err.Error(), "requires an image input") {
		t.Fatalf("error = %v", err)
	}
}

func TestMotionShotProviderRejectsUnsupportedOperation(t *testing.T) {
	provider := NewMotionShotProvider(&fakeMotionShotRenderer{})
	_, err := provider.Run(context.Background(), GenerationIntent{
		OutputType:    "video",
		OperationType: "text_to_video",
		Model:         ModelSpec{Provider: "internal_motion_video", ModelID: "remotion-motion-shot-v1"},
	})
	if !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("error = %v", err)
	}
}

func TestMotionShotProviderRequiresRenderer(t *testing.T) {
	provider := NewMotionShotProvider(nil)
	_, err := provider.Run(context.Background(), GenerationIntent{
		OutputType:    "video",
		OperationType: "image_to_motion_video",
		Model:         ModelSpec{Provider: "internal_motion_video", ModelID: "remotion-motion-shot-v1"},
		InputRefs:     []InputRef{{NodeType: "image", StorageURL: "minio://workspace/source/product.png", Mime: "image/png"}},
	})
	if !errors.Is(err, ErrProviderConfig) {
		t.Fatalf("error = %v", err)
	}
}

func TestProviderRegistryResolvesInternalMotionVideoByDefault(t *testing.T) {
	registry := NewProviderRegistry(ProviderConfig{ProviderMode: "real"})
	provider, err := registry.Resolve(GenerationIntent{
		Model: ModelSpec{Provider: "internal_motion_video", ModelID: "remotion-motion-shot-v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(MotionShotProvider); !ok {
		t.Fatalf("provider = %T, want MotionShotProvider", provider)
	}
}
