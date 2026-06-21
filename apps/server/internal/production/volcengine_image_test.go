package production

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeArkImageGenerator struct {
	messages []*schema.Message
	msg      *schema.Message
	err      error
}

func (f *fakeArkImageGenerator) Generate(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.Message, error) {
	f.messages = messages
	if f.err != nil {
		return nil, f.err
	}
	return f.msg, nil
}

func TestImageRuntimeReturnsURLForSandboxPersistence(t *testing.T) {
	url := "https://provider.invalid/image.png"
	runtime := newVolcengineImageRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", ImageModel: "doubao-seedream-5-0-260128"},
		&fakeArkImageGenerator{msg: imageMessageWithURL(url)},
		nil,
	)
	output := runImageRuntime(t, runtime)
	if output.AssetSourceURL != url || len(output.AssetContent) != 0 {
		t.Fatalf("asset url/content = %q/%d", output.AssetSourceURL, len(output.AssetContent))
	}
	if output.AssetMIME != "image/png" {
		t.Fatalf("asset mime = %q", output.AssetMIME)
	}
	if output.AssetMetadata["source"] != "url" {
		t.Fatalf("metadata = %#v", output.AssetMetadata)
	}
	if output.RequestSummary["prompt"] != "A simple studio desk with one lamp." {
		t.Fatalf("request summary prompt = %#v", output.RequestSummary["prompt"])
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestImageRuntimeDecodesBase64AndProducesAssetContent(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(onePixelPNG)
	runtime := newVolcengineImageRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", ImageModel: "doubao-seedream-5-0-260128"},
		&fakeArkImageGenerator{msg: imageMessageWithBase64(encoded, "image/png")},
		nil,
	)
	output := runImageRuntime(t, runtime)
	if output.AssetMIME != "image/png" || len(output.AssetContent) != len(onePixelPNG) {
		t.Fatalf("asset mime/len = %q/%d", output.AssetMIME, len(output.AssetContent))
	}
	if output.AssetMetadata["source"] != "base64" {
		t.Fatalf("metadata = %#v", output.AssetMetadata)
	}
	if output.AssetMetadata["width"] != 1 || output.AssetMetadata["height"] != 1 {
		t.Fatalf("metadata dimensions = %#v", output.AssetMetadata)
	}
}

func TestImageRuntimeRejectsUnexpectedMIME(t *testing.T) {
	runtime := newVolcengineImageRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", ImageModel: "doubao-seedream-5-0-260128"},
		&fakeArkImageGenerator{msg: imageMessageWithBase64(base64.StdEncoding.EncodeToString([]byte("plain text")), "text/plain")},
		nil,
	)
	stream, err := runtime.Start(context.Background(), ProductionJob{}, imageIntent())
	if err != nil {
		t.Fatal(err)
	}
	event := <-stream
	if event.Type != ProductionEventProviderProgress {
		t.Fatalf("first event = %#v", event)
	}
	event = <-stream
	if event.Type != ProductionEventJobFailed || !errors.Is(event.Err, ErrProviderExecution) {
		t.Fatalf("event = %#v", event)
	}
}

func TestImageRuntimeSendsReferenceImagesToArk(t *testing.T) {
	model := &fakeArkImageGenerator{msg: imageMessageWithURL("https://provider.invalid/image.png")}
	runtime := newVolcengineImageRuntimeForTest(
		VolcengineProviderConfig{APIKey: "test-key", ImageModel: "doubao-seedream-5-0-260128"},
		model,
		nil,
	)
	intent := imageIntent()
	intent.OperationType = "image_to_image"
	intent.InputRefs = []InputRef{{
		NodeType:   "image",
		StorageURL: "https://assets.example/reference.png",
		Mime:       "image/png",
	}}

	output := runImageRuntimeWithIntent(t, runtime, intent)
	if len(model.messages) != 1 {
		t.Fatalf("messages = %#v", model.messages)
	}
	parts := model.messages[0].MultiContent
	if len(parts) != 2 {
		t.Fatalf("multi content = %#v", parts)
	}
	if parts[0].Type != schema.ChatMessagePartTypeText || parts[0].Text != "A simple studio desk with one lamp." {
		t.Fatalf("text part = %#v", parts[0])
	}
	if parts[1].Type != schema.ChatMessagePartTypeImageURL || parts[1].ImageURL.URL != "https://assets.example/reference.png" {
		t.Fatalf("image part = %#v", parts[1])
	}
	inputImages, ok := output.RequestSummary["input_images"].([]map[string]any)
	if !ok || len(inputImages) != 1 || inputImages[0]["url"] != "https://assets.example/reference.png" {
		t.Fatalf("request summary input images = %#v", output.RequestSummary["input_images"])
	}
}

func runImageRuntime(t *testing.T, runtime VolcengineImageRuntime) ProductionOutput {
	return runImageRuntimeWithIntent(t, runtime, imageIntent())
}

func runImageRuntimeWithIntent(t *testing.T, runtime VolcengineImageRuntime, intent GenerationIntent) ProductionOutput {
	t.Helper()
	stream, err := runtime.Start(context.Background(), ProductionJob{}, intent)
	if err != nil {
		t.Fatal(err)
	}
	var output ProductionOutput
	for event := range stream {
		if event.Type == ProductionEventJobFailed {
			t.Fatalf("unexpected failure: %v", event.Err)
		}
		if event.Type == ProductionEventJobSucceeded {
			output = event.Output
		}
	}
	if len(output.AssetContent) == 0 {
		if output.AssetSourceURL == "" {
			t.Fatal("expected asset content or source url")
		}
	}
	return output
}

func imageIntent() GenerationIntent {
	return GenerationIntent{
		OperationType:  "text_to_image",
		PromptTemplate: "A simple studio desk with one lamp.",
		Model:          ModelSpec{Provider: "volcengine", ModelID: "doubao-seedream-5-0-260128"},
		Params:         map[string]any{"size": "2048x2048", "response_format": "url"},
	}
}

func imageMessageWithURL(url string) *schema.Message {
	return &schema.Message{
		Role: schema.Assistant,
		AssistantGenMultiContent: []schema.MessageOutputPart{{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageOutputImage{MessagePartCommon: schema.MessagePartCommon{
				URL:      &url,
				MIMEType: "image/png",
			}},
		}},
	}
}

func imageMessageWithBase64(value string, mime string) *schema.Message {
	return &schema.Message{
		Role: schema.Assistant,
		AssistantGenMultiContent: []schema.MessageOutputPart{{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageOutputImage{MessagePartCommon: schema.MessagePartCommon{
				Base64Data: &value,
				MIMEType:   mime,
			}},
		}},
	}
}
