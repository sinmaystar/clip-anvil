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
	if store.create.ArtifactVersionID != versionID {
		t.Fatalf("artifact_version_id = %s, want %s", uuidString(store.create.ArtifactVersionID), uuidString(versionID))
	}
	if store.create.NodeID != nodeID || store.create.GenerationJobID != jobID || store.create.RenderPlanID != planID {
		t.Fatalf("target ids were not canonicalized: %#v", store.create)
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
	return db.ReviewRecord{ID: params.ID}, nil
}

func (f *fakeSubmitReviewResultStore) CreateArtifactIssue(_ context.Context, params db.CreateArtifactIssueParams) (db.ArtifactIssue, error) {
	f.issues = append(f.issues, params)
	return db.ArtifactIssue{ID: uuidWithByte(byte(30 + len(f.issues)))}, nil
}
