package production

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVolcengineAudioRuntimeGeneratesBase64Audio(t *testing.T) {
	audioBytes := []byte("fake-mp3-content")
	var gotRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/tts/create" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "test-key" {
			t.Fatalf("X-Api-Key = %q", r.Header.Get("X-Api-Key"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("authorization must be empty, got %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "audio-task-1",
			"audio_base64": base64.StdEncoding.EncodeToString(audioBytes),
			"mime_type":    "audio/mpeg",
		})
	}))
	defer server.Close()

	runtime := NewVolcengineAudioRuntime(VolcengineProviderConfig{
		APIKey:       "test-key",
		AudioBaseURL: server.URL + "/api/v3",
		AudioModel:   "seed-audio-1.0",
	}, server.Client())
	stream, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{
		OutputType:     "audio",
		OperationType:  "text_to_audio",
		PromptTemplate: "生成 12 秒清爽可信的中文旁白。",
		Model:          ModelSpec{Provider: "volcengine"},
		Params: map[string]any{
			"speaker":       "warm_female",
			"format":        "mp3",
			"sample_rate":   float64(48000),
			"speech_rate":   float64(1.05),
			"pitch_rate":    float64(0),
			"loudness_rate": float64(0),
			"watermark":     false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectProductionEvents(stream)
	last := events[len(events)-1]
	if last.Type != ProductionEventJobSucceeded {
		t.Fatalf("events = %#v", events)
	}
	if string(last.Output.AssetContent) != string(audioBytes) || last.Output.AssetMIME != "audio/mpeg" {
		t.Fatalf("output = %#v", last.Output)
	}
	if gotRequest["model"] != "seed-audio-1.0" || gotRequest["text_prompt"] != "生成 12 秒清爽可信的中文旁白。" {
		t.Fatalf("request = %#v", gotRequest)
	}
	if _, ok := gotRequest["references"]; ok {
		t.Fatalf("request must not send unverified speaker references: %#v", gotRequest)
	}
	audioConfig, ok := gotRequest["audio_config"].(map[string]any)
	if !ok {
		t.Fatalf("audio_config = %#v", gotRequest["audio_config"])
	}
	for _, key := range []string{"format", "sample_rate", "speech_rate", "pitch_rate", "loudness_rate", "watermark"} {
		if _, ok := audioConfig[key]; !ok && key != "watermark" {
			t.Fatalf("audio_config missing %s: %#v", key, audioConfig)
		}
	}
	watermark, ok := gotRequest["watermark"].(map[string]any)
	if !ok || watermark["aigc_watermark"] != false {
		t.Fatalf("watermark = %#v", gotRequest["watermark"])
	}
}

func TestVolcengineAudioRuntimeUsesTemporaryURLFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        "audio-task-2",
			"audio_url": "https://temporary.example/audio.wav",
			"format":    "wav",
		})
	}))
	defer server.Close()

	runtime := NewVolcengineAudioRuntime(VolcengineProviderConfig{APIKey: "test-key", AudioBaseURL: server.URL, AudioModel: "seed-audio-1.0"}, server.Client())
	stream, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{
		OutputType:     "audio",
		OperationType:  "text_to_audio",
		PromptTemplate: "生成 BGM。",
		Model:          ModelSpec{Provider: "volcengine"},
		Params:         map[string]any{"format": "wav"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectProductionEvents(stream)
	last := events[len(events)-1]
	if last.Output.AssetSourceURL != "https://temporary.example/audio.wav" || last.Output.AssetMIME != "audio/wav" {
		t.Fatalf("output = %#v", last.Output)
	}
}

func TestVolcengineAudioRuntimeCapturesDurationAndSubtitleAlignment(t *testing.T) {
	audioBytes := []byte("fake-mp3-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"audio":    base64.StdEncoding.EncodeToString(audioBytes),
			"duration": 3.4,
			"subtitles": []map[string]any{
				{"text": "顺滑万向轮", "start_sec": 1.2, "end_sec": 2.8},
			},
		})
	}))
	defer server.Close()

	runtime := NewVolcengineAudioRuntime(VolcengineProviderConfig{
		APIKey:       "test-key",
		AudioBaseURL: server.URL,
		AudioModel:   "seed-audio-1.0",
	}, server.Client())
	stream, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{
		OutputType:     "audio",
		OperationType:  "text_to_audio",
		PromptTemplate: "生成旁白。",
		Model:          ModelSpec{Provider: "volcengine"},
		Params:         map[string]any{"format": "mp3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectProductionEvents(stream)
	last := events[len(events)-1]
	if last.Output.AssetMetadata["duration_sec"] != 3.4 {
		t.Fatalf("metadata missing duration: %#v", last.Output.AssetMetadata)
	}
	alignment, ok := last.Output.AssetMetadata["alignment"].(map[string]any)
	if !ok || alignment["provider"] != "volcengine_tts_subtitle" {
		t.Fatalf("metadata missing subtitle alignment: %#v", last.Output.AssetMetadata)
	}
	if last.Output.ResponseSummary["duration"] != 3.4 {
		t.Fatalf("response summary missing duration: %#v", last.Output.ResponseSummary)
	}
}

func TestVolcengineAudioRuntimeUsesAudioAPIKeyWhenConfigured(t *testing.T) {
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"audio": base64.StdEncoding.EncodeToString([]byte("audio")),
		})
	}))
	defer server.Close()

	runtime := NewVolcengineAudioRuntime(VolcengineProviderConfig{
		APIKey:       "ark-key",
		AudioAPIKey:  "speech-key",
		AudioBaseURL: server.URL,
		AudioModel:   "seed-audio-1.0",
	}, server.Client())
	stream, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{
		OutputType:     "audio",
		OperationType:  "text_to_audio",
		PromptTemplate: "生成 BGM。",
		Model:          ModelSpec{Provider: "volcengine"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectProductionEvents(stream)
	if events[len(events)-1].Type != ProductionEventJobSucceeded {
		t.Fatalf("events = %#v", events)
	}
	if gotKey != "speech-key" {
		t.Fatalf("X-Api-Key = %q, want audio key", gotKey)
	}
}

func TestVolcengineAudioRuntimeRejectsMissingConfig(t *testing.T) {
	runtime := NewVolcengineAudioRuntime(VolcengineProviderConfig{AudioModel: "seed-audio-1.0"}, nil)
	if _, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{OutputType: "audio", OperationType: "text_to_audio"}); err == nil {
		t.Fatal("expected missing API key error")
	}

	runtime = NewVolcengineAudioRuntime(VolcengineProviderConfig{APIKey: "test-key"}, nil)
	if _, err := runtime.Start(context.Background(), ProductionJob{}, GenerationIntent{OutputType: "audio", OperationType: "text_to_audio"}); err == nil {
		t.Fatal("expected missing audio model error")
	}
}

func collectProductionEvents(stream <-chan ProductionEvent) []ProductionEvent {
	events := []ProductionEvent{}
	for event := range stream {
		events = append(events, event)
	}
	return events
}
