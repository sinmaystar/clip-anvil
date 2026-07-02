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

type fakeTemplateVideoRenderer struct {
	input sandbox.RenderTemplateVideoInput
	err   error
}

func (f *fakeTemplateVideoRenderer) RenderTemplateVideo(ctx context.Context, input sandbox.RenderTemplateVideoInput) (sandbox.SandboxJobResult, error) {
	f.input = input
	if f.err != nil {
		return sandbox.SandboxJobResult{
			Job: db.SandboxJob{ID: pgtype.UUID{Bytes: [16]byte{0xee}, Valid: true}},
		}, f.err
	}
	return sandbox.SandboxJobResult{
		Job: db.SandboxJob{ID: pgtype.UUID{Bytes: [16]byte{0xdd}, Valid: true}},
		Asset: sandbox.ArtifactObject{
			StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/template.mp4",
		},
		MIME: "video/mp4",
		Size: 456,
	}, nil
}

func TestTemplateVideoProviderUsesSandboxRenderer(t *testing.T) {
	renderer := &fakeTemplateVideoRenderer{}
	provider := NewTemplateVideoProvider(renderer)
	result, err := provider.Run(context.Background(), GenerationIntent{
		WorkspaceID:   pgtype.UUID{Bytes: [16]byte{0xaa}, Valid: true},
		TargetNodeID:  pgtype.UUID{Bytes: [16]byte{0xbb}, Valid: true},
		OutputType:    "video",
		OperationType: "image_to_template_video",
		Model:         ModelSpec{Provider: "internal_template_video", ModelID: "hyperframes-html"},
		Params: map[string]any{
			"template_key": "static_fallback_ken_burns_v1",
			"duration_sec": float64(5),
			"ratio":        "9:16",
			"resolution":   "1080p",
			"variables": map[string]any{
				"headline": "轻松出发",
				"cta":      "了解更多",
			},
		},
		InputRefs: []InputRef{{
			NodeType:   "image",
			AssetID:    "product-image",
			Mime:       "image/png",
			StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/product.png",
			ModelRole:  "reference_image",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if renderer.input.TemplateKey != "static_fallback_ken_burns_v1" || len(renderer.input.Assets) != 1 {
		t.Fatalf("renderer input = %#v", renderer.input)
	}
	if renderer.input.Assets[0].WorkspacePath == "" || !strings.Contains(renderer.input.HTML, renderer.input.Assets[0].WorkspacePath) {
		t.Fatalf("renderer asset path not wired into html: input=%#v html=%s", renderer.input, renderer.input.HTML)
	}
	if strings.HasPrefix(renderer.input.Assets[0].WorkspacePath, "/workspace/") {
		t.Fatalf("template html should use project-relative asset path, got %q", renderer.input.Assets[0].WorkspacePath)
	}
	if result.AssetMIME != "video/mp4" || result.AssetStorageURL == "" || result.AssetSizeBytes != 456 {
		t.Fatalf("result = %#v", result)
	}
	if result.ProviderRequest["template_key"] != "static_fallback_ken_burns_v1" ||
		result.ProviderResponse["sandbox_job_id"] == "" ||
		result.AssetMetadata["rendering_family"] != "template_video" ||
		result.AssetMetadata["template_engine"] != "hyperframes" {
		t.Fatalf("metadata = %#v %#v %#v", result.ProviderRequest, result.ProviderResponse, result.AssetMetadata)
	}
}

func TestTemplateVideoProviderRequiresImageForImageToTemplateVideo(t *testing.T) {
	provider := NewTemplateVideoProvider(&fakeTemplateVideoRenderer{})
	_, err := provider.Run(context.Background(), GenerationIntent{
		WorkspaceID:   pgtype.UUID{Bytes: [16]byte{0xaa}, Valid: true},
		TargetNodeID:  pgtype.UUID{Bytes: [16]byte{0xbb}, Valid: true},
		OutputType:    "video",
		OperationType: "image_to_template_video",
		Model:         ModelSpec{Provider: "internal_template_video", ModelID: "hyperframes-html"},
		Params: map[string]any{
			"template_key": "marketing_ad_4_scene_v1",
			"variables": map[string]any{
				"headline": "悦行行李箱",
			},
		},
	})
	if !errors.Is(err, ErrCapabilityMismatch) || !strings.Contains(err.Error(), "product image") {
		t.Fatalf("error = %v", err)
	}
}

func TestTemplateVideoProviderRequiresRenderer(t *testing.T) {
	provider := NewTemplateVideoProvider(nil)
	_, err := provider.Run(context.Background(), GenerationIntent{
		OutputType:    "video",
		OperationType: "template_to_video",
		Model:         ModelSpec{Provider: "internal_template_video", ModelID: "hyperframes-html"},
		Params:        map[string]any{"template_key": "static_fallback_ken_burns_v1"},
	})
	if !errors.Is(err, ErrProviderConfig) {
		t.Fatalf("error = %v, want ErrProviderConfig", err)
	}
}

func TestTemplateVideoProviderRejectsUnknownTemplate(t *testing.T) {
	provider := NewTemplateVideoProvider(&fakeTemplateVideoRenderer{})
	_, err := provider.Run(context.Background(), GenerationIntent{
		OutputType:    "video",
		OperationType: "template_to_video",
		Model:         ModelSpec{Provider: "internal_template_video", ModelID: "hyperframes-html"},
		Params:        map[string]any{"template_key": "unknown"},
	})
	if !errors.Is(err, ErrCapabilityMismatch) || !strings.Contains(err.Error(), "unknown template_key") {
		t.Fatalf("error = %v", err)
	}
}

func TestProviderRegistryResolvesInternalTemplateVideoByDefault(t *testing.T) {
	registry := NewProviderRegistry(ProviderConfig{})
	provider, err := registry.Resolve(GenerationIntent{
		Model: ModelSpec{Provider: "internal_template_video", ModelID: "hyperframes-html"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(TemplateVideoProvider); !ok {
		t.Fatalf("provider = %T, want TemplateVideoProvider", provider)
	}
}
