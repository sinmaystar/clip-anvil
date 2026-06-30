package tools

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/referencevideo"
)

type ReferenceVideoAnalysisService interface {
	Analyze(ctx context.Context, input referencevideo.AnalyzeInput) (referencevideo.AnalyzeOutput, error)
}

type ObjectRefResolver interface {
	ResolveObjectRef(ctx context.Context, workspaceID pgtype.UUID, ref ToolObjectRef) (pgtype.UUID, bool, error)
}

type AnalyzeReferenceVideoNativeTool struct {
	service  ReferenceVideoAnalysisService
	resolver ObjectRefResolver
}

type AnalyzeReferenceVideoToolInput struct {
	Brief            string         `json:"brief" jsonschema:"required" jsonschema_description:"本次分析参考视频的业务目的，例如借鉴脚本结构、运镜和节奏用于行李箱广告。不要超过 160 个中文字符。"`
	VideoRef         ToolObjectRef  `json:"video_ref" jsonschema:"required" jsonschema_description:"参考视频 media_node 语义引用，必须来自 read_project_context 返回的 ObjectIndex。"`
	Focus            []string       `json:"focus" jsonschema_description:"分析重点，例如 hook、selling_script、camera_language、pacing、subtitle_style、audio_role。为空时工具使用默认重点。"`
	AdaptationTarget map[string]any `json:"adaptation_target" jsonschema_description:"当前改编目标，例如 product、platform、aspect_ratio、duration_sec。只写真实已知信息。"`
}

func NewAnalyzeReferenceVideoNativeTool(service ReferenceVideoAnalysisService, resolver ObjectRefResolver) *AnalyzeReferenceVideoNativeTool {
	return &AnalyzeReferenceVideoNativeTool{service: service, resolver: resolver}
}

func (t *AnalyzeReferenceVideoNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[AnalyzeReferenceVideoToolInput](toolAnalyzeReferenceVideo, "分析用户上传的参考视频，提取可借鉴的脚本结构、运镜、节奏、字幕、音频角色和不可复制边界，供 Producer 写入 ProjectMemory 与 Storyboard。")
}

func (t *AnalyzeReferenceVideoNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolAnalyzeReferenceVideo, argumentsInJSON, validateAnalyzeReferenceVideoInput)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolAnalyzeReferenceVideo)
	if !ok {
		return msg, nil
	}
	if t.service == nil || t.resolver == nil {
		return NaturalToolError(toolAnalyzeReferenceVideo, "reference video analysis service 未配置。", "请检查服务端 wiring 后重试。"), nil
	}
	sourceNodeID, found, err := t.resolver.ResolveObjectRef(ctx, runtime.WorkspaceID, input.VideoRef)
	if err != nil {
		return naturalErrorFromErr(toolAnalyzeReferenceVideo, err), nil
	}
	if !found {
		return NaturalToolError(toolAnalyzeReferenceVideo, "video_ref 未匹配到当前项目的 media_node。", "请先调用 read_project_context，使用 ObjectIndex 中的 media_node semantic_key。"), nil
	}
	out, err := t.service.Analyze(ctx, referencevideo.AnalyzeInput{
		WorkspaceID:      runtime.WorkspaceID,
		ThreadID:         runtime.ThreadID,
		TaskID:           runtime.TaskID,
		SourceNodeID:     sourceNodeID,
		Brief:            input.Brief,
		Focus:            input.Focus,
		AdaptationTarget: input.AdaptationTarget,
	})
	if err != nil {
		return naturalErrorFromErr(toolAnalyzeReferenceVideo, err), nil
	}
	items := []NaturalResultItem{
		{Label: "Analysis", Value: out.ID + " / " + out.Status},
		{Label: "摘要", Value: out.Summary},
	}
	if len(out.Warnings) > 0 {
		items = append(items, NaturalResultItem{Label: "Warnings", Value: strings.Join(out.Warnings, "；")})
	}
	return NaturalResult{
		Title: "参考视频分析完成",
		Items: items,
		Next:  "Producer 应把可复用的风格和结构写入 ProjectMemory/Storyboard，并保留不可复制边界；如方向有歧义，请 request_user_decision。",
	}.String(), nil
}

func validateAnalyzeReferenceVideoInput(input AnalyzeReferenceVideoToolInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if strings.TrimSpace(input.VideoRef.Type) != "media_node" {
		return fmt.Errorf("video_ref.type 必须是 media_node")
	}
	if strings.TrimSpace(input.VideoRef.Key) == "" {
		return fmt.Errorf("video_ref.key 必填，请使用 read_project_context 返回的 media_node semantic_key")
	}
	return nil
}
