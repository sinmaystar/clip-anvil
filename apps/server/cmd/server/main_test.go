package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentcontextcompact "github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	agentcraftsman "github.com/sinmaystar/clip-anvil/internal/agent/craftsman"
	agentproducer "github.com/sinmaystar/clip-anvil/internal/agent/producer"
	"github.com/sinmaystar/clip-anvil/internal/config"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestSandboxHealthURLUsesServerHealthEndpoint(t *testing.T) {
	got, err := sandboxHealthURL("http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("sandboxHealthURL error = %v", err)
	}
	if got != "http://localhost:8080/health" {
		t.Fatalf("sandboxHealthURL = %q, want http://localhost:8080/health", got)
	}
}

func TestCheckSandboxServerHealthRequiresOK(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "http://localhost:8080/health" {
			t.Fatalf("request URL = %q", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(nil),
		}, nil
	})}

	if err := checkSandboxServerHealth(context.Background(), client, "http://localhost:8080/v1"); err == nil {
		t.Fatal("expected non-OK sandbox health response to fail")
	}
}

func TestProducerResponderForConfigUsesDeterministicOutsideRealMode(t *testing.T) {
	responder := producerResponderForConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode: "mock",
		},
	}, nil)
	if _, ok := responder.(agentproducer.DeterministicResponder); !ok {
		t.Fatalf("expected deterministic responder, got %T", responder)
	}
}

func TestProducerResponderForConfigUsesDeterministicWhenRealModeHasNoKey(t *testing.T) {
	responder := producerResponderForConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode: "real",
			Volcengine: config.VolcengineConfig{
				APIKey: "   ",
			},
		},
	}, nil)
	if _, ok := responder.(agentproducer.DeterministicResponder); !ok {
		t.Fatalf("expected deterministic responder, got %T", responder)
	}
}

func TestProducerResponderForConfigUsesM1E2EFixtureWhenEnabled(t *testing.T) {
	t.Setenv("CLIPANVIL_E2E_PRODUCER_FIXTURE", "m1_creative_state")
	responder := producerResponderForConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode: "mock",
		},
	}, nil)

	out, err := responder.Respond(context.Background(), agentproducer.ProducerContext{})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage == nil || len(out.ModelMessage.ToolCalls) != 1 {
		t.Fatalf("fixture first response tool calls = %#v", out.ModelMessage)
	}
	if out.ModelMessage.ToolCalls[0].Function.Name != "upsert_project_brief" {
		t.Fatalf("first fixture tool = %q", out.ModelMessage.ToolCalls[0].Function.Name)
	}
}

func TestValidateRealMediaE2EConfigRequiresRealVolcengineImageAndAudio(t *testing.T) {
	t.Setenv("CLIPANVIL_E2E_REQUIRE_REAL_MEDIA", "1")
	err := validateRealMediaE2EConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode:    "mock",
			DefaultProvider: "mock",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "provider_mode=real") {
		t.Fatalf("error = %v", err)
	}

	err = validateRealMediaE2EConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode:    "real",
			DefaultProvider: "volcengine",
			Volcengine: config.VolcengineConfig{
				APIKey:      "ark-key",
				AudioAPIKey: "speech-key",
				ImageModel:  "doubao-seedream-5-0-260128",
				AudioModel:  "seed-audio-1.0",
			},
		},
	})
	if err != nil {
		t.Fatalf("valid config error = %v", err)
	}
}

func TestTemplateOnlyVideoFixtureDoesNotRedispatchOnWorkerSignal(t *testing.T) {
	out, err := e2eTemplateOnlyVideoProducerResponder{}.Respond(context.Background(), agentproducer.ProducerContext{
		RuntimeTriggerText: "触发原因：worker_generation_completed。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage != nil && len(out.ModelMessage.ToolCalls) > 0 {
		t.Fatalf("worker signal should not dispatch tools: %#v", out.ModelMessage.ToolCalls)
	}
	if out.Metadata["signal_handled"] != "worker_generation_completed" {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
}

func TestTemplateOnlyVideoFixturePlansAudioAndTemplateVideo(t *testing.T) {
	fixture := e2eTemplateOnlyVideoProducerResponder{}
	wantTools := []string{
		"upsert_project_brief",
		"update_project_memory",
		"upsert_storyboard",
		"upsert_audio_plan",
		"upsert_audio_plan",
		"dispatch_craftsman",
		"dispatch_craftsman",
	}
	for count, want := range wantTools {
		out, err := fixture.Respond(context.Background(), agentproducer.ProducerContext{
			SameTurnMessages: templateOnlyToolResults(count),
		})
		if err != nil {
			t.Fatal(err)
		}
		if out.ModelMessage == nil || len(out.ModelMessage.ToolCalls) != 1 {
			t.Fatalf("count %d tool calls = %#v", count, out.ModelMessage)
		}
		call := out.ModelMessage.ToolCalls[0].Function
		if call.Name != want {
			t.Fatalf("count %d tool = %q, want %q", count, call.Name, want)
		}
		if count == 2 && !containsAll(call.Arguments, "开场痛点", "产品展示", "卖点卡", "CTA") {
			t.Fatalf("storyboard is not four-stage: %s", call.Arguments)
		}
		if count == 3 && !containsAll(call.Arguments, "voiceover_script", "cue_plan", "bgm_plan") {
			t.Fatalf("audio plan missing fields: %s", call.Arguments)
		}
		if count == 5 && !containsAll(call.Arguments, `"target_phase":"preview_image"`, `"input_node_refs":["box.png"]`) {
			t.Fatalf("preview image dispatch missing Seedream image route: %s", call.Arguments)
		}
		if count == 6 && !containsAll(call.Arguments, `"video_route_policy":"template_only"`, `"target_phase":"shot_video"`, `"shot_01_template_ad preview image"`) {
			t.Fatalf("shot video dispatch missing template_only policy: %s", call.Arguments)
		}
	}
}

func TestTemplateOnlyVideoFixtureDispatchesAudioOnContinuationMessages(t *testing.T) {
	fixture := e2eTemplateOnlyVideoProducerResponder{}
	tests := []struct {
		text      string
		wantPhase string
	}{
		{text: "继续生成模板视频", wantPhase: "shot_video"},
		{text: "继续生成旁白音频", wantPhase: "voiceover_audio"},
		{text: "继续生成 BGM 音频", wantPhase: "bgm_audio"},
	}
	for _, tt := range tests {
		out, err := fixture.Respond(context.Background(), agentproducer.ProducerContext{LatestUserText: tt.text})
		if err != nil {
			t.Fatal(err)
		}
		if out.ModelMessage == nil || len(out.ModelMessage.ToolCalls) != 1 {
			t.Fatalf("tool calls = %#v", out.ModelMessage)
		}
		call := out.ModelMessage.ToolCalls[0].Function
		if call.Name != "dispatch_craftsman" || !strings.Contains(call.Arguments, `"target_phase":"`+tt.wantPhase+`"`) {
			t.Fatalf("continuation %q dispatched %s %s", tt.text, call.Name, call.Arguments)
		}
		if tt.wantPhase == "shot_video" && !containsAll(call.Arguments, `"video_route_policy":"template_only"`, `"shot_01_template_ad preview image"`) {
			t.Fatalf("template video continuation missing preview input: %s", call.Arguments)
		}
	}
}

func TestTemplateOnlyVideoFixturePrioritizesFinalCompositionOverTemplateVideoContinuation(t *testing.T) {
	out, err := e2eTemplateOnlyVideoProducerResponder{}.Respond(context.Background(), agentproducer.ProducerContext{
		LatestUserText: "继续合成最终视频，使用已经成功的 HyperFrames/template video、真实火山旁白音频和真实火山 BGM 音频。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage == nil || len(out.ModelMessage.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", out.ModelMessage)
	}
	call := out.ModelMessage.ToolCalls[0].Function
	if call.Name != "dispatch_composer" {
		t.Fatalf("tool = %s args=%s, want dispatch_composer", call.Name, call.Arguments)
	}
}

func TestTemplateOnlyVideoFixtureStopsAfterComposerDispatchResult(t *testing.T) {
	out, err := e2eTemplateOnlyVideoProducerResponder{}.Respond(context.Background(), agentproducer.ProducerContext{
		LatestUserText: "继续合成最终视频，使用已经成功的 HyperFrames/template video、真实火山旁白音频和真实火山 BGM 音频。",
		SameTurnMessages: []agentproducer.ProducerSameTurnMessage{
			{Role: "assistant", MessageType: "tool_call", ToolName: "dispatch_composer"},
			{Role: "tool", MessageType: "tool_result", ToolName: "dispatch_composer", Content: `{"status":"queued"}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage != nil && len(out.ModelMessage.ToolCalls) > 0 {
		t.Fatalf("composer result should stop same-turn dispatches: %#v", out.ModelMessage.ToolCalls)
	}
	if out.Metadata["composer_dispatched"] != true {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
}

func TestTemplateOnlyVideoCraftsmanFixtureCreatesSeedreamPreviewImagePlan(t *testing.T) {
	out, err := e2eTemplateOnlyVideoCraftsmanResponder{}.Respond(context.Background(), agentcraftsman.Context{
		Input: agentcraftsman.GraphInput{
			Mode:          "preview_image",
			InputNodeRefs: []string{"box.png"},
		},
		Shot: dbShot("shot_01_template_ad", "悦行行李箱 4 段口播模板广告"),
		SameTurnMessages: []agentcraftsman.CraftsmanSameTurnMessage{{
			Role: "tool",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage == nil || len(out.ModelMessage.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", out.ModelMessage)
	}
	call := out.ModelMessage.ToolCalls[0].Function
	if call.Name != "upsert_render_plan" ||
		!containsAll(call.Arguments, `"model_prompt_profile":"seedream_5_image"`, `"target_phase":"preview_image"`, `"source_id":"box.png"`) {
		t.Fatalf("preview image plan = %s %s", call.Name, call.Arguments)
	}
}

func dbShot(clientKey string, title string) db.Shot {
	return db.Shot{
		ID:        pgtype.UUID{Bytes: [16]byte{0x11}, Valid: true},
		ClientKey: clientKey,
		Title:     title,
	}
}

func templateOnlyToolResults(count int) []agentproducer.ProducerSameTurnMessage {
	out := make([]agentproducer.ProducerSameTurnMessage, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, agentproducer.ProducerSameTurnMessage{Role: "tool", MessageType: "tool_result", Content: "{}"})
	}
	return out
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}

func TestProducerResponderForConfigUsesVolcengineWhenRealModeHasKey(t *testing.T) {
	responder := producerResponderForConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode: "real",
			Volcengine: config.VolcengineConfig{
				APIKey:    "test-key",
				BaseURL:   "https://example.com",
				Region:    "cn-beijing",
				TextModel: "test-model",
			},
		},
	}, nil)
	if _, ok := responder.(agentproducer.VolcengineModelResponder); !ok {
		t.Fatalf("expected Volcengine responder, got %T", responder)
	}
}

func TestContextFullSummarizerForConfigUsesStaticOutsideRealMode(t *testing.T) {
	summarizer := contextFullSummarizerForConfig(&config.Config{
		Production: config.ProductionConfig{ProviderMode: "mock"},
	})
	if _, ok := summarizer.(agentcontextcompact.StaticFullSummarizer); !ok {
		t.Fatalf("expected static summarizer, got %T", summarizer)
	}
}

func TestContextFullSummarizerForConfigUsesVolcengineWhenRealModeHasKey(t *testing.T) {
	summarizer := contextFullSummarizerForConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode: "real",
			Volcengine: config.VolcengineConfig{
				APIKey:    "test-key",
				TextModel: "doubao-summary",
			},
		},
	})
	if _, ok := summarizer.(agentcontextcompact.VolcengineFullSummarizer); !ok {
		t.Fatalf("expected Volcengine summarizer, got %T", summarizer)
	}
}

func TestAgentModelMaxTokenBudgetsAreLargeEnoughForNativeToolArguments(t *testing.T) {
	if producerModelMaxTokens < 4096 {
		t.Fatalf("producerModelMaxTokens = %d, want at least 4096", producerModelMaxTokens)
	}
	if craftsmanModelMaxTokens < 8192 {
		t.Fatalf("craftsmanModelMaxTokens = %d, want at least 8192", craftsmanModelMaxTokens)
	}
	if reviewerModelMaxTokens < 4096 {
		t.Fatalf("reviewerModelMaxTokens = %d, want at least 4096", reviewerModelMaxTokens)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
