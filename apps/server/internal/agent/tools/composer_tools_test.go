package tools

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/sandbox"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestComposerNativeToolsRegisterExpectedNames(t *testing.T) {
	registry, err := NewNativeRegistry(
		NewReadFileNativeTool(nil, nil),
		NewEditFileNativeTool(nil, nil),
		NewSearchAgentHistoryNativeTool(nil, contextcompact.DefaultConfig()),
		NewGetCompositionContextNativeTool(nil),
		NewStageMediaInputsNativeTool(nil),
		NewProbeMediaNativeTool(nil),
		NewCreateTimelinePlanNativeTool(nil),
		NewUpdateTimelinePlanStatusNativeTool(nil),
		NewRenderTimelineTemplateNativeTool(nil),
		NewCreateRemotionRendererAttemptNativeTool(nil, nil, nil),
		NewValidateRemotionRendererAttemptNativeTool(nil, nil, nil),
		NewRenderAgentRemotionRendererNativeTool(nil, nil),
		NewRunFFmpegCommandNativeTool(nil),
		NewSubmitCompositionArtifactNativeTool(nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := registry.ToolInfos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, info := range infos {
		if info.ParamsOneOf == nil {
			t.Fatalf("%s ParamsOneOf is nil", info.Name)
		}
		got[info.Name] = true
	}
	for _, want := range []string{
		"read_file",
		"edit_file",
		"search_agent_history",
		"get_composition_context",
		"stage_media_inputs",
		"probe_media",
		"create_timeline_plan",
		"update_timeline_plan_status",
		"render_timeline_template",
		"create_remotion_renderer_attempt",
		"validate_remotion_renderer_attempt",
		"render_agent_remotion_renderer",
		"run_ffmpeg_command",
		"submit_composition_artifact",
	} {
		if !got[want] {
			t.Fatalf("missing composer native tool %q in %#v", want, got)
		}
	}
}

func TestComposerTemplateSchemasExposeAgentRemotionCode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "create_timeline_plan", value: CreateTimelinePlanInput{}},
		{name: "render_timeline_template", value: RenderTimelineTemplateInput{}},
		{name: "dispatch_composer", value: DispatchComposerInput{}},
	} {
		field, ok := reflect.TypeOf(tc.value).FieldByName("TemplateKey")
		if !ok {
			t.Fatalf("%s missing TemplateKey field", tc.name)
		}
		if tag := field.Tag.Get("jsonschema"); !strings.Contains(tag, "enum=agent_remotion_code_v1") {
			t.Fatalf("%s TemplateKey schema missing agent_remotion_code_v1: %s", tc.name, tag)
		}
	}
	if err := validateCreateTimelinePlan(CreateTimelinePlanInput{
		TemplateKey: "agent_remotion_code_v1",
		Plan:        map[string]any{"route": "dynamic"},
	}); err != nil {
		t.Fatalf("create timeline should accept dynamic route: %v", err)
	}
	if err := validateDispatchComposerInput(DispatchComposerInput{
		SourceStoryboardRef: ToolObjectRef{Type: "media_node", Key: "storyboard.current"},
		Instructions:        "dynamic",
		TemplateKey:         "agent_remotion_code_v1",
	}); err != nil {
		t.Fatalf("dispatch should accept dynamic route: %v", err)
	}
}

func TestCreateRemotionRendererAttemptCreatesArtifactAttemptAndWorkspace(t *testing.T) {
	store := newFakeRemotionRendererStore()
	manager := fakeRemotionSandboxManager{sandboxID: "sandbox-1"}
	client := newFakeRemotionSandboxClient()
	tool := NewCreateRemotionRendererAttemptNativeTool(store, manager, client)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		TaskID:      uuidWithByte(2),
		ScopeID:     uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"attempt_no":1,
		"route_policy":{"route":"agent_remotion_code_v1","rationale":"custom visual system"},
		"summary":"dynamic product ad renderer",
		"files":{"GeneratedComposition.tsx":"import {AbsoluteFill} from 'remotion'; export function AgentGeneratedComposition(){return <AbsoluteFill />;}"},
		"props":{"output":{"width":1080,"height":1920,"fps":30,"duration_sec":6}}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("create_remotion_renderer_attempt failed: %s", got)
	}
	if store.createdArtifactParams.TimelinePlanID != uuidWithByte(4) {
		t.Fatalf("artifact timeline id = %v, want %v", store.createdArtifactParams.TimelinePlanID, uuidWithByte(4))
	}
	if store.createdArtifactParams.CreatedByRole != "composer" || store.createdArtifactParams.CreatedByTaskID != uuidWithByte(2) {
		t.Fatalf("artifact creator not recorded: %#v", store.createdArtifactParams)
	}
	if store.createdAttemptParams.AttemptNo != 1 || store.createdAttemptParams.WorkspaceDir == "" {
		t.Fatalf("attempt not created with workspace dir: %#v", store.createdAttemptParams)
	}
	if store.createdAttemptParams.Status != "draft" {
		t.Fatalf("attempt status = %q, want draft", store.createdAttemptParams.Status)
	}
	generatedPath := store.createdAttemptParams.WorkspaceDir + "/GeneratedComposition.tsx"
	if _, ok := client.uploads[generatedPath]; !ok {
		t.Fatalf("GeneratedComposition.tsx not uploaded to %s: %#v", generatedPath, client.uploads)
	}
	if !strings.Contains(got, `"renderer_artifact_id":"08000000-0000-0000-0000-000000000000"`) ||
		!strings.Contains(got, `"renderer_attempt_id":"09000000-0000-0000-0000-000000000000"`) ||
		!strings.Contains(got, `"workspace_dir":"`+store.createdAttemptParams.WorkspaceDir+`"`) {
		t.Fatalf("result missing renderer ids/workspace dir: %s", got)
	}
}

func TestValidateRemotionRendererAttemptPersistsValidatedSnapshot(t *testing.T) {
	store := newFakeRemotionRendererStore()
	store.currentAttempt = db.RemotionRendererAttempt{
		ID:                 uuidWithByte(9),
		WorkspaceID:        uuidWithByte(1),
		TimelinePlanID:     uuidWithByte(4),
		RendererArtifactID: uuidWithByte(8),
		AttemptNo:          1,
		Status:             "draft",
		WorkspaceDir:       "/workspace/agent-remotion/08000000-0000-0000-0000-000000000000/1",
	}
	manager := fakeRemotionSandboxManager{sandboxID: "sandbox-1"}
	client := newFakeRemotionSandboxClient()
	client.files[store.currentAttempt.WorkspaceDir+"/GeneratedComposition.tsx"] = "import {AbsoluteFill} from 'remotion'; export function AgentGeneratedComposition(){return <AbsoluteFill />;}"
	client.files[store.currentAttempt.WorkspaceDir+"/props.json"] = `{"output":{"width":1080,"height":1920,"fps":30,"duration_sec":6}}`
	tool := NewValidateRemotionRendererAttemptNativeTool(store, manager, client)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: uuidWithByte(1)})

	got, err := tool.InvokableRun(ctx, `{"renderer_attempt_id":"09000000-0000-0000-0000-000000000000"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("validate_remotion_renderer_attempt failed: %s", got)
	}
	if store.updatedAttemptSnapshotParams.Status != "validated" {
		t.Fatalf("status = %q, want validated", store.updatedAttemptSnapshotParams.Status)
	}
	if !strings.Contains(got, `"passed":true`) || !strings.Contains(got, `"status":"validated"`) {
		t.Fatalf("result missing passed/status: %s", got)
	}
}

func TestValidateRemotionRendererAttemptReturnsValidationFailure(t *testing.T) {
	store := newFakeRemotionRendererStore()
	store.currentAttempt = db.RemotionRendererAttempt{
		ID:                 uuidWithByte(9),
		WorkspaceID:        uuidWithByte(1),
		TimelinePlanID:     uuidWithByte(4),
		RendererArtifactID: uuidWithByte(8),
		AttemptNo:          1,
		Status:             "draft",
		WorkspaceDir:       "/workspace/agent-remotion/08000000-0000-0000-0000-000000000000/1",
	}
	manager := fakeRemotionSandboxManager{sandboxID: "sandbox-1"}
	client := newFakeRemotionSandboxClient()
	client.files[store.currentAttempt.WorkspaceDir+"/GeneratedComposition.tsx"] = "import fs from 'fs'; export function AgentGeneratedComposition(){return null;}"
	client.files[store.currentAttempt.WorkspaceDir+"/props.json"] = `{"output":{"width":1080,"height":1920,"fps":30,"duration_sec":6}}`
	tool := NewValidateRemotionRendererAttemptNativeTool(store, manager, client)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: uuidWithByte(1)})

	got, err := tool.InvokableRun(ctx, `{"renderer_attempt_id":"09000000-0000-0000-0000-000000000000"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("validation failure should be structured, got natural tool error: %s", got)
	}
	if store.updatedAttemptSnapshotParams.Status != "validation_failed" {
		t.Fatalf("status = %q, want validation_failed", store.updatedAttemptSnapshotParams.Status)
	}
	if !strings.Contains(got, `"passed":false`) || !strings.Contains(got, "forbidden_import") {
		t.Fatalf("result missing validation issue: %s", got)
	}
}

func TestRenderAgentRemotionRendererRejectsUnvalidatedAttempt(t *testing.T) {
	store := newFakeRemotionRendererStore()
	store.currentAttempt = db.RemotionRendererAttempt{
		ID:                 uuidWithByte(9),
		WorkspaceID:        uuidWithByte(1),
		TimelinePlanID:     uuidWithByte(4),
		RendererArtifactID: uuidWithByte(8),
		AttemptNo:          1,
		Status:             "draft",
		WorkspaceDir:       "/workspace/agent-remotion/08000000-0000-0000-0000-000000000000/1",
	}
	tool := NewRenderAgentRemotionRendererNativeTool(store, &fakeAgentRemotionRenderer{})
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: uuidWithByte(1), ScopeID: uuidWithByte(3)})

	got, err := tool.InvokableRun(ctx, `{"renderer_attempt_id":"09000000-0000-0000-0000-000000000000"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "必须先 validate") {
		t.Fatalf("expected validation gate error, got: %s", got)
	}
}

func TestRenderAgentRemotionRendererPersistsRenderResult(t *testing.T) {
	store := newFakeRemotionRendererStore()
	store.currentAttempt = db.RemotionRendererAttempt{
		ID:                 uuidWithByte(9),
		WorkspaceID:        uuidWithByte(1),
		TimelinePlanID:     uuidWithByte(4),
		RendererArtifactID: uuidWithByte(8),
		AttemptNo:          1,
		Status:             "validated",
		WorkspaceDir:       "/workspace/agent-remotion/08000000-0000-0000-0000-000000000000/1",
	}
	renderer := &fakeAgentRemotionRenderer{
		result: sandbox.SandboxJobResult{
			Job: db.SandboxJob{
				ID:     uuidWithByte(6),
				Output: []byte(`{"output_path":"/workspace/output/custom.mp4","renderer_engine":"remotion","video_stream":true}`),
			},
			MIME: "video/mp4",
			Size: 123,
		},
	}
	tool := NewRenderAgentRemotionRendererNativeTool(store, renderer)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: uuidWithByte(1), ScopeID: uuidWithByte(3)})

	got, err := tool.InvokableRun(ctx, `{
		"renderer_attempt_id":"09000000-0000-0000-0000-000000000000",
		"output_path":"/workspace/output/custom.mp4"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("render_agent_remotion_renderer failed: %s", got)
	}
	if renderer.input.AttemptWorkspaceDir != store.currentAttempt.WorkspaceDir || renderer.input.OutputPath != "/workspace/output/custom.mp4" {
		t.Fatalf("unexpected render input: %#v", renderer.input)
	}
	if store.updatedRenderParams.Status != "rendered" || store.updatedRenderParams.SandboxJobID != uuidWithByte(6) {
		t.Fatalf("render result not persisted: %#v", store.updatedRenderParams)
	}
	if store.setCurrentParams.CurrentAttemptID != uuidWithByte(9) || store.setCurrentParams.Status != "rendered" {
		t.Fatalf("current attempt not set: %#v", store.setCurrentParams)
	}
	if !strings.Contains(got, `"output_path":"/workspace/output/custom.mp4"`) ||
		!strings.Contains(got, `"sandbox_job_id":"06000000-0000-0000-0000-000000000000"`) ||
		!strings.Contains(got, `"result_for_timeline_plan"`) {
		t.Fatalf("result missing render metadata: %s", got)
	}
}

func TestRenderTimelineTemplateGuidesAgentRemotionToDedicatedTool(t *testing.T) {
	tool := NewRenderTimelineTemplateNativeTool(NewSandboxTimelineTemplateRenderer(&fakeCompositionSandbox{}))
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		TaskID:      uuidWithByte(2),
		ScopeType:   "final_output",
		ScopeID:     uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"template_key":"agent_remotion_code_v1",
		"plan":{"route":"dynamic"}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "render_agent_remotion_renderer") {
		t.Fatalf("expected dedicated tool guidance, got: %s", got)
	}
}

func TestRenderTimelineTemplateRunsSandboxConcat(t *testing.T) {
	sandbox := &fakeCompositionSandbox{}
	tool := NewRenderTimelineTemplateNativeTool(NewSandboxTimelineTemplateRenderer(sandbox))
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		TaskID:      uuidWithByte(2),
		ScopeType:   "final_output",
		ScopeID:     uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"template_key":"simple_concat",
		"plan":{"segments":[
			{"workspace_path":"/workspace/input/a.mp4"},
			{"workspace_path":"/workspace/input/b.mp4"}
		]}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("render_timeline_template failed: %s", got)
	}
	if sandbox.ffmpegInput.Executable != "ffmpeg" {
		t.Fatalf("executable = %q, want ffmpeg", sandbox.ffmpegInput.Executable)
	}
	args := strings.Join(sandbox.ffmpegInput.Args, " ")
	for _, want := range []string{"/workspace/input/a.mp4", "/workspace/input/b.mp4", "concat=n=2:v=1:a=1", "/workspace/output/final-04000000.mp4"} {
		if !strings.Contains(args, want) {
			t.Fatalf("ffmpeg args %q missing %q", args, want)
		}
	}
	if !strings.Contains(got, `"sandbox_job_id":"05000000-0000-0000-0000-000000000000"`) {
		t.Fatalf("result missing sandbox job id: %s", got)
	}
}

func TestRenderTimelineTemplateBuildsAudioMixCommand(t *testing.T) {
	sandbox := &fakeCompositionSandbox{}
	tool := NewRenderTimelineTemplateNativeTool(NewSandboxTimelineTemplateRenderer(sandbox))
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		TaskID:      uuidWithByte(2),
		ScopeType:   "final_output",
		ScopeID:     uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"template_key":"concat_with_fades",
		"plan":{
			"segments":[
				{"id":"shot-01","workspace_path":"/workspace/input/a.mp4","duration_sec":4.2,"caption":"轻商务出行"},
				{"id":"shot-02","workspace_path":"/workspace/input/b.mp4","duration_sec":4.0,"caption":"顺滑万向轮"}
			],
			"audio_tracks":[
				{"id":"voiceover-main","role":"voiceover","workspace_path":"/workspace/input/voiceover.mp3","start_sec":0,"duration_sec":8.2,"volume":1,"fade_in_sec":0.05,"fade_out_sec":0.1},
				{"id":"bgm-main","role":"bgm","workspace_path":"/workspace/input/bgm.mp3","start_sec":0,"duration_sec":8.2,"volume":0.28,"fade_in_sec":0.5,"fade_out_sec":1.2,"ducking":{"sidechain_role":"voiceover","threshold":0.08,"ratio":8,"attack_ms":20,"release_ms":250}}
			],
			"output":{"workspace_path":"/workspace/output/final-audio.mp4","format":"mp4","audio_codec":"aac"}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("render_timeline_template failed: %s", got)
	}
	args := strings.Join(sandbox.ffmpegInput.Args, " ")
	for _, want := range []string{
		"/workspace/input/a.mp4",
		"/workspace/input/b.mp4",
		"/workspace/input/voiceover.mp3",
		"/workspace/input/bgm.mp3",
		"concat=n=2:v=1:a=0",
		"scale=1080:1920:force_original_aspect_ratio=decrease",
		"pad=1080:1920:(ow-iw)/2:(oh-ih)/2",
		"drawtext=",
		"fontfile=/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
		"轻商务出行",
		"顺滑万向轮",
		"between(t\\,0.000\\,4.200)",
		"atrim",
		"volume=1.000",
		"volume=0.280",
		"afade=t=in:st=0:d=0.500",
		"apad=whole_dur=8.200",
		"asplit=2",
		"sidechaincompress",
		"amix=inputs=2:duration=longest",
		"-map [vout]",
		"-map [aout]",
		"-c:a aac",
		"-shortest",
		"/workspace/output/final-audio.mp4",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("ffmpeg args %q missing %q", args, want)
		}
	}
}

func TestTimelineCaptionFilterSplitsLongChineseCaptions(t *testing.T) {
	longCaption := "底部万向轮顺滑转向，转弯不抢手，狭窄通道也能轻松掉头，赶车换乘更省力。"
	segments := []timelineSegmentInput{
		{WorkspacePath: "/workspace/input/a.mp4", DurationSec: 8, Caption: longCaption},
	}

	parts := strings.Join(timelineVideoFilterParts(segments, "vout"), ";")

	if strings.Contains(parts, "text='"+escapeDrawText(longCaption)+"'") {
		t.Fatalf("long caption should be split before drawtext: %s", parts)
	}
	for _, want := range []string{
		"底部万向轮顺滑转向，转弯不抢手，",
		"狭窄通道也能轻松掉头，",
		"赶车换乘更省力。",
		"y=h-320",
		"fontsize=50",
	} {
		if !strings.Contains(parts, want) {
			t.Fatalf("caption filter %q missing %q", parts, want)
		}
	}
	if got := strings.Count(parts, "drawtext="); got < 2 {
		t.Fatalf("expected multiple caption drawtext filters, got %d in %s", got, parts)
	}
}

func TestRenderTimelineTemplateLoopsStillSegments(t *testing.T) {
	sandbox := &fakeCompositionSandbox{}
	tool := NewRenderTimelineTemplateNativeTool(NewSandboxTimelineTemplateRenderer(sandbox))
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		TaskID:      uuidWithByte(2),
		ScopeType:   "final_output",
		ScopeID:     uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"template_key":"concat_with_fades",
		"plan":{"segments":[
			{"id":"shot-01","workspace_path":"/workspace/input/a.mp4"},
			{"id":"shot-cta","role":"still","mime_type":"image/png","workspace_path":"/workspace/input/cta.png","duration_sec":5}
		],"output":{"workspace_path":"/workspace/output/final-still.mp4","format":"mp4"}}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("render_timeline_template failed: %s", got)
	}
	args := strings.Join(sandbox.ffmpegInput.Args, " ")
	for _, want := range []string{
		"-loop 1 -t 5.000 -i /workspace/input/cta.png",
		"scale=1080:1920:force_original_aspect_ratio=decrease",
		"pad=1080:1920:(ow-iw)/2:(oh-ih)/2",
		"concat=n=2:v=1:a=0",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("ffmpeg args %q missing %q", args, want)
		}
	}
}

func TestRenderTimelineTemplateRoutesRemotionTimeline(t *testing.T) {
	remotion := &fakeRemotionTimelineRenderer{}
	ffmpegSandbox := &fakeCompositionSandbox{}
	tool := NewRenderTimelineTemplateNativeTool(NewSandboxTimelineTemplateRenderer(ffmpegSandbox).WithRemotionRenderer(remotion))
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		TaskID:      uuidWithByte(2),
		ScopeType:   "final_output",
		ScopeID:     uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"template_key":"remotion_timeline_v1",
		"plan":{
			"schema":"clipanvil.remotion_timeline.v1",
			"composition":"MarketingTimeline",
			"output":{"width":1080,"height":1920,"fps":30,"duration_sec":10,"codec":"h264","audio_codec":"aac"},
			"segments":[{"id":"seg-1","shot_ref":"shot_01","start_sec":0,"end_sec":10,"layout":"hero_packshot","assets":[{"role":"primary","type":"image","workspace_path":"/workspace/input/product.png"}],"caption":{"source":"audio_cue","text":"轻松出发","start_sec":0,"end_sec":10}}],
			"audio_tracks":[{"id":"voiceover","role":"voiceover","workspace_path":"/workspace/input/voiceover.mp3","volume":1}]
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("render_timeline_template failed: %s", got)
	}
	if remotion.input.OutputPath != "/workspace/output/final-04000000.mp4" {
		t.Fatalf("remotion output path = %q", remotion.input.OutputPath)
	}
	if remotion.input.Plan.Schema != "clipanvil.remotion_timeline.v1" || len(remotion.input.Plan.Segments) != 1 {
		t.Fatalf("remotion input = %#v", remotion.input)
	}
	if len(ffmpegSandbox.ffmpegInput.Args) != 0 {
		t.Fatalf("remotion route should not call ffmpeg path, got %#v", ffmpegSandbox.ffmpegInput.Args)
	}
	if !strings.Contains(got, `"sandbox_job_id":"06000000-0000-0000-0000-000000000000"`) {
		t.Fatalf("result missing remotion sandbox job id: %s", got)
	}
}

func TestRenderTimelineTemplateRejectsInvalidRemotionTimeline(t *testing.T) {
	tool := NewRenderTimelineTemplateNativeTool(NewSandboxTimelineTemplateRenderer(&fakeCompositionSandbox{}).WithRemotionRenderer(&fakeRemotionTimelineRenderer{}))
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		TaskID:      uuidWithByte(2),
		ScopeType:   "final_output",
		ScopeID:     uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"template_key":"remotion_timeline_v1",
		"plan":{"segments":[]}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "工具调用失败") {
		t.Fatalf("expected invalid plan failure, got %s", got)
	}
}

func TestSubmitCompositionArtifactCreatesNodeUploadsAndPersists(t *testing.T) {
	store := &fakeCompositionArtifactStore{}
	uploader := &fakeCompositionOutputUploader{
		result: sandbox.SandboxJobResult{
			Job:   db.SandboxJob{ID: uuidWithByte(8)},
			Asset: sandbox.ArtifactObject{StorageURL: "workspace-01000000-0000-0000-0000-000000000000/production/final.mp4"},
			MIME:  "video/mp4",
			Size:  1234,
		},
	}
	persister := &fakeCompositionArtifactPersister{}
	tool := NewSubmitCompositionArtifactNativeTool(persister, store).WithOutputUploader(uploader)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		TaskID:      uuidWithByte(2),
		ScopeType:   "final_output",
		ScopeID:     uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"sandbox_job_id":"05000000-0000-0000-0000-000000000000",
		"output_path":"/workspace/output/final.mp4",
		"result":{"duration":12.3}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("submit_composition_artifact failed: %s", got)
	}
	if !store.createdNode.ID.Valid {
		t.Fatal("expected output node to be created")
	}
	if uploader.input.TargetNodeID != store.createdNode.ID {
		t.Fatalf("uploader target node = %v, want created node %v", uploader.input.TargetNodeID, store.createdNode.ID)
	}
	if persister.input.OutputNodeID != store.createdNode.ID {
		t.Fatalf("persister output node = %v, want created node %v", persister.input.OutputNodeID, store.createdNode.ID)
	}
	if persister.input.ProviderResult.AssetStorageURL != uploader.result.Asset.StorageURL {
		t.Fatalf("persisted storage url = %q, want %q", persister.input.ProviderResult.AssetStorageURL, uploader.result.Asset.StorageURL)
	}
	if store.updatedTimeline.OutputNodeID != store.createdNode.ID || store.updatedTimeline.ArtifactVersionID != persister.result.Version.ID {
		t.Fatalf("timeline was not linked to output node/artifact: %#v", store.updatedTimeline)
	}
	if store.updatedAudioPlanTimeline.TimelinePlanID != uuidWithByte(4) || store.updatedAudioPlanTimeline.WorkspaceID != uuidWithByte(1) {
		t.Fatalf("audio plan timeline was not linked: %#v", store.updatedAudioPlanTimeline)
	}
}

func TestUpdateTimelinePlanStatusMergesExistingResult(t *testing.T) {
	store := &fakeCompositionTimelineStore{
		plan: db.TimelinePlan{
			ID:     uuidWithByte(4),
			Status: "completed",
			Result: []byte(`{"output_path":"/workspace/output/final.mp4","storage_url":"workspace-1/final.mp4","duration":10}`),
		},
	}
	tool := NewUpdateTimelinePlanStatusNativeTool(store)
	got, err := tool.InvokableRun(context.Background(), `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"status":"completed",
		"result":{"segments_count":2}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("update_timeline_plan_status failed: %s", got)
	}
	var merged map[string]any
	if err := json.Unmarshal(store.updated.Result, &merged); err != nil {
		t.Fatal(err)
	}
	if merged["output_path"] != "/workspace/output/final.mp4" || merged["storage_url"] != "workspace-1/final.mp4" || merged["segments_count"].(float64) != 2 {
		t.Fatalf("merged result = %#v", merged)
	}
}

func TestProbeMediaRejectsInputDirectory(t *testing.T) {
	tool := NewProbeMediaNativeTool(nil)
	out, err := tool.InvokableRun(context.Background(), `{"workspace_path":"/workspace/input"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"工具调用失败", "workspace_path", "具体文件路径", "stage_media_inputs"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestRunFFmpegCommandRejectsUnstagedRootInputPath(t *testing.T) {
	tool := NewRunFFmpegCommandNativeTool(nil)
	out, err := tool.InvokableRun(context.Background(), `{
		"executable":"ffmpeg",
		"args":["-i","/workspace/shot_02.mp4","-c:v","libx264","/workspace/output/final.mp4"]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"工具调用失败", "/workspace/input", "stage_media_inputs"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

type fakeCompositionSandbox struct {
	ffmpegInput sandbox.RunFFmpegCommandInput
}

func (f *fakeCompositionSandbox) StageMediaInputs(context.Context, sandbox.StageMediaInputsInput) (sandbox.StageMediaInputsResult, error) {
	return sandbox.StageMediaInputsResult{}, nil
}

func (f *fakeCompositionSandbox) ProbeMedia(context.Context, sandbox.ProbeMediaInput) (sandbox.ProbeMediaResult, error) {
	return sandbox.ProbeMediaResult{}, nil
}

func (f *fakeCompositionSandbox) RunFFmpegCommand(_ context.Context, input sandbox.RunFFmpegCommandInput) (sandbox.SandboxJobResult, error) {
	f.ffmpegInput = input
	return sandbox.SandboxJobResult{Job: db.SandboxJob{ID: uuidWithByte(5)}}, nil
}

type fakeRemotionTimelineRenderer struct {
	input sandbox.RenderRemotionTimelineInput
}

func (f *fakeRemotionTimelineRenderer) RenderRemotionTimeline(_ context.Context, input sandbox.RenderRemotionTimelineInput) (sandbox.SandboxJobResult, error) {
	f.input = input
	return sandbox.SandboxJobResult{Job: db.SandboxJob{ID: uuidWithByte(6)}, MIME: "video/mp4", Size: 123}, nil
}

type fakeCompositionArtifactStore struct {
	createdNode              db.MediaNode
	updatedTimeline          db.UpdateTimelinePlanStatusParams
	updatedAudioPlanTimeline db.UpdateAudioPlanTimelinePlanParams
}

func (f *fakeCompositionArtifactStore) CreateTimelinePlan(context.Context, db.CreateTimelinePlanParams) (db.TimelinePlan, error) {
	return db.TimelinePlan{}, nil
}

func (f *fakeCompositionArtifactStore) CreateAgentGenerationNode(_ context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error) {
	f.createdNode = db.MediaNode{
		ID:            uuidWithByte(7),
		WorkspaceID:   params.WorkspaceID,
		NodeType:      params.NodeType,
		OperationType: params.OperationType,
		SemanticKey:   params.SemanticKey,
		DisplayName:   params.DisplayName,
		ArtifactKind:  params.ArtifactKind,
	}
	return f.createdNode, nil
}

func (f *fakeCompositionArtifactStore) GetTimelinePlan(context.Context, pgtype.UUID) (db.TimelinePlan, error) {
	return db.TimelinePlan{ID: uuidWithByte(4), WorkspaceID: uuidWithByte(1), Status: "completed"}, nil
}

func (f *fakeCompositionArtifactStore) UpdateTimelinePlanStatus(_ context.Context, params db.UpdateTimelinePlanStatusParams) (db.TimelinePlan, error) {
	f.updatedTimeline = params
	return db.TimelinePlan{
		ID:                params.ID,
		WorkspaceID:       uuidWithByte(1),
		Status:            params.Status,
		OutputNodeID:      params.OutputNodeID,
		ProductionJobID:   params.ProductionJobID,
		ArtifactVersionID: params.ArtifactVersionID,
		SandboxJobID:      params.SandboxJobID,
		Result:            params.Result,
	}, nil
}

func (f *fakeCompositionArtifactStore) UpdateAudioPlanTimelinePlan(_ context.Context, params db.UpdateAudioPlanTimelinePlanParams) (db.AudioPlan, error) {
	f.updatedAudioPlanTimeline = params
	return db.AudioPlan{WorkspaceID: params.WorkspaceID, TimelinePlanID: params.TimelinePlanID}, nil
}

type fakeCompositionOutputUploader struct {
	input  sandbox.UploadCompositionOutputInput
	result sandbox.SandboxJobResult
}

type fakeRemotionSandboxManager struct {
	sandboxID string
}

func (f fakeRemotionSandboxManager) EnsureSandbox(context.Context, pgtype.UUID) (sandbox.WorkspaceSandbox, error) {
	return sandbox.WorkspaceSandbox{SandboxID: f.sandboxID, VolumeName: "volume-1"}, nil
}

type fakeRemotionSandboxClient struct {
	uploads map[string]string
	files   map[string]string
}

func newFakeRemotionSandboxClient() *fakeRemotionSandboxClient {
	return &fakeRemotionSandboxClient{
		uploads: map[string]string{},
		files:   map[string]string{},
	}
}

func (f *fakeRemotionSandboxClient) Create(context.Context, sandbox.CreateRequest) (sandbox.SandboxInfo, error) {
	return sandbox.SandboxInfo{}, nil
}

func (f *fakeRemotionSandboxClient) Get(context.Context, string) (sandbox.SandboxInfo, error) {
	return sandbox.SandboxInfo{}, nil
}

func (f *fakeRemotionSandboxClient) Ping(context.Context, string) error { return nil }

func (f *fakeRemotionSandboxClient) Exec(_ context.Context, _ string, req sandbox.ExecRequest) (sandbox.ExecResult, error) {
	if strings.HasPrefix(req.Command, "find ") {
		paths := make([]string, 0, len(f.files))
		for path := range f.files {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		return sandbox.ExecResult{Stdout: strings.Join(paths, "\n"), ExitCode: 0}, nil
	}
	return sandbox.ExecResult{}, nil
}

func (f *fakeRemotionSandboxClient) Upload(_ context.Context, _ string, path string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.uploads[path] = string(data)
	f.files[path] = string(data)
	return nil
}

func (f *fakeRemotionSandboxClient) Download(_ context.Context, _ string, path string) (io.ReadCloser, sandbox.FileInfo, error) {
	content, ok := f.files[path]
	if !ok {
		return nil, sandbox.FileInfo{}, io.ErrUnexpectedEOF
	}
	return io.NopCloser(strings.NewReader(content)), sandbox.FileInfo{Path: path, SizeBytes: int64(len(content)), Mime: "text/plain"}, nil
}

func (f *fakeRemotionSandboxClient) Delete(context.Context, string) error { return nil }

type fakeRemotionRendererStore struct {
	timelinePlan                 db.TimelinePlan
	createdArtifactParams        db.CreateRemotionRendererArtifactParams
	createdAttemptParams         db.CreateRemotionRendererAttemptParams
	updatedAttemptSnapshotParams db.UpdateRemotionRendererAttemptSnapshotParams
	updatedRenderParams          db.UpdateRemotionRendererAttemptRenderResultParams
	setCurrentParams             db.SetCurrentRemotionRendererAttemptParams
	currentAttempt               db.RemotionRendererAttempt
}

func newFakeRemotionRendererStore() *fakeRemotionRendererStore {
	return &fakeRemotionRendererStore{
		timelinePlan: db.TimelinePlan{
			ID:          uuidWithByte(4),
			WorkspaceID: uuidWithByte(1),
			Status:      "draft",
			TemplateKey: "agent_remotion_code_v1",
		},
	}
}

func (f *fakeRemotionRendererStore) CreateTimelinePlan(context.Context, db.CreateTimelinePlanParams) (db.TimelinePlan, error) {
	return db.TimelinePlan{}, nil
}

func (f *fakeRemotionRendererStore) GetTimelinePlan(context.Context, pgtype.UUID) (db.TimelinePlan, error) {
	return f.timelinePlan, nil
}

func (f *fakeRemotionRendererStore) UpdateTimelinePlanStatus(context.Context, db.UpdateTimelinePlanStatusParams) (db.TimelinePlan, error) {
	return db.TimelinePlan{}, nil
}

func (f *fakeRemotionRendererStore) CreateRemotionRendererArtifact(_ context.Context, params db.CreateRemotionRendererArtifactParams) (db.RemotionRendererArtifact, error) {
	f.createdArtifactParams = params
	return db.RemotionRendererArtifact{
		ID:              uuidWithByte(8),
		WorkspaceID:     params.WorkspaceID,
		TimelinePlanID:  params.TimelinePlanID,
		Status:          params.Status,
		RoutePolicy:     params.RoutePolicy,
		Summary:         params.Summary,
		CreatedByRole:   params.CreatedByRole,
		CreatedByTaskID: params.CreatedByTaskID,
	}, nil
}

func (f *fakeRemotionRendererStore) GetRemotionRendererArtifact(context.Context, pgtype.UUID) (db.RemotionRendererArtifact, error) {
	return db.RemotionRendererArtifact{ID: uuidWithByte(8), WorkspaceID: uuidWithByte(1), TimelinePlanID: uuidWithByte(4), Status: "draft"}, nil
}

func (f *fakeRemotionRendererStore) CreateRemotionRendererAttempt(_ context.Context, params db.CreateRemotionRendererAttemptParams) (db.RemotionRendererAttempt, error) {
	f.createdAttemptParams = params
	return db.RemotionRendererAttempt{
		ID:                 uuidWithByte(9),
		WorkspaceID:        params.WorkspaceID,
		TimelinePlanID:     params.TimelinePlanID,
		RendererArtifactID: params.RendererArtifactID,
		AttemptNo:          params.AttemptNo,
		Status:             params.Status,
		SourceSnapshot:     params.SourceSnapshot,
		PropsJson:          params.PropsJson,
		SourceHash:         params.SourceHash,
		PropsHash:          params.PropsHash,
		WorkspaceDir:       params.WorkspaceDir,
		ValidationResult:   params.ValidationResult,
		CompileResult:      params.CompileResult,
		RenderResult:       params.RenderResult,
		QaResult:           params.QaResult,
	}, nil
}

func (f *fakeRemotionRendererStore) GetRemotionRendererAttempt(context.Context, pgtype.UUID) (db.RemotionRendererAttempt, error) {
	return f.currentAttempt, nil
}

func (f *fakeRemotionRendererStore) UpdateRemotionRendererAttemptSnapshot(_ context.Context, params db.UpdateRemotionRendererAttemptSnapshotParams) (db.RemotionRendererAttempt, error) {
	f.updatedAttemptSnapshotParams = params
	f.currentAttempt.Status = params.Status
	f.currentAttempt.SourceSnapshot = params.SourceSnapshot
	f.currentAttempt.PropsJson = params.PropsJson
	f.currentAttempt.SourceHash = params.SourceHash
	f.currentAttempt.PropsHash = params.PropsHash
	f.currentAttempt.WorkspaceDir = params.WorkspaceDir
	f.currentAttempt.ValidationResult = params.ValidationResult
	f.currentAttempt.CompileResult = params.CompileResult
	return f.currentAttempt, nil
}

func (f *fakeRemotionRendererStore) UpdateRemotionRendererAttemptRenderResult(_ context.Context, params db.UpdateRemotionRendererAttemptRenderResultParams) (db.RemotionRendererAttempt, error) {
	f.updatedRenderParams = params
	f.currentAttempt.Status = params.Status
	f.currentAttempt.RenderResult = params.RenderResult
	f.currentAttempt.SandboxJobID = params.SandboxJobID
	return f.currentAttempt, nil
}

func (f *fakeRemotionRendererStore) SetCurrentRemotionRendererAttempt(_ context.Context, params db.SetCurrentRemotionRendererAttemptParams) (db.RemotionRendererArtifact, error) {
	f.setCurrentParams = params
	return db.RemotionRendererArtifact{
		ID:               params.ID,
		WorkspaceID:      f.currentAttempt.WorkspaceID,
		TimelinePlanID:   f.currentAttempt.TimelinePlanID,
		CurrentAttemptID: params.CurrentAttemptID,
		Status:           params.Status,
	}, nil
}

type fakeAgentRemotionRenderer struct {
	input  sandbox.RenderAgentRemotionCodeInput
	result sandbox.SandboxJobResult
}

func (f *fakeAgentRemotionRenderer) RenderAgentRemotionCode(_ context.Context, input sandbox.RenderAgentRemotionCodeInput) (sandbox.SandboxJobResult, error) {
	f.input = input
	return f.result, nil
}

type fakeCompositionTimelineStore struct {
	plan    db.TimelinePlan
	updated db.UpdateTimelinePlanStatusParams
}

func (f *fakeCompositionTimelineStore) CreateTimelinePlan(context.Context, db.CreateTimelinePlanParams) (db.TimelinePlan, error) {
	return db.TimelinePlan{}, nil
}

func (f *fakeCompositionTimelineStore) GetTimelinePlan(context.Context, pgtype.UUID) (db.TimelinePlan, error) {
	return f.plan, nil
}

func (f *fakeCompositionTimelineStore) UpdateTimelinePlanStatus(_ context.Context, params db.UpdateTimelinePlanStatusParams) (db.TimelinePlan, error) {
	f.updated = params
	return db.TimelinePlan{ID: params.ID, Status: params.Status, Result: params.Result}, nil
}

func (f *fakeCompositionOutputUploader) UploadCompositionOutput(_ context.Context, input sandbox.UploadCompositionOutputInput) (sandbox.SandboxJobResult, error) {
	f.input = input
	return f.result, nil
}

type fakeCompositionArtifactPersister struct {
	input  production.ComposerArtifactInput
	result production.RunResult
}

func (f *fakeCompositionArtifactPersister) PersistComposerArtifact(_ context.Context, input production.ComposerArtifactInput) (production.RunResult, error) {
	f.input = input
	f.result = production.RunResult{
		Node:    db.MediaNode{ID: input.OutputNodeID, WorkspaceID: input.WorkspaceID},
		Job:     db.GenerationJob{ID: uuidWithByte(9), WorkspaceID: input.WorkspaceID},
		Version: db.ArtifactVersion{ID: uuidWithByte(10), WorkspaceID: input.WorkspaceID, NodeID: input.OutputNodeID},
	}
	return f.result, nil
}
