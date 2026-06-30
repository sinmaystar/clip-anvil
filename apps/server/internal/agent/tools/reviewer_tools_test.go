package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestReviewerNativeToolInfosUseChineseDescriptions(t *testing.T) {
	tools := []NativeTool{
		NewSubmitReviewResultNativeTool(nil),
	}
	for _, item := range tools {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "" || !strings.Contains(info.Desc, "Reviewer") {
			t.Fatalf("bad tool info: %#v", info)
		}
		if info.ParamsOneOf == nil {
			t.Fatalf("%s ParamsOneOf is nil", info.Name)
		}
	}
}

func TestSubmitReviewResultReturnsNaturalValidationError(t *testing.T) {
	tool := NewSubmitReviewResultNativeTool(nil)
	out, err := tool.InvokableRun(context.Background(), `{"review_task":"shot_video_review"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"工具调用失败", "submit_review_result", "重试建议"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestSubmitReviewResultUsesReviewerTaskRuntimeTarget(t *testing.T) {
	workspaceID := uuidWithByte(1)
	threadID := uuidWithByte(2)
	taskID := uuidWithByte(3)
	shotID := uuidWithByte(4)
	nodeID := uuidWithByte(5)
	versionID := uuidWithByte(6)
	jobID := uuidWithByte(7)
	planID := uuidWithByte(8)
	store := &fakeSubmitReviewResultStore{}
	tool := NewSubmitReviewResultNativeTool(store)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID:        workspaceID,
		ThreadID:           threadID,
		TaskID:             taskID,
		ReviewTask:         reviewTaskPreviewImage,
		ReviewShotID:       uuidString(shotID),
		ReviewNodeID:       uuidString(nodeID),
		ReviewVersionID:    uuidString(versionID),
		ReviewJobID:        uuidString(jobID),
		ReviewRenderPlanID: uuidString(planID),
	})
	out, err := tool.InvokableRun(ctx, `{
		"brief":"提交 shot_01 预览图评审",
		"review_task":"preview_image_review",
		"target":{
			"workspace_scope":"shot",
			"shot_id":"04000000-0000-0000-0000-000000000000",
			"node_id":"05000000-0000-0000-0000-000000000000",
			"render_plan_id":"05000000-0000-0000-0000-000000000000",
			"generation_job_id":"05000000-0000-0000-0000-000000000000",
			"artifact_version_id":"05000000-0000-0000-0000-000000000000"
		},
		"verdict":"accepted",
		"overall_score":0.92,
		"rubric":[
			{"axis":"faithfulness","score":0.9,"pass":true,"severity":"info","reason":"符合分镜目标"},
			{"axis":"subject_consistency","score":0.9,"pass":true,"severity":"info","reason":"商品一致"},
			{"axis":"product_visibility","score":0.9,"pass":true,"severity":"info","reason":"商品清晰"},
			{"axis":"brand_style_consistency","score":0.9,"pass":true,"severity":"info","reason":"风格统一"},
			{"axis":"composition_proportion","score":0.9,"pass":true,"severity":"info","reason":"构图合理"},
			{"axis":"visual_quality","score":0.9,"pass":true,"severity":"info","reason":"画质清晰"}
		],
		"critique":"预览图可用。",
		"reason":"required axes 均通过。",
		"retry_recommendation":{"should_repair":false,"suggested_fix":"none"}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "已提交 Reviewer 评审结果") {
		t.Fatalf("unexpected output: %s", out)
	}
	if strings.Contains(out, uuidString(uuidWithByte(20))) {
		t.Fatalf("output leaked review record UUID: %s", out)
	}
	if !strings.Contains(out, "review_record/shot_01.preview_image.r1.review.v1") {
		t.Fatalf("output missing review semantic ref: %s", out)
	}
	if store.create.ArtifactVersionID != versionID {
		t.Fatalf("artifact_version_id = %s, want %s", uuidString(store.create.ArtifactVersionID), uuidString(versionID))
	}
	if store.create.NodeID != nodeID || store.create.GenerationJobID != jobID || store.create.RenderPlanID != planID {
		t.Fatalf("target ids were not canonicalized: %#v", store.create)
	}
}

func TestSubmitReviewResultDerivesFinalVideoIssueTarget(t *testing.T) {
	workspaceID := uuidWithByte(1)
	threadID := uuidWithByte(2)
	taskID := uuidWithByte(3)
	nodeID := uuidWithByte(5)
	versionID := uuidWithByte(6)
	store := &fakeSubmitReviewResultStore{}
	tool := NewSubmitReviewResultNativeTool(store)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID:      workspaceID,
		ThreadID:         threadID,
		TaskID:           taskID,
		ReviewTask:       reviewTaskFinalVideo,
		ReviewNodeID:     uuidString(nodeID),
		ReviewNodeKey:    "composer.final_output.f86a42ed.node",
		ReviewVersionID:  uuidString(versionID),
		ReviewVersionKey: "composer.final_output.f86a42ed.compose.artifact.v1",
	})
	out, err := tool.InvokableRun(ctx, `{
		"brief":"提交 final video 评审",
		"review_task":"final_video_review",
		"target":{"workspace_scope":"final_video"},
		"verdict":"accepted",
		"overall_score":0.93,
		"rubric":[
			{"axis":"faithfulness","score":0.95,"pass":true,"severity":"info","reason":"符合 brief"},
			{"axis":"brand_style_consistency","score":0.92,"pass":true,"severity":"info","reason":"风格一致"},
			{"axis":"visual_quality","score":0.91,"pass":true,"severity":"info","reason":"画质可用"},
			{"axis":"continuity","score":0.9,"pass":true,"severity":"info","reason":"叙事连贯"},
			{"axis":"audio_sync","score":0.94,"pass":true,"severity":"info","reason":"旁白和画面同步"},
			{"axis":"platform_selling_power","score":0.93,"pass":true,"severity":"info","reason":"带货表达清晰"}
		],
		"critique":"成片可用。",
		"issues":[{
			"dimension":"audio_sync",
			"severity":"info",
			"title":"可选混音优化",
			"description":"当前可用，可后续优化 ducking。",
			"target_object_type":"final_video",
			"suggested_fix":"edit",
			"fix_hint":"如需精修，可对 BGM 做轻量 ducking。"
		}],
		"retry_recommendation":{"should_repair":false,"suggested_fix":"none","target_object_type":"final_video"},
		"reason":"required axes 均通过。"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "已提交 Reviewer 评审结果") {
		t.Fatalf("unexpected output: %s", out)
	}
	if len(store.issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(store.issues))
	}
	if store.issues[0].TargetObjectType != "final_video" || store.issues[0].TargetObjectID != nodeID {
		t.Fatalf("final video issue target = %s/%s, want final_video/%s", store.issues[0].TargetObjectType, uuidString(store.issues[0].TargetObjectID), uuidString(nodeID))
	}
}

func TestSubmitReviewResultDefaultsBriefFromRuntimeReviewTask(t *testing.T) {
	workspaceID := uuidWithByte(1)
	threadID := uuidWithByte(2)
	taskID := uuidWithByte(3)
	nodeID := uuidWithByte(5)
	versionID := uuidWithByte(6)
	store := &fakeSubmitReviewResultStore{}
	tool := NewSubmitReviewResultNativeTool(store)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID:      workspaceID,
		ThreadID:         threadID,
		TaskID:           taskID,
		ReviewTask:       reviewTaskFinalVideo,
		ReviewNodeID:     uuidString(nodeID),
		ReviewNodeKey:    "composer.final_output.f86a42ed.node",
		ReviewVersionID:  uuidString(versionID),
		ReviewVersionKey: "composer.final_output.f86a42ed.compose.artifact.v1",
	})
	out, err := tool.InvokableRun(ctx, `{
		"review_task":"final_video_review",
		"target":{"workspace_scope":"final_video"},
		"verdict":"accepted",
		"overall_score":0.93,
		"rubric":[
			{"axis":"faithfulness","score":0.95,"pass":true,"severity":"info","reason":"符合 brief"},
			{"axis":"brand_style_consistency","score":0.92,"pass":true,"severity":"info","reason":"风格一致"},
			{"axis":"visual_quality","score":0.91,"pass":true,"severity":"info","reason":"画质可用"},
			{"axis":"continuity","score":0.9,"pass":true,"severity":"info","reason":"叙事连贯"},
			{"axis":"audio_sync","score":0.94,"pass":true,"severity":"info","reason":"旁白和画面同步"},
			{"axis":"platform_selling_power","score":0.93,"pass":true,"severity":"info","reason":"带货表达清晰"}
		],
		"critique":"成片可用。",
		"retry_recommendation":{"should_repair":false,"suggested_fix":"none","target_object_type":"final_video"},
		"reason":"required axes 均通过。"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "已提交 Reviewer 评审结果") {
		t.Fatalf("unexpected output: %s", out)
	}
	if store.create.SemanticKey != "composer.final_output.f86a42ed.compose.review.v1" {
		t.Fatalf("review semantic key = %q", store.create.SemanticKey)
	}
	if store.create.DisplayName != "Review composer.final_output.f86a42ed.compose.review.v1" {
		t.Fatalf("review display name = %q", store.create.DisplayName)
	}
}

func TestSubmitReviewResultReportsAllMissingRequiredAxes(t *testing.T) {
	tool := NewSubmitReviewResultNativeTool(&fakeSubmitReviewResultStore{})
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		ReviewTask:       reviewTaskFinalVideo,
		ReviewNodeID:     uuidString(uuidWithByte(5)),
		ReviewNodeKey:    "composer.final_output.f86a42ed.node",
		ReviewVersionID:  uuidString(uuidWithByte(6)),
		ReviewVersionKey: "composer.final_output.f86a42ed.compose.artifact.v1",
	})
	out, err := tool.InvokableRun(ctx, `{
		"brief":"提交 final video 音频专项评审",
		"review_task":"final_video_review",
		"target":{"workspace_scope":"final_video"},
		"verdict":"rejected",
		"overall_score":0.62,
		"rubric":[
			{"axis":"faithfulness","score":0.55,"pass":false,"severity":"blocking","reason":"开场动态钩子缺失"},
			{"axis":"audio_sync","score":0.58,"pass":false,"severity":"blocking","reason":"首尾音画不匹配"},
			{"axis":"platform_selling_power","score":0.61,"pass":false,"severity":"blocking","reason":"CTA 转化弱"}
		],
		"critique":"首尾关键节点不可用。",
		"issues":[{
			"dimension":"audio_sync",
			"severity":"blocking",
			"title":"首尾音画不匹配",
			"description":"旁白与静态开场和缺失 CTA 不匹配。",
			"target_object_type":"final_video",
			"suggested_fix":"regenerate",
			"fix_hint":"重做开场动态镜头并补 CTA overlay。"
		}],
		"retry_recommendation":{"should_repair":true,"suggested_fix":"regenerate","target_object_type":"final_video"},
		"reason":"存在 blocking issue。"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"缺少 required axes",
		"brand_style_consistency",
		"visual_quality",
		"continuity",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

type fakeSubmitReviewResultStore struct {
	create   db.CreateReviewRecordParams
	complete db.CompleteReviewRecordParams
	issues   []db.CreateArtifactIssueParams
}

func (f *fakeSubmitReviewResultStore) CreateReviewRecord(_ context.Context, params db.CreateReviewRecordParams) (db.ReviewRecord, error) {
	f.create = params
	return db.ReviewRecord{ID: uuidWithByte(20), WorkspaceID: params.WorkspaceID}, nil
}

func (f *fakeSubmitReviewResultStore) CompleteReviewRecord(_ context.Context, params db.CompleteReviewRecordParams) (db.ReviewRecord, error) {
	f.complete = params
	return db.ReviewRecord{ID: params.ID, SemanticKey: "shot_01.preview_image.r1.review.v1"}, nil
}

func (f *fakeSubmitReviewResultStore) CreateArtifactIssue(_ context.Context, params db.CreateArtifactIssueParams) (db.ArtifactIssue, error) {
	f.issues = append(f.issues, params)
	return db.ArtifactIssue{ID: uuidWithByte(byte(30 + len(f.issues)))}, nil
}
