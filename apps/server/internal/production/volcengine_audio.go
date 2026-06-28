package production

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type VolcengineAudioRuntime struct {
	cfg        VolcengineProviderConfig
	httpClient *http.Client
}

func NewVolcengineAudioRuntime(cfg VolcengineProviderConfig, httpClient *http.Client) VolcengineAudioRuntime {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return VolcengineAudioRuntime{cfg: cfg, httpClient: httpClient}
}

func (r VolcengineAudioRuntime) Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error) {
	if strings.TrimSpace(r.cfg.APIKey) == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for provider volcengine", ErrProviderConfig)
	}
	modelID := strings.TrimSpace(intent.Model.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(r.cfg.AudioModel)
	}
	if modelID == "" {
		return nil, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_MODEL is required", ErrProviderConfig)
	}
	if strings.TrimSpace(intent.OperationType) != "text_to_audio" {
		return nil, fmt.Errorf("%w: volcengine audio only supports text_to_audio", ErrCapabilityMismatch)
	}
	events := make(chan ProductionEvent, 8)
	go r.generate(ctx, modelID, job, intent, events)
	return events, nil
}

func (r VolcengineAudioRuntime) generate(ctx context.Context, modelID string, job ProductionJob, intent GenerationIntent, events chan<- ProductionEvent) {
	defer close(events)
	rendered := strings.TrimSpace(intent.EffectivePrompt())
	if rendered == "" {
		rendered = "empty prompt"
	}
	requestPayload := audioGenerationRequest(modelID, rendered, intent.Params)
	events <- ProductionEvent{Type: ProductionEventProviderProgress, Progress: 20, Payload: map[string]any{"stage": "audio_generation_started", "provider": "volcengine"}}
	rawRequest, err := json.Marshal(requestPayload)
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: err}
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.audioEndpoint(), bytes.NewReader(rawRequest))
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: err}
		return
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(r.cfg.APIKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: generate volcengine audio: %v", ErrProviderExecution, err)}
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: err}
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: fmt.Errorf("%w: volcengine audio status %d: %s", ErrProviderExecution, resp.StatusCode, strings.TrimSpace(string(body)))}
		return
	}
	output, responseSummary, err := audioOutputFromResponse(body, stringParam(intent.Params, "format", "mp3"))
	if err != nil {
		events <- ProductionEvent{Type: ProductionEventJobFailed, Progress: 100, Err: err}
		return
	}
	output.RenderedPrompt = rendered
	output.RequestSummary = map[string]any{
		"provider":       "volcengine",
		"model_id":       modelID,
		"operation_type": intent.OperationType,
		"prompt":         rendered,
		"params":         intent.Params,
	}
	output.ResponseSummary = responseSummary
	events <- ProductionEvent{
		JobID:        job.ID,
		WorkspaceID:  job.WorkspaceID,
		TargetNodeID: job.TargetNodeID,
		Type:         ProductionEventJobSucceeded,
		Progress:     100,
		Output:       output,
	}
}

func (r VolcengineAudioRuntime) audioEndpoint() string {
	baseURL := strings.TrimSpace(r.cfg.BaseURL)
	if baseURL == "" {
		baseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}
	return strings.TrimRight(baseURL, "/") + "/audio/generations"
}

func audioGenerationRequest(modelID string, rendered string, params map[string]any) map[string]any {
	out := map[string]any{
		"model":       modelID,
		"text_prompt": rendered,
	}
	for _, key := range []string{"speaker", "format", "sample_rate", "speech_rate", "pitch_rate", "loudness_rate", "watermark"} {
		if params == nil {
			continue
		}
		if value, ok := params[key]; ok {
			out[key] = value
		}
	}
	if _, ok := out["format"]; !ok {
		out["format"] = "mp3"
	}
	return out
}

func audioOutputFromResponse(raw []byte, requestedFormat string) (ProductionOutput, map[string]any, error) {
	var response map[string]any
	if err := json.Unmarshal(raw, &response); err != nil {
		return ProductionOutput{}, nil, fmt.Errorf("%w: decode volcengine audio response: %v", ErrProviderExecution, err)
	}
	audioBase64 := firstResponseString(response, "audio_base64", "audio", "data")
	audioURL := firstResponseString(response, "audio_url", "url", "file_url")
	format := firstNonEmptyAudio(firstResponseString(response, "format"), requestedFormat)
	mime := firstNonEmptyAudio(firstResponseString(response, "mime_type", "mime"), audioMIMEForFormat(format))
	output := ProductionOutput{
		AssetMIME:     mime,
		AssetMetadata: map[string]any{"provider": "volcengine", "source": "response"},
	}
	if audioBase64 != "" {
		content, err := base64.StdEncoding.DecodeString(audioBase64)
		if err != nil {
			return ProductionOutput{}, response, fmt.Errorf("%w: decode audio_base64: %v", ErrProviderExecution, err)
		}
		output.AssetContent = content
		return output, response, nil
	}
	if audioURL != "" {
		output.AssetSourceURL = audioURL
		return output, response, nil
	}
	return ProductionOutput{}, response, fmt.Errorf("%w: volcengine audio response returned no audio content", ErrProviderExecution)
}

func firstResponseString(response map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := response[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case map[string]any:
			if value := firstResponseString(typed, "audio_base64", "audio_url", "url", "file_url"); value != "" {
				return value
			}
		case []any:
			for _, item := range typed {
				nested, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if value := firstResponseString(nested, "audio_base64", "audio_url", "url", "file_url"); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func audioMIMEForFormat(format string) string {
	switch strings.TrimSpace(strings.ToLower(format)) {
	case "wav":
		return "audio/wav"
	case "ogg_opus", "ogg":
		return "audio/ogg"
	case "pcm":
		return "audio/L16"
	default:
		return "audio/mpeg"
	}
}

func firstNonEmptyAudio(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
