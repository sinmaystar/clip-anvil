package composer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/remotiontimeline"
)

func TestVolcengineModelResponderAppliesContextCompactionProjection(t *testing.T) {
	longResult := strings.Repeat("ffmpeg stderr line with frame details\n", 180)
	model := &fakeComposerArkModel{final: &schema.Message{Role: schema.Assistant, Content: "ok"}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:     compactComposerResponderTestConfig(),
			Store:      contextcompactTestStore(),
			FileWriter: contextcompactTestFileWriter(),
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{
		WorkspaceID: uuidWithByte(1),
		Input: GraphInput{
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(2),
			TaskID:      uuidWithByte(3),
			Input:       CompositionInput{Instructions: "compose"},
		},
		SameTurnMessages: []ComposerSameTurnMessage{
			{Role: "tool", MessageType: "tool_result", ToolCallID: "call-old", ToolName: "run_ffmpeg_command", Content: longResult},
			{Role: "assistant", MessageType: "tool_call", ToolCallID: "call-current", ToolName: "probe_media", ToolArguments: map[string]any{"path": "/workspace/current.mp4"}},
			{Role: "tool", MessageType: "tool_result", ToolCallID: "call-current", ToolName: "probe_media", Content: "current probe result must stay visible"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Metadata["context_compaction_applied"] != true {
		t.Fatalf("metadata missing compaction applied: %#v", out.Metadata)
	}
	if out.Metadata["context_compaction_mode"] != "micro" || out.Metadata["context_compaction_count"] != 1 {
		t.Fatalf("metadata missing compaction mode/count: %#v", out.Metadata)
	}
	if refs, ok := out.Metadata["context_compaction_refs"].([]string); !ok || len(refs) == 0 {
		t.Fatalf("metadata missing compaction refs: %#v", out.Metadata)
	}
	if files, ok := out.Metadata["context_compaction_detail_files"].([]string); !ok || len(files) == 0 {
		t.Fatalf("metadata missing compaction detail files: %#v", out.Metadata)
	}
	if len(model.messages) < 5 {
		t.Fatalf("messages = %#v", model.messages)
	}
	oldTool := model.messages[2]
	if oldTool.Role != schema.Tool || oldTool.ToolCallID != "call-old" || oldTool.ToolName != "run_ffmpeg_command" {
		t.Fatalf("old tool identity changed: %#v", oldTool)
	}
	if !strings.Contains(oldTool.Content, "compact_ref:") || strings.Contains(oldTool.Content, "ffmpeg stderr line with frame details") {
		t.Fatalf("old tool result not compacted correctly: %s", oldTool.Content)
	}
	currentTool := model.messages[len(model.messages)-1]
	if !strings.Contains(currentTool.Content, "current probe result must stay visible") {
		t.Fatalf("current tool loop result was compacted: %#v", currentTool)
	}
}

func TestVolcengineModelResponderRetriesOnceWithFullCompactOnContextOverflow(t *testing.T) {
	model := &fakeComposerArkModel{
		final:        &schema.Message{Role: schema.Assistant, Content: "ok"},
		generateErrs: []error{errors.New("context length exceeded")},
	}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:         compactComposerResponderTestConfig(),
			Store:          contextcompactTestStore(),
			FileWriter:     contextcompactTestFileWriter(),
			FullSummarizer: contextcompact.StaticFullSummarizer{},
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{
		WorkspaceID: uuidWithByte(1),
		Input: GraphInput{
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(2),
			TaskID:      uuidWithByte(3),
			Input:       CompositionInput{Instructions: "compose"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Metadata["context_compaction_retry"] != true {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
	if model.generateCalls != 2 {
		t.Fatalf("generateCalls = %d, want 2", model.generateCalls)
	}
	if !strings.Contains(model.messages[1].Content, "# Compacted Agent Handoff Summary") {
		t.Fatalf("retry prompt missing full summary: %#v", model.messages)
	}
}

func TestComposerCompactsOldProbeOutputButPreservesCurrentTimelinePath(t *testing.T) {
	longProbe := strings.Repeat("ffprobe stream codec duration bitrate\n", 1200)
	model := &fakeComposerArkModel{final: &schema.Message{Role: schema.Assistant, Content: "ok"}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:         compactComposerResponderTestConfig(),
			Store:          contextcompactTestStore(),
			FileWriter:     contextcompactTestFileWriter(),
			FullSummarizer: contextcompact.StaticFullSummarizer{},
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})

	_, err := responder.Respond(context.Background(), Context{
		WorkspaceID: uuidWithByte(1),
		Input: GraphInput{
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(2),
			TaskID:      uuidWithByte(3),
			Input:       CompositionInput{Instructions: "compose"},
		},
		SameTurnMessages: []ComposerSameTurnMessage{
			{Role: "tool", MessageType: "tool_result", ToolCallID: "old-probe", ToolName: "probe_media", Content: longProbe},
			{Role: "assistant", MessageType: "tool_call", ToolCallID: "current-render", ToolName: "render_timeline", ToolArguments: map[string]any{"timeline_path": "/workspace/timeline/current.json"}},
			{Role: "tool", MessageType: "tool_result", ToolCallID: "current-render", ToolName: "render_timeline", Content: "current timeline path /workspace/timeline/current.json must stay visible"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.messages[2].Content, "compact_ref:") {
		t.Fatalf("old probe was not compacted: %#v", model.messages)
	}
	if !strings.Contains(model.messages[len(model.messages)-1].Content, "/workspace/timeline/current.json") {
		t.Fatalf("current timeline path not preserved: %#v", model.messages)
	}
}

func TestComposerResponderUsesDeterministicTemplatePathBeforeModel(t *testing.T) {
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{})

	out, err := responder.Respond(context.Background(), Context{
		Input: GraphInput{Input: CompositionInput{
			TemplateKey:            "concat_with_fades",
			SourceStoryboardNodeID: "04000000-0000-0000-0000-000000000000",
			Instructions:           "compose final video",
			ProducerTaskID:         "producer-task",
			ParentToolCallID:       "producer-tool-call",
		}},
	})
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if out.ModelMessage == nil || len(out.ModelMessage.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v, want one deterministic call", out.ModelMessage)
	}
	call := out.ModelMessage.ToolCalls[0]
	if call.Function.Name != "get_composition_context" {
		t.Fatalf("tool = %q, want get_composition_context", call.Function.Name)
	}
	if !strings.Contains(call.Function.Arguments, "04000000-0000-0000-0000-000000000000") {
		t.Fatalf("arguments missing source id: %s", call.Function.Arguments)
	}
	if out.Metadata["provider"] != "deterministic_template" {
		t.Fatalf("metadata provider = %v", out.Metadata["provider"])
	}
}

func TestDeterministicComposerTimelinePlanUsesAudioCuesForSegmentTimingAndCaptions(t *testing.T) {
	compositionContext := map[string]any{
		"audio_plan": map[string]any{
			"target_duration_sec": float64(12),
			"cue_plan": []any{
				map[string]any{"shot_ref": "shot_capacity", "start_sec": float64(0), "end_sec": float64(6), "text": "两三天短途，分区收纳。", "caption": "分区收纳"},
				map[string]any{"shot_ref": "shot_wheels", "start_sec": float64(6), "end_sec": float64(12), "text": "顺滑万向轮，推着走更轻松。", "caption": "顺滑万向轮"},
			},
		},
		"available_composition_assets": []any{
			map[string]any{"role": "clip", "shot_ref": "shot_wheels", "asset_id": "asset-wheels", "mime_type": "video/mp4"},
			map[string]any{"role": "clip", "shot_ref": "shot_capacity", "asset_id": "asset-capacity", "mime_type": "video/mp4"},
			map[string]any{"role": "voiceover", "asset_id": "asset-voice", "mime_type": "audio/mpeg", "metadata": map[string]any{"duration_sec": float64(10)}},
		},
	}
	staged := map[string]any{
		"files": []any{
			map[string]any{"asset_id": "asset-wheels", "workspace_path": "/workspace/input/wheels.mp4"},
			map[string]any{"asset_id": "asset-capacity", "workspace_path": "/workspace/input/capacity.mp4"},
			map[string]any{"asset_id": "asset-voice", "workspace_path": "/workspace/input/voiceover.mp3"},
		},
	}
	context := Context{SameTurnMessages: []ComposerSameTurnMessage{
		composerToolResultMessage(t, "get_composition_context", compositionContext),
		composerToolResultMessage(t, "stage_media_inputs", staged),
	}}

	plan, err := deterministicComposerTimelinePlan(context, "concat_with_fades")
	if err != nil {
		t.Fatal(err)
	}
	segments, ok := plan["segments"].([]any)
	if !ok || len(segments) != 2 {
		t.Fatalf("segments = %#v", plan["segments"])
	}
	first := segments[0].(map[string]any)
	second := segments[1].(map[string]any)
	if first["id"] != "shot_capacity" || first["caption"] != "分区收纳" || first["workspace_path"] != "/workspace/input/capacity.mp4" {
		t.Fatalf("first segment not cue-ordered/captioned: %#v", first)
	}
	if second["id"] != "shot_wheels" || second["caption"] != "顺滑万向轮" || second["workspace_path"] != "/workspace/input/wheels.mp4" {
		t.Fatalf("second segment not cue-ordered/captioned: %#v", second)
	}
	if first["duration_sec"] != float64(5) || second["duration_sec"] != float64(5) {
		t.Fatalf("cue durations should be scaled to 10s voiceover duration: %#v / %#v", first["duration_sec"], second["duration_sec"])
	}
}

func TestDeterministicComposerRemotionTimelinePlanUsesCuePlanAndStills(t *testing.T) {
	compositionContext := map[string]any{
		"audio_plan": map[string]any{
			"target_duration_sec": float64(30),
			"cue_plan": []any{
				map[string]any{"shot_ref": "shot_wheels", "start_sec": float64(0), "end_sec": float64(15), "text": "顺滑万向轮，转弯不费力。", "caption": "顺滑万向轮"},
				map[string]any{"shot_ref": "shot_storage", "start_sec": float64(15), "end_sec": float64(30), "text": "打开就是分区收纳。", "caption": "分区收纳"},
			},
		},
		"available_composition_assets": []any{
			map[string]any{"role": "still", "shot_ref": "shot_storage", "shot_title": "打开收纳", "asset_id": "asset-storage", "mime_type": "image/png", "visual_intent": "打开箱体内景"},
			map[string]any{"role": "still", "shot_ref": "shot_wheels", "shot_title": "万向轮特写", "asset_id": "asset-wheels", "mime_type": "image/png", "visual_intent": "轮组近景"},
			map[string]any{"role": "voiceover", "asset_id": "asset-voice", "mime_type": "audio/mpeg", "metadata": map[string]any{"duration_sec": float64(30)}},
			map[string]any{"role": "bgm", "asset_id": "asset-bgm", "mime_type": "audio/mpeg"},
		},
	}
	staged := map[string]any{
		"files": []any{
			map[string]any{"asset_id": "asset-storage", "workspace_path": "/workspace/input/storage.png"},
			map[string]any{"asset_id": "asset-wheels", "workspace_path": "/workspace/input/wheels.png"},
			map[string]any{"asset_id": "asset-voice", "workspace_path": "/workspace/input/voiceover.mp3"},
			map[string]any{"asset_id": "asset-bgm", "workspace_path": "/workspace/input/bgm.mp3"},
		},
	}
	context := Context{SameTurnMessages: []ComposerSameTurnMessage{
		composerToolResultMessage(t, "get_composition_context", compositionContext),
		composerToolResultMessage(t, "stage_media_inputs", staged),
	}}

	plan, err := deterministicComposerTimelinePlan(context, "remotion_timeline_v1")
	if err != nil {
		t.Fatal(err)
	}
	if plan["schema"] != "clipanvil.remotion_timeline.v1" || plan["composition"] != "MarketingTimeline" {
		t.Fatalf("remotion plan header = %#v", plan)
	}
	output, _ := plan["output"].(map[string]any)
	if output["width"] != 1080 || output["height"] != 1920 || output["duration_sec"] != float64(30) {
		t.Fatalf("output = %#v", output)
	}
	segments, ok := plan["segments"].([]any)
	if !ok || len(segments) != 2 {
		t.Fatalf("segments = %#v", plan["segments"])
	}
	first := segments[0].(map[string]any)
	second := segments[1].(map[string]any)
	if first["id"] != "shot_wheels" || first["start_sec"] != float64(0) || first["end_sec"] != float64(15) {
		t.Fatalf("first segment = %#v", first)
	}
	if second["id"] != "shot_storage" || second["start_sec"] != float64(15) || second["end_sec"] != float64(30) {
		t.Fatalf("second segment = %#v", second)
	}
	firstAssets := first["assets"].([]any)
	if firstAssets[0].(map[string]any)["workspace_path"] != "/workspace/input/wheels.png" {
		t.Fatalf("wheel cue not matched to wheel still: %#v", firstAssets)
	}
	if firstAssets[0].(map[string]any)["type"] != "image" {
		t.Fatalf("no-Seedance still fixture should use image segment, got %#v", firstAssets[0])
	}
	firstCaption := first["caption"].(map[string]any)
	if firstCaption["text"] != "顺滑万向轮" || firstCaption["source"] != "audio_cue" {
		t.Fatalf("caption should come from cue caption: %#v", firstCaption)
	}
	rawPlan := string(mustComposerJSON(t, plan))
	if strings.Contains(rawPlan, "轮组近景") || strings.Contains(rawPlan, "打开箱体内景") {
		t.Fatalf("internal visual_intent leaked into captions or text layers: %#v", plan)
	}
	decoded, err := remotiontimeline.Decode(plan)
	if err != nil {
		t.Fatalf("remotion timeline decode failed: %v", err)
	}
	if err := remotiontimeline.Validate(decoded); err != nil {
		t.Fatalf("remotion timeline validation failed: %v", err)
	}
	audioTracks, ok := plan["audio_tracks"].([]any)
	if !ok || len(audioTracks) != 2 {
		t.Fatalf("audio_tracks = %#v", plan["audio_tracks"])
	}
}

func TestDeterministicComposerRemotionTimelinePlanSupportsMixedVideoAndImageSegments(t *testing.T) {
	compositionContext := map[string]any{
		"audio_plan": map[string]any{
			"target_duration_sec": float64(18),
			"cue_plan": []any{
				map[string]any{"shot_ref": "shot_hero", "start_sec": float64(0), "end_sec": float64(6), "text": "开场展示悦行行李箱，轻松出发。", "caption": "轻松出发"},
				map[string]any{"shot_ref": "shot_wheels", "start_sec": float64(6), "end_sec": float64(12), "text": "顺滑万向轮，转弯不费力。", "caption": "顺滑万向轮"},
				map[string]any{"shot_ref": "shot_storage", "start_sec": float64(12), "end_sec": float64(18), "text": "打开就是分区收纳。", "caption": "分区收纳"},
			},
		},
		"available_composition_assets": []any{
			map[string]any{"role": "clip", "shot_ref": "shot_hero", "shot_title": "Seedance hero video", "asset_id": "asset-hero-video", "mime_type": "video/mp4", "file_name": "hero-seedance.mp4"},
			map[string]any{"role": "still", "shot_ref": "shot_wheels", "shot_title": "万向轮特写", "asset_id": "asset-wheels", "mime_type": "image/png", "file_name": "wheel-detail.png"},
			map[string]any{"role": "still", "shot_ref": "shot_storage", "shot_title": "打开收纳", "asset_id": "asset-storage", "mime_type": "image/png", "file_name": "open-storage.png"},
			map[string]any{"role": "voiceover", "asset_id": "asset-voice", "mime_type": "audio/mpeg", "metadata": map[string]any{"duration_sec": float64(18)}},
		},
	}
	staged := map[string]any{"files": []any{
		map[string]any{"asset_id": "asset-hero-video", "workspace_path": "/workspace/input/hero-seedance.mp4"},
		map[string]any{"asset_id": "asset-wheels", "workspace_path": "/workspace/input/wheel-detail.png"},
		map[string]any{"asset_id": "asset-storage", "workspace_path": "/workspace/input/open-storage.png"},
		map[string]any{"asset_id": "asset-voice", "workspace_path": "/workspace/input/voiceover.mp3"},
	}}
	context := Context{SameTurnMessages: []ComposerSameTurnMessage{
		composerToolResultMessage(t, "get_composition_context", compositionContext),
		composerToolResultMessage(t, "stage_media_inputs", staged),
	}}

	plan, err := deterministicComposerTimelinePlan(context, "remotion_timeline_v1")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := remotiontimeline.Decode(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := remotiontimeline.Validate(decoded); err != nil {
		t.Fatal(err)
	}
	types := map[string]int{}
	for _, segment := range decoded.Segments {
		for _, asset := range segment.Assets {
			types[asset.Type]++
		}
	}
	if types["video"] != 1 || types["image"] != 2 {
		t.Fatalf("asset types = %#v, want one video and two image segments; plan=%#v", types, decoded.Segments)
	}
	if decoded.Segments[0].Assets[0].WorkspacePath != "/workspace/input/hero-seedance.mp4" {
		t.Fatalf("hero cue should use staged video clip: %#v", decoded.Segments[0])
	}
}

func TestDeterministicComposerRemotionTimelinePlanUsesLayoutDiversity(t *testing.T) {
	cuePlan := []any{
		map[string]any{"shot_ref": "shot_hero", "start_sec": float64(0), "end_sec": float64(6), "text": "悦行行李箱，短途轻松出发。", "caption": "短途轻松出发"},
		map[string]any{"shot_ref": "shot_wheels", "start_sec": float64(6), "end_sec": float64(12), "text": "顺滑万向轮，转弯不费力。", "caption": "顺滑万向轮", "visual_focus": "万向轮特写"},
		map[string]any{"shot_ref": "shot_storage", "start_sec": float64(12), "end_sec": float64(18), "text": "打开就是分区收纳。", "caption": "分区收纳", "visual_focus": "打开收纳"},
		map[string]any{"shot_ref": "shot_scene", "start_sec": float64(18), "end_sec": float64(24), "text": "周末出行，一箱刚刚好。", "caption": "周末出行"},
		map[string]any{"shot_ref": "shot_cta", "start_sec": float64(24), "end_sec": float64(30), "text": "现在出发，悦行陪你走。", "caption": "现在出发"},
	}
	assets := []any{
		map[string]any{"role": "still", "shot_ref": "shot_hero", "shot_title": "产品主视觉", "asset_id": "asset-hero", "mime_type": "image/png", "file_name": "hero-packshot.png"},
		map[string]any{"role": "still", "shot_ref": "shot_wheels", "shot_title": "万向轮特写", "asset_id": "asset-wheels", "mime_type": "image/png", "file_name": "wheel-detail.png"},
		map[string]any{"role": "still", "shot_ref": "shot_storage", "shot_title": "打开收纳", "asset_id": "asset-storage", "mime_type": "image/png", "file_name": "open-storage.png"},
		map[string]any{"role": "still", "shot_ref": "shot_scene", "shot_title": "周末出行场景", "asset_id": "asset-scene", "mime_type": "image/png", "file_name": "scenario-packshot.png"},
		map[string]any{"role": "still", "shot_ref": "shot_cta", "shot_title": "CTA packshot", "asset_id": "asset-cta", "mime_type": "image/png", "file_name": "cta-packshot.png"},
		map[string]any{"role": "voiceover", "asset_id": "asset-voice", "mime_type": "audio/mpeg", "metadata": map[string]any{"duration_sec": float64(30)}},
	}
	staged := map[string]any{"files": []any{
		map[string]any{"asset_id": "asset-hero", "workspace_path": "/workspace/input/hero-packshot.png"},
		map[string]any{"asset_id": "asset-wheels", "workspace_path": "/workspace/input/wheel-detail.png"},
		map[string]any{"asset_id": "asset-storage", "workspace_path": "/workspace/input/open-storage.png"},
		map[string]any{"asset_id": "asset-scene", "workspace_path": "/workspace/input/scenario-packshot.png"},
		map[string]any{"asset_id": "asset-cta", "workspace_path": "/workspace/input/cta-packshot.png"},
		map[string]any{"asset_id": "asset-voice", "workspace_path": "/workspace/input/voiceover.mp3"},
	}}
	context := Context{SameTurnMessages: []ComposerSameTurnMessage{
		composerToolResultMessage(t, "get_composition_context", map[string]any{
			"audio_plan": map[string]any{
				"target_duration_sec": float64(30),
				"cue_plan":            cuePlan,
			},
			"available_composition_assets": assets,
		}),
		composerToolResultMessage(t, "stage_media_inputs", staged),
	}}

	plan, err := deterministicComposerTimelinePlan(context, "remotion_timeline_v1")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := remotiontimeline.Decode(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := remotiontimeline.Validate(decoded); err != nil {
		t.Fatal(err)
	}
	layouts := map[string]bool{}
	for _, segment := range decoded.Segments {
		layouts[segment.Layout] = true
		if segment.Caption.Source != "audio_cue" {
			t.Fatalf("caption source = %q, want audio_cue", segment.Caption.Source)
		}
	}
	if len(layouts) < 4 {
		t.Fatalf("layout diversity = %d, want at least 4 layouts: %#v", len(layouts), layouts)
	}
	captions := strings.Join(captionsForTest(decoded), " ")
	if strings.Contains(captions, "痛点钩子") || strings.Contains(captions, "前三秒抓住") {
		t.Fatalf("captions contain internal planning text: %s", captions)
	}
}

func captionsForTest(plan remotiontimeline.Plan) []string {
	out := make([]string, 0, len(plan.Segments))
	for _, segment := range plan.Segments {
		out = append(out, segment.Caption.Text)
	}
	return out
}

func composerToolResultMessage(t *testing.T, name string, value any) ComposerSameTurnMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return ComposerSameTurnMessage{Role: "tool", MessageType: "tool_result", ToolName: name, Content: string(raw)}
}

type fakeComposerArkModel struct {
	messages      []*schema.Message
	final         *schema.Message
	generateErrs  []error
	generateCalls int
}

func (f *fakeComposerArkModel) Generate(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.Message, error) {
	f.messages = messages
	if f.generateCalls < len(f.generateErrs) && f.generateErrs[f.generateCalls] != nil {
		f.generateCalls++
		return nil, f.generateErrs[f.generateCalls-1]
	}
	f.generateCalls++
	return f.final, nil
}

func (f *fakeComposerArkModel) Stream(context.Context, []*schema.Message, ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}
