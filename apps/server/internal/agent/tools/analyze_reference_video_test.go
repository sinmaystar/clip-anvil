package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/referencevideo"
)

func TestAnalyzeReferenceVideoToolInfo(t *testing.T) {
	tool := NewAnalyzeReferenceVideoNativeTool(nil, nil)
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "analyze_reference_video" {
		t.Fatalf("name = %q", info.Name)
	}
	if !strings.Contains(info.Desc, "参考视频") {
		t.Fatalf("desc = %q", info.Desc)
	}
}

func TestAnalyzeReferenceVideoToolRequiresSemanticMediaNodeRef(t *testing.T) {
	tool := NewAnalyzeReferenceVideoNativeTool(nil, nil)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByteForAnalyzeVideoTest(1),
		ThreadID:    uuidWithByteForAnalyzeVideoTest(2),
		TaskID:      uuidWithByteForAnalyzeVideoTest(3),
		TaskType:    "producer_turn",
	})
	out, err := tool.InvokableRun(ctx, `{"brief":"分析参考视频","video_ref":{"type":"media_node"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "video_ref.key 必填") {
		t.Fatalf("out = %s", out)
	}
}

func TestAnalyzeReferenceVideoToolCallsService(t *testing.T) {
	service := &fakeReferenceVideoAnalysisService{
		out: referencevideo.AnalyzeOutput{
			ID:       "analysis-1",
			Status:   referencevideo.StatusSucceeded,
			Summary:  "前三秒痛点 hook。",
			Warnings: []string{"不要复制原字幕。"},
		},
	}
	resolver := fakeObjectResolver{
		objects: map[string]objectResolution{
			"media_node/source.ref_video_01.node": {
				objectType: "media_node",
				objectID:   uuidWithByteForAnalyzeVideoTest(9),
			},
		},
	}
	tool := NewAnalyzeReferenceVideoNativeTool(service, resolver)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByteForAnalyzeVideoTest(1),
		ThreadID:    uuidWithByteForAnalyzeVideoTest(2),
		TaskID:      uuidWithByteForAnalyzeVideoTest(3),
		TaskType:    "producer_turn",
	})
	out, err := tool.InvokableRun(ctx, `{
		"brief":"借鉴脚本和运镜",
		"video_ref":{"type":"media_node","key":"source.ref_video_01.node"},
		"focus":["hook","camera_language"],
		"adaptation_target":{"product":"悦行行李箱"}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if service.input.SourceNodeID != uuidWithByteForAnalyzeVideoTest(9) || service.input.Brief != "借鉴脚本和运镜" {
		t.Fatalf("input = %#v", service.input)
	}
	if !strings.Contains(out, "前三秒痛点 hook") || !strings.Contains(out, "analysis-1") {
		t.Fatalf("out = %s", out)
	}
}

type fakeReferenceVideoAnalysisService struct {
	input referencevideo.AnalyzeInput
	out   referencevideo.AnalyzeOutput
	err   error
}

func (f *fakeReferenceVideoAnalysisService) Analyze(_ context.Context, input referencevideo.AnalyzeInput) (referencevideo.AnalyzeOutput, error) {
	f.input = input
	return f.out, f.err
}

type objectResolution struct {
	objectType string
	objectID   pgtype.UUID
}

type fakeObjectResolver struct {
	objects map[string]objectResolution
}

func (f fakeObjectResolver) ResolveObjectRef(_ context.Context, workspaceID pgtype.UUID, ref ToolObjectRef) (pgtype.UUID, bool, error) {
	item, ok := f.objects[ref.Type+"/"+ref.Key]
	if !ok || item.objectType != ref.Type || !workspaceID.Valid {
		return pgtype.UUID{}, false, nil
	}
	return item.objectID, true, nil
}

func uuidWithByteForAnalyzeVideoTest(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{value, value, value, value, value, value, value, value, value, value, value, value, value, value, value, value}, Valid: true}
}
