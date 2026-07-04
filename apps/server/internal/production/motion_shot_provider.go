package production

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/sinmaystar/clip-anvil/internal/motionshot"
	"github.com/sinmaystar/clip-anvil/internal/sandbox"
)

type SandboxMotionShotRenderer interface {
	RenderMotionShot(ctx context.Context, input sandbox.RenderMotionShotInput) (sandbox.SandboxJobResult, error)
}

type MotionShotProvider struct {
	renderer SandboxMotionShotRenderer
}

func NewMotionShotProvider(operator any) MotionShotProvider {
	provider := MotionShotProvider{}
	if renderer, ok := operator.(SandboxMotionShotRenderer); ok {
		provider.renderer = renderer
	}
	return provider
}

func (p MotionShotProvider) Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error) {
	if p.renderer == nil {
		return ProviderResult{}, fmt.Errorf("%w: sandbox motion shot renderer is not configured", ErrProviderConfig)
	}
	if intent.OutputType != "video" {
		return ProviderResult{}, fmt.Errorf("%w: motion shot output type must be video", ErrCapabilityMismatch)
	}
	if intent.OperationType != "image_to_motion_video" {
		return ProviderResult{}, fmt.Errorf("%w: unsupported motion shot operation %s", ErrCapabilityMismatch, intent.OperationType)
	}
	assets := motionAssets(intent.InputRefs)
	if len(assets) == 0 {
		return ProviderResult{}, fmt.Errorf("%w: image_to_motion_video requires an image input", ErrCapabilityMismatch)
	}
	plan, err := motionshot.Normalize(motionshot.RenderInput{
		DurationSec: motionIntParam(intent.Params, "duration_sec", 5),
		Ratio:       stringParam(intent.Params, "ratio", "9:16"),
		Resolution:  stringParam(intent.Params, "resolution", "1080p"),
		FPS:         motionIntParam(intent.Params, "fps", 30),
		Assets:      motionRenderAssets(assets),
		Params:      intent.Params,
	})
	if err != nil {
		return ProviderResult{}, fmt.Errorf("%w: %v", ErrCapabilityMismatch, err)
	}
	result, err := p.renderer.RenderMotionShot(ctx, sandbox.RenderMotionShotInput{
		WorkspaceID:  intent.WorkspaceID,
		TargetNodeID: intent.TargetNodeID,
		Plan:         plan,
		Meta: sandbox.MotionShotMeta{
			DurationSec: plan.DurationSec,
			Width:       plan.Width,
			Height:      plan.Height,
			FPS:         plan.FPS,
		},
		Assets: assets,
	})
	response := map[string]any{
		"sandbox_job_id":  uuidToString(result.Job.ID),
		"mime":            result.MIME,
		"size_bytes":      result.Size,
		"renderer_engine": "remotion",
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
			"provider":         "internal_motion_video",
			"operation_type":   intent.OperationType,
			"rendering_family": "motion_shot_video",
			"renderer_engine":  "remotion",
			"motion_style":     plan.MotionStyle,
			"duration_sec":     plan.DurationSec,
			"width":            plan.Width,
			"height":           plan.Height,
			"fps":              plan.FPS,
			"sandbox_job_id":   uuidToString(result.Job.ID),
		},
		ProviderRequest: map[string]any{
			"provider":       "internal_motion_video",
			"model_id":       "remotion-motion-shot-v1",
			"operation_type": intent.OperationType,
			"asset_count":    len(assets),
			"motion_style":   plan.MotionStyle,
		},
		ProviderResponse: response,
	}, nil
}

func motionAssets(refs []InputRef) []sandbox.RenderMotionAssetInput {
	out := make([]sandbox.RenderMotionAssetInput, 0, len(refs))
	for _, ref := range refs {
		if ref.NodeType != "image" || strings.TrimSpace(ref.StorageURL) == "" {
			continue
		}
		fileName := sandbox.SafeAssetName(firstNonEmptyMotionString(ref.AssetID, ref.CurrentVersionID, "product")) + extensionForMIME(ref.Mime)
		workspacePath := path.Join("assets", fileName)
		out = append(out, sandbox.RenderMotionAssetInput{
			AssetID:       ref.AssetID,
			StorageURL:    ref.StorageURL,
			Mime:          ref.Mime,
			FileName:      fileName,
			WorkspacePath: workspacePath,
		})
	}
	return out
}

func motionRenderAssets(assets []sandbox.RenderMotionAssetInput) []motionshot.Asset {
	out := make([]motionshot.Asset, 0, len(assets))
	for _, asset := range assets {
		out = append(out, motionshot.Asset{
			AssetID:       asset.AssetID,
			WorkspacePath: asset.WorkspacePath,
			Mime:          asset.Mime,
		})
	}
	return out
}

func motionIntParam(params map[string]any, key string, fallback int) int {
	if params == nil {
		return fallback
	}
	switch value := params[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return fallback
	}
}

func firstNonEmptyMotionString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
