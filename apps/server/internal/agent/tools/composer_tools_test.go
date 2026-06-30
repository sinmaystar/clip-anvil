package tools

import (
	"context"
	"encoding/json"
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
		"run_ffmpeg_command",
		"submit_composition_artifact",
	} {
		if !got[want] {
			t.Fatalf("missing composer native tool %q in %#v", want, got)
		}
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
				{"id":"shot-01","workspace_path":"/workspace/input/a.mp4","duration_sec":4.2},
				{"id":"shot-02","workspace_path":"/workspace/input/b.mp4","duration_sec":4.0}
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
		"atrim",
		"volume=1.000",
		"volume=0.280",
		"afade=t=in:st=0:d=0.500",
		"asplit=2",
		"sidechaincompress",
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
