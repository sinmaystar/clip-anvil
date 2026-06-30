package referencevideo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type VolcengineAnalyzerConfig struct {
	APIKey  string
	BaseURL string
	Region  string
	Model   string
}

type VolcengineVideoAnalysisRequest struct {
	Model    string
	Prompt   string
	VideoURL string
	FPS      float64
}

type VolcengineVideoAnalysisClient interface {
	AnalyzeVideo(ctx context.Context, req VolcengineVideoAnalysisRequest) (string, map[string]any, error)
}

type VolcengineAnalyzer struct {
	cfg    VolcengineAnalyzerConfig
	client VolcengineVideoAnalysisClient
}

func NewVolcengineAnalyzer(cfg VolcengineAnalyzerConfig, client VolcengineVideoAnalysisClient) VolcengineAnalyzer {
	return VolcengineAnalyzer{cfg: cfg, client: client}
}

func (a VolcengineAnalyzer) AnalyzeReferenceVideo(ctx context.Context, input AnalyzerRequest) (AnalyzerResponse, error) {
	model := strings.TrimSpace(a.cfg.Model)
	if model == "" {
		return AnalyzerResponse{}, fmt.Errorf("reference video analyzer model is required")
	}
	if strings.TrimSpace(input.Media.StorageURL) == "" {
		return AnalyzerResponse{}, fmt.Errorf("reference video analyzer requires a provider-reachable video URL")
	}
	if a.client == nil {
		return AnalyzerResponse{}, fmt.Errorf("reference video analyzer client is not configured")
	}
	prompt := buildVideoAnalysisPrompt(input)
	raw, meta, err := a.client.AnalyzeVideo(ctx, VolcengineVideoAnalysisRequest{
		Model:    model,
		Prompt:   prompt,
		VideoURL: input.Media.StorageURL,
		FPS:      1,
	})
	if err != nil {
		return AnalyzerResponse{}, err
	}
	var result AnalysisResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return AnalyzerResponse{}, fmt.Errorf("reference video analyzer returned invalid JSON: %w", err)
	}
	return AnalyzerResponse{
		ModelProvider: "volcengine",
		ModelID:       model,
		RequestSummary: map[string]any{
			"provider":       "volcengine",
			"model_id":       model,
			"brief":          input.Brief,
			"focus":          input.Focus,
			"source_node_id": input.Media.SourceNodeID,
			"video_url":      input.Media.StorageURL,
			"response_meta":  meta,
		},
		Result: result,
	}, nil
}

func buildVideoAnalysisPrompt(input AnalyzerRequest) string {
	focus := strings.Join(input.Focus, ", ")
	if strings.TrimSpace(focus) == "" {
		focus = "hook, script_structure, shot_breakdown, camera_language, pacing, audio, text_style, adaptation_plan"
	}
	targetJSON := "{}"
	if len(input.AdaptationTarget) > 0 {
		if raw, err := json.Marshal(input.AdaptationTarget); err == nil {
			targetJSON = string(raw)
		}
	}
	parts := []string{
		strings.TrimSpace(input.FixedProtocol),
		"Producer brief:\n" + strings.TrimSpace(input.Brief),
		"Focus:\n" + focus,
		"Adaptation target JSON:\n" + targetJSON,
		"Media evidence:\nsource_node_id=" + input.Media.SourceNodeID + "\ntitle=" + input.Media.Title + "\nmime=" + input.Media.Mime,
		"Return only JSON.",
	}
	return strings.Join(parts, "\n\n")
}

type arkVideoAnalysisClient struct {
	apiKey     string
	baseURL    string
	region     string
	httpClient *http.Client
}

func NewArkVideoAnalysisClient(apiKey string, baseURL string, region string) VolcengineVideoAnalysisClient {
	return arkVideoAnalysisClient{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: strings.TrimSpace(baseURL),
		region:  strings.TrimSpace(region),
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (c arkVideoAnalysisClient) AnalyzeVideo(ctx context.Context, req VolcengineVideoAnalysisRequest) (string, map[string]any, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return "", nil, fmt.Errorf("reference video analyzer api key is required")
	}
	if strings.TrimSpace(req.Model) == "" {
		return "", nil, fmt.Errorf("reference video analyzer model is required")
	}
	if strings.TrimSpace(req.VideoURL) == "" {
		return "", nil, fmt.Errorf("reference video analyzer video url is required")
	}
	payload := map[string]any{
		"model": req.Model,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": req.Prompt},
				{"type": "video_url", "video_url": map[string]any{"url": req.VideoURL, "fps": req.FPS}},
			},
		}},
		"response_format": map[string]any{"type": "json_object"},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(raw))
	if err != nil {
		return "", nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := c.client().Do(httpReq)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("reference video analyzer http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	content, meta, err := parseArkChatCompletion(body)
	if err != nil {
		return "", nil, err
	}
	return content, meta, nil
}

func (c arkVideoAnalysisClient) endpoint() string {
	baseURL := strings.TrimRight(strings.TrimSpace(c.baseURL), "/")
	if baseURL == "" {
		baseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func (c arkVideoAnalysisClient) client() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}
	return http.DefaultClient
}

func parseArkChatCompletion(raw []byte) (string, map[string]any, error) {
	var decoded struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage any `json:"usage"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", nil, fmt.Errorf("decode reference video analyzer response: %w", err)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", nil, fmt.Errorf("reference video analyzer returned empty content")
	}
	meta := map[string]any{}
	if decoded.ID != "" {
		meta["id"] = decoded.ID
	}
	if decoded.Usage != nil {
		meta["usage"] = decoded.Usage
	}
	return decoded.Choices[0].Message.Content, meta, nil
}
