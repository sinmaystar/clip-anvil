package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentcomposer "github.com/sinmaystar/clip-anvil/internal/agent/composer"
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

func TestMotionShotVideoFixtureDoesNotRedispatchOnWorkerSignal(t *testing.T) {
	out, err := e2eMotionShotVideoProducerResponder{}.Respond(context.Background(), agentproducer.ProducerContext{
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

func TestMotionShotVideoFixturePlansAudioAndMotionShotVideo(t *testing.T) {
	fixture := e2eMotionShotVideoProducerResponder{}
	wantTools := []string{
		"upsert_project_brief",
		"update_project_memory",
		"upsert_storyboard",
		"upsert_audio_plan",
		"upsert_audio_plan",
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
		if count == 2 && !containsAll(call.Arguments,
			`"client_key":"shot_01_hook"`,
			`"client_key":"shot_02_product"`,
			`"client_key":"shot_03_wheels"`,
			`"client_key":"shot_04_storage"`,
			`"client_key":"shot_05_cta"`,
			`"duration_sec":6`,
			`"duration_sec":8`,
		) {
			t.Fatalf("storyboard is not dynamic multi-shot: %s", call.Arguments)
		}
		if count == 3 && !containsAll(call.Arguments, `"target_duration_sec":34`, `"voiceover_script"`, `"cue_plan"`, `"bgm_plan"`) {
			t.Fatalf("audio plan missing 34s fields: %s", call.Arguments)
		}
		if count == 5 && !containsAll(call.Arguments, `"target_phase":"preview_image"`, `"shot_refs":["shot_01_hook","shot_02_product","shot_03_wheels","shot_04_storage","shot_05_cta"]`) {
			t.Fatalf("preview image dispatch missing multi-shot route: %s", call.Arguments)
		}
		if strings.Contains(call.Arguments, "seedance_2_video") {
			t.Fatalf("fixture must not mention seedance_2_video: %s", call.Arguments)
		}
	}
	out, err := fixture.Respond(context.Background(), agentproducer.ProducerContext{
		SameTurnMessages: templateOnlyToolResults(len(wantTools)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage != nil && len(out.ModelMessage.ToolCalls) > 0 {
		t.Fatalf("initial turn should stop after preview dispatch, got %#v", out.ModelMessage.ToolCalls)
	}
}

func TestMotionShotVideoFixtureDoesNotTreatInitialBriefAsContinuation(t *testing.T) {
	out, err := e2eMotionShotVideoProducerResponder{}.Respond(context.Background(), agentproducer.ProducerContext{
		LatestUserText: "使用 box.png 生成悦行行李箱口播广告，视频用 Remotion motion shot，不要调用 Seedance。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage == nil || len(out.ModelMessage.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", out.ModelMessage)
	}
	call := out.ModelMessage.ToolCalls[0].Function
	if call.Name != "upsert_project_brief" {
		t.Fatalf("initial brief dispatched %s %s, want upsert_project_brief", call.Name, call.Arguments)
	}
	if out.Metadata["e2e_fixture"] != "motion_shot_video" {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
}

func TestMotionShotVideoFixtureDispatchesAudioOnContinuationMessages(t *testing.T) {
	fixture := e2eMotionShotVideoProducerResponder{}
	tests := []struct {
		text      string
		wantPhase string
	}{
		{text: "继续生成动效视频", wantPhase: "shot_video"},
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
		if tt.wantPhase == "shot_video" && !containsAll(call.Arguments, `"video_route_policy":"motion_only"`, `"shot_01_hook.preview_image.r1.node"`, `"shot_05_cta.preview_image.r1.node"`) {
			t.Fatalf("motion shot continuation missing preview input: %s", call.Arguments)
		}
	}
}

func TestMotionShotVideoFixtureRepairsAudioPlanDuration(t *testing.T) {
	fixture := e2eMotionShotVideoProducerResponder{}
	out, err := fixture.Respond(context.Background(), agentproducer.ProducerContext{
		LatestUserText: "修复旁白时长，重新生成 AudioPlan。",
	})
	if err != nil {
		t.Fatal(err)
	}
	call := out.ModelMessage.ToolCalls[0].Function
	if call.Name != "upsert_audio_plan" || !containsAll(call.Arguments, `"mode":"replace_draft"`, "地铁换乘和酒店大厅", "狭窄通道也能轻松掉头") {
		t.Fatalf("repair did not rewrite longer AudioPlan: %s %s", call.Name, call.Arguments)
	}
	out, err = fixture.Respond(context.Background(), agentproducer.ProducerContext{
		LatestUserText:   "修复旁白时长，重新生成 AudioPlan。",
		SameTurnMessages: templateOnlyToolResults(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	call = out.ModelMessage.ToolCalls[0].Function
	if call.Name != "upsert_audio_plan" || !strings.Contains(call.Arguments, `"mode":"approve"`) {
		t.Fatalf("repair did not approve AudioPlan: %s %s", call.Name, call.Arguments)
	}
}

func TestMotionShotVideoFixturePrioritizesFinalCompositionOverMotionShotContinuation(t *testing.T) {
	for _, text := range []string{
		"继续合成最终视频，使用已经成功的 Remotion motion shot、真实火山旁白音频和真实火山 BGM 音频。",
		"重新合成最终视频，只使用已经成功的 Remotion motion shot、34 秒火山旁白和 BGM。不要重新生成图片、不要调用 Seedance。",
	} {
		out, err := e2eMotionShotVideoProducerResponder{}.Respond(context.Background(), agentproducer.ProducerContext{
			LatestUserText: text,
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
}

func TestMotionShotVideoFixtureStopsAfterComposerDispatchResult(t *testing.T) {
	out, err := e2eMotionShotVideoProducerResponder{}.Respond(context.Background(), agentproducer.ProducerContext{
		LatestUserText: "继续合成最终视频，使用已经成功的 Remotion motion shot、真实火山旁白音频和真实火山 BGM 音频。",
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

func TestMotionShotVideoCraftsmanFixtureCreatesSeedreamPreviewImagePlan(t *testing.T) {
	out, err := e2eMotionShotVideoCraftsmanResponder{}.Respond(context.Background(), agentcraftsman.Context{
		Input: agentcraftsman.GraphInput{
			Mode:          "preview_image",
			InputNodeRefs: []string{"box.png"},
		},
		Shot: dbShot("shot_01_motion_ad", "悦行行李箱 4 段口播模板广告"),
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

func TestMotionShotVideoCraftsmanFixtureVariesPlansByShotFacts(t *testing.T) {
	fixture := e2eMotionShotVideoCraftsmanResponder{}
	shots := []db.Shot{
		{ID: e2eUUIDWithByte(11), ClientKey: "shot_01_hook", Title: "痛点开场", DurationSec: pgtype.Float8{Float64: 6, Valid: true}, NarrativePurpose: "痛点钩子", VisualIntent: "大标题上方", ActionText: "商品轻推近", CameraIntent: "慢推", Narration: "短途出行，最怕箱子沉、转弯卡。"},
		{ID: e2eUUIDWithByte(12), ClientKey: "shot_03_wheels", Title: "万向轮卖点", DurationSec: pgtype.Float8{Float64: 8, Valid: true}, NarrativePurpose: "证明顺滑", VisualIntent: "三点卖点卡", ActionText: "卖点逐条入场", CameraIntent: "稳定信息卡", Narration: "顺滑万向轮，转向更稳。"},
	}
	plans := []string{}
	for _, shot := range shots {
		out, err := fixture.Respond(context.Background(), agentcraftsman.Context{
			Input: agentcraftsman.GraphInput{Mode: "shot_video", InputNodeRefs: []string{shot.ClientKey + " preview image"}, VideoRoutePolicy: "motion_only"},
			Shot:  shot,
			SameTurnMessages: []agentcraftsman.CraftsmanSameTurnMessage{{
				Role: "tool",
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		call := out.ModelMessage.ToolCalls[0].Function
		if call.Name != "upsert_render_plan" {
			t.Fatalf("tool = %s", call.Name)
		}
		if !containsAll(call.Arguments, `"model_prompt_profile":"motion_shot_video"`, `"operation":"image_to_motion_video"`, `"video_route_policy=motion_only"`) {
			t.Fatalf("motion plan missing route policy: %s", call.Arguments)
		}
		if strings.Contains(call.Arguments, `"model_prompt_profile":"seedance_2_video"`) {
			t.Fatalf("motion fixture emitted Seedance: %s", call.Arguments)
		}
		plans = append(plans, call.Arguments)
	}
	if plans[0] == plans[1] {
		t.Fatal("different shots produced identical motion plans")
	}
	if !containsAll(plans[0], `"duration_sec":6`) || !containsAll(plans[1], `"duration_sec":8`) {
		t.Fatalf("plans did not inherit durations: %#v", plans)
	}
}

func TestMotionShotVideoCraftsmanFixtureCreatesShotSpecificSeedreamVisuals(t *testing.T) {
	fixture := e2eMotionShotVideoCraftsmanResponder{}
	cases := []struct {
		shot    db.Shot
		wantAll []string
	}{
		{
			shot: db.Shot{ID: e2eUUIDWithByte(12), ClientKey: "shot_03_wheels", Title: "顺滑万向轮卖点", VisualIntent: "底部轮子细节"},
			wantAll: []string{
				`"target_phase":"preview_image"`,
				"底部万向轮超近景特写",
				"轮组占画面 55%",
				"不要完整行李箱大图",
			},
		},
		{
			shot: db.Shot{ID: e2eUUIDWithByte(13), ClientKey: "shot_04_storage", Title: "大周出行轻松收纳", VisualIntent: "内部分区收纳"},
			wantAll: []string{
				`"target_phase":"preview_image"`,
				"打开的银灰色行李箱内景",
				"衣物、电脑、洗漱包分区",
				"不要万向轮特写",
			},
		},
	}
	for _, tc := range cases {
		out, err := fixture.Respond(context.Background(), agentcraftsman.Context{
			Input: agentcraftsman.GraphInput{Mode: "preview_image", InputNodeRefs: []string{"box.png"}},
			Shot:  tc.shot,
			SameTurnMessages: []agentcraftsman.CraftsmanSameTurnMessage{{
				Role: "tool",
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		call := out.ModelMessage.ToolCalls[0].Function
		if call.Name != "upsert_render_plan" {
			t.Fatalf("tool = %s", call.Name)
		}
		if !containsAll(call.Arguments, tc.wantAll...) {
			t.Fatalf("shot-specific Seedream prompt missing constraints: %s", call.Arguments)
		}
	}
}

func TestMotionShotVideoComposerFixtureBuildsThirtyFourSecondMultiShotTimeline(t *testing.T) {
	context := agentcomposer.Context{
		SameTurnMessages: []agentcomposer.ComposerSameTurnMessage{
			{
				Role:     "tool",
				ToolName: "get_composition_context",
				Content: `{
					"source_storyboard_node_id":"shot_01_hook.shot_video.r1.node",
					"audio_plan":{
						"cue_plan":[
							{"shot_ref":"shot_01_hook","text":"短途出行，最怕箱子沉、转弯卡，一路拖得很狼狈；地铁换乘和酒店大厅，每一步都怕被行李拖慢。"},
							{"shot_ref":"shot_02_product","text":"悦行行李箱采用轻量硬壳和顺滑手感，从地铁口到酒店前台都推得更稳。"},
							{"shot_ref":"shot_03_wheels","text":"底部万向轮顺滑转向，转弯不抢手，狭窄通道也能轻松掉头，赶车换乘更省力。"},
							{"shot_ref":"shot_04_storage","text":"两三天换洗衣物、电脑和洗漱包分区放好，拉链网袋一眼看清，打开就能快速拿取。"},
							{"shot_ref":"shot_05_cta","text":"周末旅行、商务通勤、短途回家，一个箱子装下刚刚好的从容。悦行行李箱，现在出发。"}
						]
					},
					"available_composition_assets":[
						{"role":"clip","asset_id":"shot_01_hook_clip","source_url":"http://example/shot_01_hook.mp4","file_name":"shot_01_hook.mp4","mime_type":"video/mp4"},
						{"role":"clip","asset_id":"shot_02_product_clip","source_url":"http://example/shot_02_product.mp4","file_name":"shot_02_product.mp4","mime_type":"video/mp4"},
						{"role":"clip","asset_id":"shot_03_wheels_clip","source_url":"http://example/shot_03_wheels.mp4","file_name":"shot_03_wheels.mp4","mime_type":"video/mp4"},
						{"role":"clip","asset_id":"shot_04_storage_clip","source_url":"http://example/shot_04_storage.mp4","file_name":"shot_04_storage.mp4","mime_type":"video/mp4"},
						{"role":"clip","asset_id":"shot_05_cta_clip","source_url":"http://example/shot_05_cta.mp4","file_name":"shot_05_cta.mp4","mime_type":"video/mp4"},
						{"role":"voiceover","asset_id":"voiceover_asset","source_url":"http://example/voiceover.mp3","file_name":"voiceover.mp3","mime_type":"audio/mpeg","metadata":{"duration_sec":34}},
						{"role":"bgm","asset_id":"bgm_asset","source_url":"http://example/bgm.mp3","file_name":"bgm.mp3","mime_type":"audio/mpeg"}
					]
				}`,
			},
			{
				Role:     "tool",
				ToolName: "stage_media_inputs",
				Content: `{
					"files":[
						{"asset_id":"shot_01_hook_clip","workspace_path":"/workspace/input/shot_01_hook.mp4","file_name":"shot_01_hook.mp4","mime_type":"video/mp4"},
						{"asset_id":"shot_02_product_clip","workspace_path":"/workspace/input/shot_02_product.mp4","file_name":"shot_02_product.mp4","mime_type":"video/mp4"},
						{"asset_id":"shot_03_wheels_clip","workspace_path":"/workspace/input/shot_03_wheels.mp4","file_name":"shot_03_wheels.mp4","mime_type":"video/mp4"},
						{"asset_id":"shot_04_storage_clip","workspace_path":"/workspace/input/shot_04_storage.mp4","file_name":"shot_04_storage.mp4","mime_type":"video/mp4"},
						{"asset_id":"shot_05_cta_clip","workspace_path":"/workspace/input/shot_05_cta.mp4","file_name":"shot_05_cta.mp4","mime_type":"video/mp4"},
						{"asset_id":"voiceover_asset","workspace_path":"/workspace/input/voiceover.mp3","file_name":"voiceover.mp3","mime_type":"audio/mpeg"},
						{"asset_id":"bgm_asset","workspace_path":"/workspace/input/bgm.mp3","file_name":"bgm.mp3","mime_type":"audio/mpeg"}
					]
				}`,
			},
		},
	}
	plan, err := e2eTimelinePlan(context)
	if err != nil {
		t.Fatal(err)
	}
	segments := plan["segments"].([]map[string]any)
	if len(segments) != 5 {
		t.Fatalf("segments = %#v", segments)
	}
	if segments[0]["id"] != "shot_01_hook" || segments[1]["start_sec"] != 6 || segments[4]["start_sec"] != 28 || segments[4]["duration_sec"] != 6 {
		t.Fatalf("unexpected segments = %#v", segments)
	}
	if segments[0]["caption"] != "短途出行，最怕箱子沉、转弯卡，一路拖得很狼狈；地铁换乘和酒店大厅，每一步都怕被行李拖慢。" || segments[2]["caption"] != "底部万向轮顺滑转向，转弯不抢手，狭窄通道也能轻松掉头，赶车换乘更省力。" {
		t.Fatalf("segments missing cue captions = %#v", segments)
	}
	audioTracks := plan["audio_tracks"].([]map[string]any)
	if len(audioTracks) != 2 || audioTracks[0]["duration_sec"] != 34 || audioTracks[1]["duration_sec"] != 34 {
		t.Fatalf("audio tracks = %#v", audioTracks)
	}
	output := plan["output"].(map[string]any)
	if output["workspace_path"] != "/workspace/output/yuexing-dynamic-remotion-final.mp4" {
		t.Fatalf("output = %#v", output)
	}
}

func TestMotionShotVideoComposerFixtureBlocksShortVoiceover(t *testing.T) {
	context := agentcomposer.Context{
		SameTurnMessages: []agentcomposer.ComposerSameTurnMessage{
			{
				Role:     "tool",
				ToolName: "get_composition_context",
				Content: `{
					"source_storyboard_node_id":"shot_01_hook.shot_video.r1.node",
					"audio_plan":{"cue_plan":[]},
					"available_composition_assets":[
						{"role":"clip","asset_id":"shot_01_hook_clip","source_url":"http://example/shot_01_hook.mp4","file_name":"shot_01_hook.mp4","mime_type":"video/mp4"},
						{"role":"clip","asset_id":"shot_02_product_clip","source_url":"http://example/shot_02_product.mp4","file_name":"shot_02_product.mp4","mime_type":"video/mp4"},
						{"role":"clip","asset_id":"shot_03_wheels_clip","source_url":"http://example/shot_03_wheels.mp4","file_name":"shot_03_wheels.mp4","mime_type":"video/mp4"},
						{"role":"clip","asset_id":"shot_04_storage_clip","source_url":"http://example/shot_04_storage.mp4","file_name":"shot_04_storage.mp4","mime_type":"video/mp4"},
						{"role":"clip","asset_id":"shot_05_cta_clip","source_url":"http://example/shot_05_cta.mp4","file_name":"shot_05_cta.mp4","mime_type":"video/mp4"},
						{"role":"voiceover","asset_id":"voiceover_asset","source_url":"http://example/voiceover.mp3","file_name":"voiceover.mp3","mime_type":"audio/mpeg","metadata":{"duration_sec":17.8}},
						{"role":"bgm","asset_id":"bgm_asset","source_url":"http://example/bgm.mp3","file_name":"bgm.mp3","mime_type":"audio/mpeg"}
					]
				}`,
			},
			{
				Role:     "tool",
				ToolName: "stage_media_inputs",
				Content: `{
					"files":[
						{"asset_id":"shot_01_hook_clip","workspace_path":"/workspace/input/shot_01_hook.mp4","file_name":"shot_01_hook.mp4","mime_type":"video/mp4"},
						{"asset_id":"shot_02_product_clip","workspace_path":"/workspace/input/shot_02_product.mp4","file_name":"shot_02_product.mp4","mime_type":"video/mp4"},
						{"asset_id":"shot_03_wheels_clip","workspace_path":"/workspace/input/shot_03_wheels.mp4","file_name":"shot_03_wheels.mp4","mime_type":"video/mp4"},
						{"asset_id":"shot_04_storage_clip","workspace_path":"/workspace/input/shot_04_storage.mp4","file_name":"shot_04_storage.mp4","mime_type":"video/mp4"},
						{"asset_id":"shot_05_cta_clip","workspace_path":"/workspace/input/shot_05_cta.mp4","file_name":"shot_05_cta.mp4","mime_type":"video/mp4"},
						{"asset_id":"voiceover_asset","workspace_path":"/workspace/input/voiceover.mp3","file_name":"voiceover.mp3","mime_type":"audio/mpeg"},
						{"asset_id":"bgm_asset","workspace_path":"/workspace/input/bgm.mp3","file_name":"bgm.mp3","mime_type":"audio/mpeg"}
					]
				}`,
			},
		},
	}
	_, err := e2eTimelinePlan(context)
	if err == nil || !strings.Contains(err.Error(), "voiceover duration 17.8s is too short") {
		t.Fatalf("expected short voiceover block, got %v", err)
	}
}

func dbShot(clientKey string, title string) db.Shot {
	return db.Shot{
		ID:        pgtype.UUID{Bytes: [16]byte{0x11}, Valid: true},
		ClientKey: clientKey,
		Title:     title,
	}
}

func e2eUUIDWithByte(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{value}, Valid: true}
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
