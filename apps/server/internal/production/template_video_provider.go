package production

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/sinmaystar/clip-anvil/internal/sandbox"
	"github.com/sinmaystar/clip-anvil/internal/templatevideo"
)

type SandboxTemplateVideoRenderer interface {
	RenderTemplateVideo(ctx context.Context, input sandbox.RenderTemplateVideoInput) (sandbox.SandboxJobResult, error)
}

type TemplateVideoProvider struct {
	renderer SandboxTemplateVideoRenderer
}

func NewTemplateVideoProvider(operator any) TemplateVideoProvider {
	provider := TemplateVideoProvider{}
	if renderer, ok := operator.(SandboxTemplateVideoRenderer); ok {
		provider.renderer = renderer
	}
	return provider
}

func (p TemplateVideoProvider) Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error) {
	if p.renderer == nil {
		return ProviderResult{}, fmt.Errorf("%w: sandbox template video renderer is not configured", ErrProviderConfig)
	}
	if intent.OutputType != "video" {
		return ProviderResult{}, fmt.Errorf("%w: template video output type must be video", ErrCapabilityMismatch)
	}
	if intent.OperationType != "template_to_video" && intent.OperationType != "image_to_template_video" {
		return ProviderResult{}, fmt.Errorf("%w: unsupported template video operation %s", ErrCapabilityMismatch, intent.OperationType)
	}
	templateKey := templateStringParam(intent.Params, "template_key", "static_fallback_ken_burns_v1")
	assets := templateAssets(intent.InputRefs)
	if intent.OperationType == "image_to_template_video" && len(assets) == 0 {
		return ProviderResult{}, fmt.Errorf("%w: image_to_template_video requires a product image input", ErrCapabilityMismatch)
	}
	html, meta, err := templatevideo.Render(templatevideo.RenderInput{
		TemplateKey: templateKey,
		DurationSec: intParam(intent.Params, "duration_sec", 5),
		Ratio:       templateStringParam(intent.Params, "ratio", "9:16"),
		Resolution:  templateStringParam(intent.Params, "resolution", "1080p"),
		FPS:         intParam(intent.Params, "fps", 24),
		Variables:   mapParam(intent.Params, "variables"),
		Assets:      templateRenderAssets(assets),
	})
	if err != nil {
		return ProviderResult{}, fmt.Errorf("%w: %v", ErrCapabilityMismatch, err)
	}
	result, err := p.renderer.RenderTemplateVideo(ctx, sandbox.RenderTemplateVideoInput{
		WorkspaceID:  intent.WorkspaceID,
		TargetNodeID: intent.TargetNodeID,
		TemplateKey:  templateKey,
		HTML:         html,
		Meta: sandbox.TemplateVideoMeta{
			DurationSec: meta.DurationSec,
			Width:       meta.Width,
			Height:      meta.Height,
			FPS:         meta.FPS,
		},
		Variables: mapParam(intent.Params, "variables"),
		Assets:    assets,
	})
	response := map[string]any{
		"sandbox_job_id":  uuidToString(result.Job.ID),
		"mime":            result.MIME,
		"size_bytes":      result.Size,
		"template_key":    templateKey,
		"template_engine": "hyperframes",
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
			"provider":         "internal_template_video",
			"operation_type":   intent.OperationType,
			"rendering_family": "template_video",
			"template_engine":  "hyperframes",
			"template_key":     templateKey,
			"duration_sec":     meta.DurationSec,
			"width":            meta.Width,
			"height":           meta.Height,
			"fps":              meta.FPS,
			"sandbox_job_id":   uuidToString(result.Job.ID),
		},
		ProviderRequest: map[string]any{
			"provider":       "internal_template_video",
			"model_id":       "hyperframes-html",
			"operation_type": intent.OperationType,
			"template_key":   templateKey,
			"asset_count":    len(assets),
		},
		ProviderResponse: response,
	}, nil
}

func templateAssets(refs []InputRef) []sandbox.RenderTemplateAssetInput {
	out := make([]sandbox.RenderTemplateAssetInput, 0, len(refs))
	for _, ref := range refs {
		if ref.NodeType != "image" || strings.TrimSpace(ref.StorageURL) == "" {
			continue
		}
		fileName := sandbox.SafeAssetName(templateFirstNonEmpty(ref.AssetID, ref.CurrentVersionID, "product")) + extensionForMIME(ref.Mime)
		workspacePath := path.Join("assets", fileName)
		out = append(out, sandbox.RenderTemplateAssetInput{
			AssetID:       ref.AssetID,
			StorageURL:    ref.StorageURL,
			Mime:          ref.Mime,
			FileName:      fileName,
			WorkspacePath: workspacePath,
		})
	}
	return out
}

func templateRenderAssets(assets []sandbox.RenderTemplateAssetInput) []templatevideo.Asset {
	out := make([]templatevideo.Asset, 0, len(assets))
	for _, asset := range assets {
		out = append(out, templatevideo.Asset{
			ClientKey:     asset.AssetID,
			WorkspacePath: asset.WorkspacePath,
			Mime:          asset.Mime,
		})
	}
	return out
}

func templateStringParam(params map[string]any, key string, fallback string) string {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return strings.TrimSpace(text)
}

func intParam(params map[string]any, key string, fallback int) int {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func mapParam(params map[string]any, key string) map[string]any {
	value, ok := params[key]
	if !ok {
		return map[string]any{}
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return typed
}

func templateFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
