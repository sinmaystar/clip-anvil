package tools

import (
	"strings"
	"testing"
)

func TestNaturalToolErrorIsRetryableChineseObservation(t *testing.T) {
	got := NaturalToolError("upsert_project_brief", "mode 只能是 create、patch、archive", "请修正 mode 后重试。")
	for _, want := range []string{"工具调用失败", "upsert_project_brief", "mode 只能是", "请修正 mode 后重试"} {
		if !strings.Contains(got, want) {
			t.Fatalf("result missing %q: %s", want, got)
		}
	}
}

func TestNaturalResultStringDoesNotReturnRawJSON(t *testing.T) {
	got := NaturalResult{
		Title: "已更新 CreativeBrief",
		Items: []NaturalResultItem{
			{Label: "标题", Value: "悦行行李箱机场广告"},
			{Label: "下一步", Value: "继续创建 ProjectMemory"},
		},
		Next: "读取上下文确认状态。",
	}.String()
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Fatalf("natural result looks like raw JSON: %s", got)
	}
	if !strings.Contains(got, "已更新 CreativeBrief") || !strings.Contains(got, "下一步") {
		t.Fatalf("unexpected natural result: %s", got)
	}
}
