package production

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type arkImageGenerator interface {
	Generate(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.Message, error)
}

type arkImageModelFactory func(ctx context.Context, config *ark.ImageGenerationConfig) (arkImageGenerator, error)

type VolcengineImageRuntime struct {
	cfg           VolcengineProviderConfig
	factory       arkImageModelFactory
	httpClient    *http.Client
	inputResolver ProviderInputResolver
}

func NewVolcengineImageRuntime(cfg VolcengineProviderConfig, httpClient *http.Client) VolcengineImageRuntime {
	return VolcengineImageRuntime{
		cfg:        cfg,
		httpClient: httpClient,
		factory: func(ctx context.Context, config *ark.ImageGenerationConfig) (arkImageGenerator, error) {
			return ark.NewImageGenerationModel(ctx, config)
		},
	}
}

func newVolcengineImageRuntimeForTest(cfg VolcengineProviderConfig, model arkImageGenerator, httpClient *http.Client) VolcengineImageRuntime {
	return VolcengineImageRuntime{
		cfg:        cfg,
		httpClient: httpClient,
		factory: func(context.Context, *ark.ImageGenerationConfig) (arkImageGenerator, error) {
			return model, nil
		},
	}
}

func (r VolcengineImageRuntime) Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error) {
	if strings.TrimSpace(r.cfg.APIKey) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for provider volcengine", ErrProviderConfig)
	}
	modelID := strings.TrimSpace(intent.Model.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(r.cfg.ImageModel)
	}
	if modelID == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_IMAGE_MODEL is required", ErrProviderConfig)
	}
	model, err := r.factory(ctx, &ark.ImageGenerationConfig{
		APIKey:           r.cfg.APIKey,
		BaseURL:          strings.TrimSpace(r.cfg.BaseURL),
		Region:           strings.TrimSpace(r.cfg.Region),
		Model:            modelID,
		Size:             stringParam(intent.Params, "size", "2048x2048"),
		ResponseFormat:   ark.ImageResponseFormat(stringParam(intent.Params, "response_format", string(ark.ImageResponseFormatURL))),
		DisableWatermark: boolModelParam(intent.Params, "disable_watermark"),
		Timeout:          durationPtr(10 * time.Minute),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create ark image model: %v", ErrProviderUnavailable, err)
	}
	events := make(chan ProductionEvent, 8)
	go r.generate(ctx, model, job, intent, events)
	return events, nil
}

func (r VolcengineImageRuntime) generate(ctx context.Context, model arkImageGenerator, job ProductionJob, intent GenerationIntent, events chan<- ProductionEvent) {
	defer close(events)
	rendered := strings.TrimSpace(intent.EffectivePrompt())
	if rendered == "" {
		rendered = "empty prompt"
	}
	if r.inputResolver != nil {
		resolved, err := r.inputResolver.ResolveInputRefs(ctx, job, intent)
		if err != nil {
			events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: err}
			return
		}
		intent = resolved
	}
	if err := validateProviderReachableImageInputs(intent); err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: err}
		return
	}
	events <- ProductionEvent{Type: ProductionEventProviderProgress, Progress: 20, Payload: map[string]any{"stage": "image_generation_started"}}
	msg, err := model.Generate(ctx, imageGenerationMessages(rendered, intent))
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: generate ark image: %v", ErrProviderExecution, err)}
		return
	}
	image, err := r.imageOutput(msg)
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: err}
		return
	}
	events <- ProductionEvent{
		JobID:        job.ID,
		WorkspaceID:  job.WorkspaceID,
		TargetNodeID: job.TargetNodeID,
		Type:         ProductionEventJobSucceeded,
		Progress:     100,
		Output: ProductionOutput{
			RenderedPrompt: rendered,
			AssetContent:   image.content,
			AssetSourceURL: image.sourceURL,
			AssetMIME:      image.mime,
			AssetMetadata:  imageAssetMetadata(image.content, map[string]any{"provider": "volcengine", "source": image.source}),
			RequestSummary: map[string]any{
				"provider":       "volcengine",
				"model_id":       intent.Model.ModelID,
				"operation_type": intent.OperationType,
				"prompt":         rendered,
				"params":         intent.Params,
				"input_images":   imageInputSummaries(intent),
			},
			ResponseSummary: map[string]any{"provider": "volcengine", "source": image.source},
		},
	}
}

func imageGenerationMessages(rendered string, intent GenerationIntent) []*schema.Message {
	imageRefs := imageInputRefs(intent)
	if len(imageRefs) == 0 {
		return []*schema.Message{schema.UserMessage(rendered)}
	}
	parts := []schema.MessageInputPart{{
		Type: schema.ChatMessagePartTypeText,
		Text: rendered,
	}}
	for _, ref := range imageRefs {
		url := strings.TrimSpace(ref.StorageURL)
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{
					URL:      &url,
					MIMEType: strings.TrimSpace(ref.Mime),
				},
				Detail: schema.ImageURLDetailAuto,
			},
		})
	}
	return []*schema.Message{{
		Role:                  schema.User,
		Content:               rendered,
		UserInputMultiContent: parts,
	}}
}

func validateProviderReachableImageInputs(intent GenerationIntent) error {
	for _, ref := range imageInputRefs(intent) {
		if strings.HasPrefix(ref.StorageURL, "http://") || strings.HasPrefix(ref.StorageURL, "https://") {
			continue
		}
		return fmt.Errorf("%w: image input %s must be staged to a provider-reachable URL before image generation", ErrProviderConfig, uuidToString(ref.NodeID))
	}
	return nil
}

func imageInputSummaries(intent GenerationIntent) []map[string]any {
	refs := imageInputRefs(intent)
	summaries := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		summaries = append(summaries, map[string]any{
			"node_id":  uuidToString(ref.NodeID),
			"asset_id": ref.AssetID,
			"mime":     ref.Mime,
			"url":      strings.TrimSpace(ref.StorageURL),
		})
	}
	return summaries
}

func imageInputRefs(intent GenerationIntent) []InputRef {
	refs := []InputRef{}
	for _, ref := range intent.InputRefs {
		if ref.NodeType != "image" || strings.TrimSpace(ref.StorageURL) == "" {
			continue
		}
		refs = append(refs, ref)
	}
	return refs
}

type providerImageOutput struct {
	content   []byte
	sourceURL string
	mime      string
	source    string
}

func (r VolcengineImageRuntime) imageOutput(msg *schema.Message) (providerImageOutput, error) {
	for _, part := range msg.AssistantGenMultiContent {
		if part.Image == nil {
			continue
		}
		if part.Image.Base64Data != nil && strings.TrimSpace(*part.Image.Base64Data) != "" {
			content, err := decodeImageBase64(*part.Image.Base64Data)
			if err != nil {
				return providerImageOutput{}, err
			}
			mime := strings.TrimSpace(part.Image.MIMEType)
			if mime == "" {
				mime = http.DetectContentType(content)
			}
			if !allowedImageMIMEs()[mime] {
				return providerImageOutput{}, fmt.Errorf("%w: unsupported provider output mime %s", ErrProviderExecution, mime)
			}
			return providerImageOutput{content: content, mime: mime, source: "base64"}, nil
		}
		if part.Image.URL != nil && strings.TrimSpace(*part.Image.URL) != "" {
			mime := strings.TrimSpace(part.Image.MIMEType)
			if mime == "" {
				mime = "image/png"
			}
			if !allowedImageMIMEs()[mime] {
				return providerImageOutput{}, fmt.Errorf("%w: unsupported provider output mime %s", ErrProviderExecution, mime)
			}
			return providerImageOutput{sourceURL: strings.TrimSpace(*part.Image.URL), mime: mime, source: "url"}, nil
		}
	}
	return providerImageOutput{}, fmt.Errorf("%w: image generation returned no image content", ErrProviderExecution)
}

func imageAssetMetadata(content []byte, metadata map[string]any) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if len(content) == 0 {
		return metadata
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return metadata
	}
	if cfg.Width > 0 {
		metadata["width"] = cfg.Width
	}
	if cfg.Height > 0 {
		metadata["height"] = cfg.Height
	}
	return metadata
}

func allowedImageMIMEs() map[string]bool {
	return map[string]bool{
		"image/png":  true,
		"image/jpeg": true,
		"image/webp": true,
	}
}

func decodeImageBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if before, after, ok := strings.Cut(value, ","); ok && strings.Contains(before, ";base64") {
		value = after
	}
	out, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%w: decode provider image base64", ErrProviderExecution)
	}
	return out, nil
}

func stringParam(params map[string]any, key string, fallback string) string {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return text
}

func boolModelParam(params map[string]any, key string) bool {
	value, ok := params[key]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}
