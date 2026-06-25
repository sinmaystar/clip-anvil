package tools

import (
	"context"
	"strings"
	"testing"
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
