package tools

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/creative"
)

type UpsertProjectBriefNativeTool struct {
	service *creative.Service
}

type UpsertProjectBriefToolInput struct {
	Brief          string           `json:"brief" jsonschema:"required" jsonschema_description:"本次写入 CreativeBrief 的业务目的，例如为新广告创建 active brief。不要超过 160 个中文字符。"`
	Mode           string           `json:"mode" jsonschema:"required,enum=create,enum=patch,enum=archive" jsonschema_description:"create 创建新 active brief；patch 局部更新已有 brief；archive 归档指定 brief。"`
	BriefID        string           `json:"brief_id" jsonschema_description:"要 patch 或 archive 的 CreativeBrief UUID。create 时通常为空。为空 patch 时默认更新当前 active brief。"`
	Title          string           `json:"title" jsonschema_description:"视频项目标题，给用户和画布展示使用，例如“悦行行李箱机场广告”。"`
	VideoType      string           `json:"video_type" jsonschema_description:"视频类型，例如 marketing_ad、product_demo、brand_story、social_short。不要写具体分镜。"`
	TargetAudience string           `json:"target_audience" jsonschema_description:"目标受众，例如“短途商务出行用户”。如果用户没说，可以留空或写合理摘要。"`
	Tone           string           `json:"tone" jsonschema_description:"整体情感调性，例如轻快、可靠、高级。"`
	VisualStyle    string           `json:"visual_style" jsonschema_description:"整体视觉风格，例如现代机场、清晨自然光、商业质感。不要写 provider prompt 约束包。"`
	DurationSec    *float64         `json:"duration_sec" jsonschema_description:"目标总时长，单位秒。未知时留空；必须大于 0。"`
	AspectRatio    string           `json:"aspect_ratio" jsonschema_description:"视频比例，例如 9:16、16:9、1:1。未知时留空。"`
	Language       string           `json:"language" jsonschema_description:"主要语言，例如 zh-CN。"`
	Objective      string           `json:"objective" jsonschema_description:"视频要达成的业务目标，例如突出悦行行李箱适合短途商务出行。"`
	Concept        string           `json:"concept" jsonschema_description:"一句或几句自然语言创意概念，例如在机场拉箱的轻松出行广告。不要写分镜列表或模型 prompt。"`
	Constraints    []BriefRuleInput `json:"constraints" jsonschema_description:"brief 层约束列表。全片不可妥协的约束应提升到 ProjectMemory。"`
	Reason         string           `json:"reason" jsonschema_description:"为什么创建或修改这个 brief，用于审计。"`
}

type BriefRuleInput struct {
	Text     string `json:"text" jsonschema:"required" jsonschema_description:"约束内容。"`
	Severity string `json:"severity" jsonschema:"enum=low,enum=medium,enum=high,enum=blocking" jsonschema_description:"约束严重程度。blocking 表示不能违反。"`
}

func NewUpsertProjectBriefNativeTool(service *creative.Service) *UpsertProjectBriefNativeTool {
	return &UpsertProjectBriefNativeTool{service: service}
}

func (t *UpsertProjectBriefNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[UpsertProjectBriefToolInput](toolUpsertBrief, "创建、局部更新或归档当前 ClipAnvil Agent workspace 的创意简报 CreativeBrief。CreativeBrief 描述这条视频要做什么、给谁看、整体调性、风格、比例、语言、时长、目标和创意概念。")
}

func (t *UpsertProjectBriefNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolUpsertBrief, argumentsInJSON, validateUpsertProjectBriefInput)
	if !ok {
		return msg, nil
	}
	if msg, ok := serviceOrError(t.service, toolUpsertBrief); !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolUpsertBrief)
	if !ok {
		return msg, nil
	}
	brief, err := t.service.UpsertProjectBrief(ctx, creative.UpsertProjectBriefInput{
		WorkspaceID:    runtime.WorkspaceID,
		ThreadID:       runtime.ThreadID,
		TaskID:         runtime.TaskID,
		Brief:          input.Brief,
		Mode:           input.Mode,
		BriefID:        input.BriefID,
		Title:          input.Title,
		VideoType:      input.VideoType,
		TargetAudience: input.TargetAudience,
		Tone:           input.Tone,
		VisualStyle:    input.VisualStyle,
		DurationSec:    ptrFloat64(input.DurationSec),
		AspectRatio:    input.AspectRatio,
		Language:       input.Language,
		Objective:      input.Objective,
		Concept:        input.Concept,
		Constraints:    toCreativeBriefRules(input.Constraints),
		Reason:         input.Reason,
	})
	if err != nil {
		return naturalErrorFromErr(toolUpsertBrief, err), nil
	}
	return NaturalResult{
		Title: "已更新 CreativeBrief",
		Items: []NaturalResultItem{
			{Label: "标题", Value: brief.Title},
			{Label: "状态", Value: brief.Status},
			{Label: "视频类型", Value: brief.VideoType},
			{Label: "比例", Value: brief.AspectRatio},
		},
		Next: fmt.Sprintf("继续检查是否需要更新 ProjectMemory 和关键元素。brief_id=%s", uuidString(brief.ID)),
	}.String(), nil
}

func validateUpsertProjectBriefInput(input UpsertProjectBriefToolInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireMode(input.Mode, "create", "patch", "archive"); err != nil {
		return err
	}
	if input.DurationSec != nil && *input.DurationSec <= 0 {
		return fmt.Errorf("duration_sec 必须大于 0")
	}
	if input.Mode == "create" && input.Title == "" {
		return requireText("", "title")
	}
	for _, rule := range input.Constraints {
		if strings.TrimSpace(rule.Text) == "" {
			return requireText("", "constraints.text")
		}
		if rule.Severity != "" {
			if err := requireMode(rule.Severity, "low", "medium", "high", "blocking"); err != nil {
				return err
			}
		}
	}
	return nil
}

func toCreativeBriefRules(input []BriefRuleInput) []creative.BriefRule {
	out := make([]creative.BriefRule, 0, len(input))
	for _, item := range input {
		out = append(out, creative.BriefRule{Text: item.Text, Severity: item.Severity})
	}
	return out
}
