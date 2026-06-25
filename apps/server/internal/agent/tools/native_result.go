package tools

import (
	"fmt"
	"strings"
)

type NaturalResult struct {
	Title string
	Items []NaturalResultItem
	Next  string
}

type NaturalResultItem struct {
	Label string
	Value string
}

func (r NaturalResult) String() string {
	var b strings.Builder
	title := strings.TrimSpace(r.Title)
	if title == "" {
		title = "工具调用完成"
	}
	b.WriteString(title)
	for _, item := range r.Items {
		label := strings.TrimSpace(item.Label)
		value := strings.TrimSpace(item.Value)
		if label == "" || value == "" {
			continue
		}
		fmt.Fprintf(&b, "\n- %s：%s", label, value)
	}
	if next := strings.TrimSpace(r.Next); next != "" {
		fmt.Fprintf(&b, "\n下一步：%s", next)
	}
	return b.String()
}

func NaturalToolError(toolName string, problem string, retrySuggestion string) string {
	problem = strings.TrimSpace(problem)
	if problem == "" {
		problem = "参数或当前项目状态不满足工具要求"
	}
	retrySuggestion = strings.TrimSpace(retrySuggestion)
	if retrySuggestion == "" {
		retrySuggestion = "请读取当前项目上下文，修正参数后重试。"
	}
	return NaturalResult{
		Title: "工具调用失败",
		Items: []NaturalResultItem{
			{Label: "工具", Value: strings.TrimSpace(toolName)},
			{Label: "问题", Value: problem},
			{Label: "重试建议", Value: retrySuggestion},
		},
	}.String()
}
