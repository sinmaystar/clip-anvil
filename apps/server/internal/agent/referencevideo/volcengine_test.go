package referencevideo

import (
	"context"
	"strings"
	"testing"
)

func TestVolcengineAnalyzerBuildsFixedProtocolBriefAndVideoEvidence(t *testing.T) {
	client := &fakeMultimodalClient{
		response: `{"summary":"前三秒用痛点 hook。","reference_intent":{"preserve":["痛点 hook"],"must_be_original":["商品外观"]},"warnings":["不要复制原字幕。"]}`,
	}
	analyzer := NewVolcengineAnalyzer(VolcengineAnalyzerConfig{
		APIKey: "test-key",
		Model:  "doubao-seed-2-1-pro-260628",
	}, client)
	out, err := analyzer.AnalyzeReferenceVideo(context.Background(), AnalyzerRequest{
		FixedProtocol: FixedAnalysisProtocol,
		Brief:         "借鉴脚本结构",
		Focus:         []string{"hook"},
		Media: MediaEvidence{
			SourceNodeID: "node-1",
			Title:        "ref.mp4",
			Mime:         "video/mp4",
			StorageURL:   "https://assets.example/ref.mp4",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelProvider != "volcengine" || out.ModelID != "doubao-seed-2-1-pro-260628" {
		t.Fatalf("model = %s/%s", out.ModelProvider, out.ModelID)
	}
	if out.Result.Summary != "前三秒用痛点 hook。" {
		t.Fatalf("result = %#v", out.Result)
	}
	prompt := client.lastPrompt
	if !strings.Contains(prompt, "must_be_original") || !strings.Contains(prompt, "借鉴脚本结构") {
		t.Fatalf("prompt = %s", prompt)
	}
	if client.lastVideoURL != "https://assets.example/ref.mp4" {
		t.Fatalf("video url = %q", client.lastVideoURL)
	}
}

type fakeMultimodalClient struct {
	response     string
	lastPrompt   string
	lastVideoURL string
}

func (f *fakeMultimodalClient) AnalyzeVideo(_ context.Context, req VolcengineVideoAnalysisRequest) (string, map[string]any, error) {
	f.lastPrompt = req.Prompt
	f.lastVideoURL = req.VideoURL
	return f.response, map[string]any{"request_id": "req-1"}, nil
}
