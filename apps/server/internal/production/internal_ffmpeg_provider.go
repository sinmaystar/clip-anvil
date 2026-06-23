package production

import (
	"context"
	"fmt"
	"strings"

	"github.com/sinmaystar/clip-anvil/internal/sandbox"
)

type FrameExtractionMode string

const (
	FrameExtractionFirst FrameExtractionMode = "first"
	FrameExtractionLast  FrameExtractionMode = "last"
)

type SandboxFrameExtractor interface {
	ExtractFrame(ctx context.Context, input sandbox.ExtractFrameInput) (sandbox.SandboxJobResult, error)
}

type SandboxVideoComposer interface {
	ComposeVideos(ctx context.Context, input sandbox.ComposeVideosInput) (sandbox.SandboxJobResult, error)
}

type InternalFFmpegProvider struct {
	extractor SandboxFrameExtractor
	composer  SandboxVideoComposer
}

func NewInternalFFmpegProvider(operator any) InternalFFmpegProvider {
	provider := InternalFFmpegProvider{}
	if extractor, ok := operator.(SandboxFrameExtractor); ok {
		provider.extractor = extractor
	}
	if composer, ok := operator.(SandboxVideoComposer); ok {
		provider.composer = composer
	}
	return provider
}

func (p InternalFFmpegProvider) Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error) {
	if intent.OperationType == "compose_final_video" {
		return p.runCompose(ctx, intent)
	}
	if boolParam(intent.Params, "mock_extract_fail") {
		return ProviderResult{}, fmt.Errorf("%w: mock internal ffmpeg failure", ErrProviderExecution)
	}
	mode, err := frameModeForOperation(intent.OperationType)
	if err != nil {
		return ProviderResult{}, err
	}
	if boolParam(intent.Params, "mock_extract") {
		return ProviderResult{
			RenderedPrompt: intent.EffectivePrompt(),
			AssetContent:   onePixelPNG,
			AssetMIME:      "image/png",
			AssetMetadata: map[string]any{
				"provider":       "internal_ffmpeg",
				"operation_type": intent.OperationType,
				"mock_extract":   true,
			},
			ProviderRequest: map[string]any{
				"provider":       "internal_ffmpeg",
				"operation_type": intent.OperationType,
				"mode":           mode,
			},
			ProviderResponse: map[string]any{"mime": "image/png", "mock_extract": true},
		}, nil
	}
	if p.extractor == nil {
		return ProviderResult{}, fmt.Errorf("%w: sandbox frame extractor is not configured", ErrProviderConfig)
	}
	source, ok := videoInputRef(intent.InputRefs)
	if !ok {
		return ProviderResult{}, fmt.Errorf("%w: missing video input", ErrCapabilityMismatch)
	}
	result, err := p.extractor.ExtractFrame(ctx, sandbox.ExtractFrameInput{
		WorkspaceID:  intent.WorkspaceID,
		TargetNodeID: intent.TargetNodeID,
		Source: sandbox.SandboxAssetInput{
			AssetID:    source.AssetID,
			StorageURL: source.StorageURL,
			Mime:       source.Mime,
		},
		Mode: sandboxFrameMode(mode),
	})
	response := map[string]any{
		"sandbox_job_id": uuidToString(result.Job.ID),
		"mime":           result.MIME,
		"size_bytes":     result.Size,
	}
	if err != nil {
		return ProviderResult{}, ProviderRunError{
			Err:      fmt.Errorf("%w: %v", ErrProviderExecution, err),
			Response: response,
		}
	}
	return ProviderResult{
		RenderedPrompt:  intent.EffectivePrompt(),
		AssetStorageURL: result.Asset.StorageURL,
		AssetMIME:       result.MIME,
		AssetSizeBytes:  result.Size,
		AssetMetadata: map[string]any{
			"provider":       "internal_ffmpeg",
			"operation_type": intent.OperationType,
			"sandbox_job_id": uuidToString(result.Job.ID),
		},
		ProviderRequest: map[string]any{
			"provider":        "internal_ffmpeg",
			"operation_type":  intent.OperationType,
			"source_asset_id": source.AssetID,
			"mode":            mode,
		},
		ProviderResponse: response,
	}, nil
}

func (p InternalFFmpegProvider) runCompose(ctx context.Context, intent GenerationIntent) (ProviderResult, error) {
	if boolParam(intent.Params, "mock_compose") {
		return ProviderResult{
			RenderedPrompt: intent.EffectivePrompt(),
			AssetContent:   mockMP4,
			AssetMIME:      "video/mp4",
			AssetMetadata: map[string]any{
				"provider":       "internal_ffmpeg",
				"operation_type": intent.OperationType,
				"mock_compose":   true,
			},
			ProviderRequest: map[string]any{
				"provider":       "internal_ffmpeg",
				"operation_type": intent.OperationType,
			},
			ProviderResponse: map[string]any{"mime": "video/mp4", "mock_compose": true},
		}, nil
	}
	if p.composer == nil {
		return ProviderResult{}, fmt.Errorf("%w: sandbox video composer is not configured", ErrProviderConfig)
	}
	sources := videoInputRefs(intent.InputRefs)
	if len(sources) == 0 {
		return ProviderResult{}, fmt.Errorf("%w: missing video inputs", ErrCapabilityMismatch)
	}
	result, err := p.composer.ComposeVideos(ctx, sandbox.ComposeVideosInput{
		WorkspaceID:  intent.WorkspaceID,
		TargetNodeID: intent.TargetNodeID,
		Sources:      sources,
	})
	response := map[string]any{
		"sandbox_job_id": uuidToString(result.Job.ID),
		"mime":           result.MIME,
		"size_bytes":     result.Size,
	}
	if err != nil {
		return ProviderResult{}, ProviderRunError{
			Err:      fmt.Errorf("%w: %v", ErrProviderExecution, err),
			Response: response,
		}
	}
	return ProviderResult{
		RenderedPrompt:  intent.EffectivePrompt(),
		AssetStorageURL: result.Asset.StorageURL,
		AssetMIME:       result.MIME,
		AssetSizeBytes:  result.Size,
		AssetMetadata: map[string]any{
			"provider":       "internal_ffmpeg",
			"operation_type": intent.OperationType,
			"sandbox_job_id": uuidToString(result.Job.ID),
		},
		ProviderRequest: map[string]any{
			"provider":       "internal_ffmpeg",
			"operation_type": intent.OperationType,
			"source_count":   len(sources),
		},
		ProviderResponse: response,
	}, nil
}

func frameModeForOperation(operation string) (FrameExtractionMode, error) {
	switch operation {
	case "extract_first_frame":
		return FrameExtractionFirst, nil
	case "extract_last_frame":
		return FrameExtractionLast, nil
	default:
		return "", fmt.Errorf("%w: unsupported internal operation %s", ErrCapabilityMismatch, operation)
	}
}

func sandboxFrameMode(mode FrameExtractionMode) sandbox.FrameMode {
	if mode == FrameExtractionLast {
		return sandbox.FrameLast
	}
	return sandbox.FrameFirst
}

func videoInputRef(refs []InputRef) (InputRef, bool) {
	for _, ref := range refs {
		if ref.NodeType == "video" && strings.TrimSpace(ref.StorageURL) != "" {
			return ref, true
		}
	}
	return InputRef{}, false
}

func videoInputRefs(refs []InputRef) []sandbox.SandboxAssetInput {
	out := make([]sandbox.SandboxAssetInput, 0, len(refs))
	for _, ref := range refs {
		if ref.NodeType != "video" || strings.TrimSpace(ref.StorageURL) == "" {
			continue
		}
		out = append(out, sandbox.SandboxAssetInput{
			AssetID:    ref.AssetID,
			StorageURL: ref.StorageURL,
			Mime:       ref.Mime,
		})
	}
	return out
}

func boolParam(params map[string]any, key string) bool {
	value, ok := params[key]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}
