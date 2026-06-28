package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/sandbox"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const (
	toolGetCompositionContext      = "get_composition_context"
	toolStageMediaInputs           = "stage_media_inputs"
	toolProbeMedia                 = "probe_media"
	toolCreateTimelinePlan         = "create_timeline_plan"
	toolUpdateTimelinePlanStatus   = "update_timeline_plan_status"
	toolRenderTimelineTemplate     = "render_timeline_template"
	toolRunFFmpegCommand           = "run_ffmpeg_command"
	toolSubmitCompositionArtifact  = "submit_composition_artifact"
	composerTemplateSimpleConcat   = "simple_concat"
	composerTemplateConcatWithFade = "concat_with_fades"
)

type CompositionContextProvider interface {
	GetCompositionContext(ctx context.Context, runtime NativeRuntimeContext, sourceNodeID pgtype.UUID) (map[string]any, error)
}

type CompositionSandbox interface {
	StageMediaInputs(ctx context.Context, input sandbox.StageMediaInputsInput) (sandbox.StageMediaInputsResult, error)
	ProbeMedia(ctx context.Context, input sandbox.ProbeMediaInput) (sandbox.ProbeMediaResult, error)
	RunFFmpegCommand(ctx context.Context, input sandbox.RunFFmpegCommandInput) (sandbox.SandboxJobResult, error)
}

type CompositionTimelineStore interface {
	CreateTimelinePlan(ctx context.Context, arg db.CreateTimelinePlanParams) (db.TimelinePlan, error)
	GetTimelinePlan(ctx context.Context, id pgtype.UUID) (db.TimelinePlan, error)
	UpdateTimelinePlanStatus(ctx context.Context, arg db.UpdateTimelinePlanStatusParams) (db.TimelinePlan, error)
}

type CompositionTemplateRenderer interface {
	RenderTimelineTemplate(ctx context.Context, runtime NativeRuntimeContext, input RenderTimelineTemplateInput) (RenderTimelineTemplateResult, error)
}

type CompositionArtifactPersister interface {
	PersistComposerArtifact(ctx context.Context, input production.ComposerArtifactInput) (production.RunResult, error)
}

type CompositionArtifactStore interface {
	CreateAgentGenerationNode(ctx context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error)
	GetTimelinePlan(ctx context.Context, id pgtype.UUID) (db.TimelinePlan, error)
	UpdateTimelinePlanStatus(ctx context.Context, arg db.UpdateTimelinePlanStatusParams) (db.TimelinePlan, error)
	UpdateAudioPlanTimelinePlan(ctx context.Context, arg db.UpdateAudioPlanTimelinePlanParams) (db.AudioPlan, error)
}

type CompositionOutputUploader interface {
	UploadCompositionOutput(ctx context.Context, input sandbox.UploadCompositionOutputInput) (sandbox.SandboxJobResult, error)
}

type GetCompositionContextNativeTool struct{ provider CompositionContextProvider }
type StageMediaInputsNativeTool struct{ sandbox CompositionSandbox }
type ProbeMediaNativeTool struct{ sandbox CompositionSandbox }
type CreateTimelinePlanNativeTool struct{ store CompositionTimelineStore }
type UpdateTimelinePlanStatusNativeTool struct{ store CompositionTimelineStore }
type RenderTimelineTemplateNativeTool struct{ renderer CompositionTemplateRenderer }
type RunFFmpegCommandNativeTool struct{ sandbox CompositionSandbox }
type SubmitCompositionArtifactNativeTool struct {
	persister CompositionArtifactPersister
	store     CompositionArtifactStore
	uploader  CompositionOutputUploader
}

type SandboxTimelineTemplateRenderer struct {
	sandbox CompositionSandbox
}

type GetCompositionContextInput struct {
	SourceStoryboardNodeID string `json:"source_storyboard_node_id,omitempty" jsonschema_description:"可选兼容字段。来源媒体/故事板节点内部 ID；通常由 dispatch_composer task 自动提供，模型不要自行填写。为空时读取 workspace 下可用于成片的候选素材。"`
}

type StageMediaInputsInput struct {
	Assets    []StageMediaAssetInput `json:"assets" jsonschema:"required" jsonschema_description:"需要 staging 到 sandbox 的媒体资产列表。"`
	TargetDir string                 `json:"target_dir,omitempty" jsonschema_description:"sandbox 目标目录，默认 /workspace/input。"`
}

type StageMediaAssetInput struct {
	AssetID   string `json:"asset_id" jsonschema:"required" jsonschema_description:"媒体资产内部 ID。必须从 get_composition_context 返回的 assets 原样复制，不要自行填写。"`
	SourceURL string `json:"source_url" jsonschema:"required" jsonschema_description:"内部 storage url 或已签名下载 URL。"`
	FileName  string `json:"file_name" jsonschema:"required" jsonschema_description:"期望写入 sandbox 的文件名。"`
	MimeType  string `json:"mime_type,omitempty" jsonschema_description:"媒体 MIME 类型。"`
	SizeBytes int64  `json:"size_bytes,omitempty" jsonschema_description:"可选文件大小。"`
}

type ProbeMediaToolInput struct {
	WorkspacePath string `json:"workspace_path" jsonschema:"required" jsonschema_description:"sandbox 内 /workspace 下的媒体路径。"`
}

type CreateTimelinePlanInput struct {
	SourceStoryboardNodeID string         `json:"source_storyboard_node_id,omitempty" jsonschema_description:"来源媒体/故事板节点内部 ID。通常从当前 Composer task 或 get_composition_context 返回值原样复制。"`
	TemplateKey            string         `json:"template_key" jsonschema:"required,enum=simple_concat,enum=concat_with_fades" jsonschema_description:"Phase 1 timeline 模版。"`
	Plan                   map[string]any `json:"plan" jsonschema:"required" jsonschema_description:"TimelinePlan JSON。"`
	RenderSettings         map[string]any `json:"render_settings,omitempty" jsonschema_description:"渲染设置。"`
}

type UpdateTimelinePlanStatusInput struct {
	TimelinePlanID    string         `json:"timeline_plan_id" jsonschema:"required" jsonschema_description:"timeline_plan 内部 ID。必须从 create_timeline_plan 返回结果原样复制。"`
	Status            string         `json:"status" jsonschema:"required,enum=draft,enum=rendering,enum=completed,enum=blocked,enum=failed" jsonschema_description:"新的 timeline_plan 状态。"`
	ProductionJobID   string         `json:"production_job_id,omitempty"`
	ArtifactVersionID string         `json:"artifact_version_id,omitempty"`
	SandboxJobID      string         `json:"sandbox_job_id,omitempty"`
	Result            map[string]any `json:"result,omitempty"`
	ErrorMessage      string         `json:"error_message,omitempty"`
}

type RenderTimelineTemplateInput struct {
	TimelinePlanID string         `json:"timeline_plan_id" jsonschema:"required" jsonschema_description:"timeline_plan 内部 ID。必须从 create_timeline_plan 返回结果原样复制。"`
	TemplateKey    string         `json:"template_key" jsonschema:"required,enum=simple_concat,enum=concat_with_fades" jsonschema_description:"Phase 1 模版。"`
	Plan           map[string]any `json:"plan" jsonschema:"required" jsonschema_description:"TimelinePlan JSON。"`
}

type RenderTimelineTemplateResult struct {
	OutputPath   string      `json:"output_path"`
	SandboxJobID pgtype.UUID `json:"sandbox_job_id"`
	Summary      string      `json:"summary"`
}

type RunFFmpegCommandToolInput struct {
	Executable string   `json:"executable" jsonschema:"required,enum=ffmpeg,enum=ffprobe" jsonschema_description:"只能是 ffmpeg 或 ffprobe。"`
	Cwd        string   `json:"cwd,omitempty" jsonschema_description:"sandbox 工作目录，默认 /workspace。"`
	Args       []string `json:"args" jsonschema:"required" jsonschema_description:"ffmpeg/ffprobe 参数数组。路径必须留在 /workspace 内。"`
	TimeoutSec int      `json:"timeout_sec,omitempty" jsonschema_description:"超时时间秒，最大由 sandbox 限制。"`
}

type SubmitCompositionArtifactInput struct {
	OutputNodeID   string         `json:"output_node_id,omitempty" jsonschema_description:"可选。最终成片输出 media_node 内部 ID；通常不要填写，工具会根据 timeline_plan_id 自动创建或复用。"`
	OutputPath     string         `json:"output_path" jsonschema:"required" jsonschema_description:"sandbox 内 /workspace/output 下的最终视频路径。"`
	TimelinePlanID string         `json:"timeline_plan_id" jsonschema:"required" jsonschema_description:"对应 timeline_plan 内部 ID。必须从 create_timeline_plan 返回结果原样复制；工具会用它自动创建/复用最终输出节点并回填 timeline。"`
	SandboxJobID   string         `json:"sandbox_job_id,omitempty" jsonschema_description:"渲染 sandbox_job 内部 ID。必须从 render_timeline_template 或 run_ffmpeg_command 返回结果原样复制。"`
	MimeType       string         `json:"mime_type,omitempty" jsonschema_description:"默认 video/mp4。"`
	StorageURL     string         `json:"storage_url,omitempty" jsonschema_description:"可选。已上传到对象存储后的 storage url；通常不要填写，工具会从 output_path 自动上传到对象存储。"`
	SizeBytes      int64          `json:"size_bytes,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
}

func NewGetCompositionContextNativeTool(provider CompositionContextProvider) GetCompositionContextNativeTool {
	return GetCompositionContextNativeTool{provider: provider}
}

func NewStageMediaInputsNativeTool(sandbox CompositionSandbox) StageMediaInputsNativeTool {
	return StageMediaInputsNativeTool{sandbox: sandbox}
}

func NewProbeMediaNativeTool(sandbox CompositionSandbox) ProbeMediaNativeTool {
	return ProbeMediaNativeTool{sandbox: sandbox}
}

func NewCreateTimelinePlanNativeTool(store CompositionTimelineStore) CreateTimelinePlanNativeTool {
	return CreateTimelinePlanNativeTool{store: store}
}

func NewUpdateTimelinePlanStatusNativeTool(store CompositionTimelineStore) UpdateTimelinePlanStatusNativeTool {
	return UpdateTimelinePlanStatusNativeTool{store: store}
}

func NewRenderTimelineTemplateNativeTool(renderer CompositionTemplateRenderer) RenderTimelineTemplateNativeTool {
	return RenderTimelineTemplateNativeTool{renderer: renderer}
}

func NewRunFFmpegCommandNativeTool(sandbox CompositionSandbox) RunFFmpegCommandNativeTool {
	return RunFFmpegCommandNativeTool{sandbox: sandbox}
}

func NewSandboxTimelineTemplateRenderer(sandbox CompositionSandbox) SandboxTimelineTemplateRenderer {
	return SandboxTimelineTemplateRenderer{sandbox: sandbox}
}

func NewSubmitCompositionArtifactNativeTool(persister CompositionArtifactPersister, store ...CompositionArtifactStore) SubmitCompositionArtifactNativeTool {
	tool := SubmitCompositionArtifactNativeTool{persister: persister}
	if len(store) > 0 {
		tool.store = store[0]
	}
	return tool
}

func (t SubmitCompositionArtifactNativeTool) WithOutputUploader(uploader CompositionOutputUploader) SubmitCompositionArtifactNativeTool {
	t.uploader = uploader
	return t
}

func (t GetCompositionContextNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[GetCompositionContextInput](toolGetCompositionContext, "读取 Composer 成片上下文，包括可用媒体、历史 timeline plan、审核状态和 sandbox 状态。")
}

func (t GetCompositionContextNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs[GetCompositionContextInput](toolGetCompositionContext, raw, nil)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolGetCompositionContext)
	if !ok {
		return msg, nil
	}
	if t.provider == nil {
		return NaturalToolError(toolGetCompositionContext, "composition context provider 未配置。", "请检查 Composer graph wiring。"), nil
	}
	sourceID, _ := pgUUIDFromString(input.SourceStoryboardNodeID)
	result, err := t.provider.GetCompositionContext(ctx, runtime, sourceID)
	if err != nil {
		return NaturalToolError(toolGetCompositionContext, err.Error(), "请确认 workspace 和 source_storyboard_node_id 后重试。"), nil
	}
	return jsonStringResult(toolGetCompositionContext, result)
}

func (t StageMediaInputsNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[StageMediaInputsInput](toolStageMediaInputs, "把 Composer 需要的媒体输入显式 staging 到 sandbox 的 /workspace/input，返回 sandbox 路径 manifest。")
}

func (t StageMediaInputsNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolStageMediaInputs, raw, validateStageMediaInputs)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolStageMediaInputs)
	if !ok {
		return msg, nil
	}
	if t.sandbox == nil {
		return NaturalToolError(toolStageMediaInputs, "composition sandbox 未配置。", "请检查 Composer graph wiring。"), nil
	}
	assets := make([]sandbox.StageMediaAssetInput, 0, len(input.Assets))
	for _, item := range input.Assets {
		assetID, _ := pgUUIDFromString(item.AssetID)
		assets = append(assets, sandbox.StageMediaAssetInput{
			AssetID:   assetID,
			SourceURL: item.SourceURL,
			FileName:  item.FileName,
			MimeType:  item.MimeType,
			SizeBytes: item.SizeBytes,
		})
	}
	result, err := t.sandbox.StageMediaInputs(ctx, sandbox.StageMediaInputsInput{
		WorkspaceID:  runtime.WorkspaceID,
		TargetNodeID: runtime.ScopeID,
		Assets:       assets,
		TargetDir:    input.TargetDir,
	})
	if err != nil {
		return NaturalToolError(toolStageMediaInputs, err.Error(), "请检查 source_url、asset_id 和 sandbox 状态后重试。"), nil
	}
	return jsonStringResult(toolStageMediaInputs, result)
}

func (t ProbeMediaNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[ProbeMediaToolInput](toolProbeMedia, "用 ffprobe 读取 sandbox 内媒体的 streams/format 信息。")
}

func (t ProbeMediaNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolProbeMedia, raw, func(input ProbeMediaToolInput) error {
		return requireText(input.WorkspacePath, "workspace_path")
	})
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolProbeMedia)
	if !ok {
		return msg, nil
	}
	if t.sandbox == nil {
		return NaturalToolError(toolProbeMedia, "composition sandbox 未配置。", "请检查 Composer graph wiring。"), nil
	}
	result, err := t.sandbox.ProbeMedia(ctx, sandbox.ProbeMediaInput{WorkspaceID: runtime.WorkspaceID, TargetNodeID: runtime.ScopeID, WorkspacePath: input.WorkspacePath})
	if err != nil {
		return NaturalToolError(toolProbeMedia, err.Error(), "请确认媒体已经通过 stage_media_inputs 写入 sandbox。"), nil
	}
	return jsonStringResult(toolProbeMedia, result)
}

func (t CreateTimelinePlanNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[CreateTimelinePlanInput](toolCreateTimelinePlan, "创建 Composer timeline_plan 草稿，作为最终成片渲染的持久化计划。")
}

func (t CreateTimelinePlanNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolCreateTimelinePlan, raw, validateCreateTimelinePlan)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolCreateTimelinePlan)
	if !ok {
		return msg, nil
	}
	if t.store == nil {
		return NaturalToolError(toolCreateTimelinePlan, "timeline plan store 未配置。", "请检查 Composer graph wiring。"), nil
	}
	sourceID, _ := pgUUIDFromString(input.SourceStoryboardNodeID)
	planJSON, _ := json.Marshal(input.Plan)
	settingsJSON, _ := json.Marshal(input.RenderSettings)
	plan, err := t.store.CreateTimelinePlan(ctx, db.CreateTimelinePlanParams{
		WorkspaceID:            runtime.WorkspaceID,
		SourceStoryboardNodeID: sourceID,
		Status:                 "draft",
		TemplateKey:            input.TemplateKey,
		PlanJson:               planJSON,
		RenderSettings:         defaultComposerJSON(settingsJSON),
		Result:                 []byte("{}"),
		CreatedByRole:          "composer",
		CreatedByTaskID:        runtime.TaskID,
	})
	if err != nil {
		return NaturalToolError(toolCreateTimelinePlan, err.Error(), "请检查 plan_json 是否符合 Composer timeline schema。"), nil
	}
	return jsonStringResult(toolCreateTimelinePlan, map[string]any{"timeline_plan_id": uuidString(plan.ID), "status": plan.Status})
}

func (t UpdateTimelinePlanStatusNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[UpdateTimelinePlanStatusInput](toolUpdateTimelinePlanStatus, "更新 Composer timeline_plan 状态和渲染结果。")
}

func (t UpdateTimelinePlanStatusNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolUpdateTimelinePlanStatus, raw, validateUpdateTimelinePlanStatus)
	if !ok {
		return msg, nil
	}
	if t.store == nil {
		return NaturalToolError(toolUpdateTimelinePlanStatus, "timeline plan store 未配置。", "请检查 Composer graph wiring。"), nil
	}
	planID, _ := pgUUIDFromString(input.TimelinePlanID)
	productionJobID, _ := pgUUIDFromString(input.ProductionJobID)
	artifactVersionID, _ := pgUUIDFromString(input.ArtifactVersionID)
	sandboxJobID, _ := pgUUIDFromString(input.SandboxJobID)
	resultJSON, _ := json.Marshal(input.Result)
	if len(input.Result) > 0 {
		if plan, err := t.store.GetTimelinePlan(ctx, planID); err == nil {
			resultJSON = mergeComposerResultJSON(plan.Result, resultJSON)
		}
	}
	plan, err := t.store.UpdateTimelinePlanStatus(ctx, db.UpdateTimelinePlanStatusParams{
		ID:                planID,
		Status:            input.Status,
		ProductionJobID:   productionJobID,
		ArtifactVersionID: artifactVersionID,
		SandboxJobID:      sandboxJobID,
		Result:            defaultComposerJSON(resultJSON),
		ErrorMessage:      pgtype.Text{String: input.ErrorMessage, Valid: strings.TrimSpace(input.ErrorMessage) != ""},
	})
	if err != nil {
		return NaturalToolError(toolUpdateTimelinePlanStatus, err.Error(), "请确认 timeline_plan_id 存在且状态转换合理。"), nil
	}
	return jsonStringResult(toolUpdateTimelinePlanStatus, map[string]any{"timeline_plan_id": uuidString(plan.ID), "status": plan.Status})
}

func (t RenderTimelineTemplateNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[RenderTimelineTemplateInput](toolRenderTimelineTemplate, "按 Phase 1 模版渲染 timeline plan，支持 simple_concat 和 concat_with_fades。")
}

func (t RenderTimelineTemplateNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolRenderTimelineTemplate, raw, validateRenderTimelineTemplate)
	if !ok {
		return msg, nil
	}
	if t.renderer == nil {
		return NaturalToolError(toolRenderTimelineTemplate, "timeline template renderer 未配置。", "请检查 Composer graph wiring。"), nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolRenderTimelineTemplate)
	if !ok {
		return msg, nil
	}
	result, err := t.renderer.RenderTimelineTemplate(ctx, runtime, input)
	if err != nil {
		return NaturalToolError(toolRenderTimelineTemplate, err.Error(), "请先 stage/probe 媒体并确认 timeline plan 可渲染。"), nil
	}
	return jsonStringResult(toolRenderTimelineTemplate, result)
}

func (t RunFFmpegCommandNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[RunFFmpegCommandToolInput](toolRunFFmpegCommand, "在 sandbox 中执行受控 ffmpeg/ffprobe 命令。不能执行 bash、sh 或访问 /workspace 外路径。")
}

func (t RunFFmpegCommandNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolRunFFmpegCommand, raw, validateRunFFmpegCommand)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolRunFFmpegCommand)
	if !ok {
		return msg, nil
	}
	if t.sandbox == nil {
		return NaturalToolError(toolRunFFmpegCommand, "composition sandbox 未配置。", "请检查 Composer graph wiring。"), nil
	}
	result, err := t.sandbox.RunFFmpegCommand(ctx, sandbox.RunFFmpegCommandInput{
		WorkspaceID:  runtime.WorkspaceID,
		TargetNodeID: runtime.ScopeID,
		Executable:   input.Executable,
		Cwd:          input.Cwd,
		Args:         input.Args,
		TimeoutSec:   input.TimeoutSec,
	})
	if err != nil {
		return NaturalToolError(toolRunFFmpegCommand, err.Error(), "请只使用 ffmpeg/ffprobe，并确保输入输出路径位于 /workspace。"), nil
	}
	return jsonStringResult(toolRunFFmpegCommand, map[string]any{
		"sandbox_job_id": uuidString(result.Job.ID),
		"exit_code":      result.Exec.ExitCode,
		"stdout":         result.Exec.Stdout,
		"stderr":         result.Exec.Stderr,
	})
}

func (t SubmitCompositionArtifactNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[SubmitCompositionArtifactInput](toolSubmitCompositionArtifact, "提交 Composer 已渲染的最终成片 artifact，并通过 production 持久化为最终输出。")
}

func (t SubmitCompositionArtifactNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolSubmitCompositionArtifact, raw, validateSubmitCompositionArtifact)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolSubmitCompositionArtifact)
	if !ok {
		return msg, nil
	}
	if t.persister == nil {
		return NaturalToolError(toolSubmitCompositionArtifact, "production artifact persister 未配置。", "请检查 Composer graph wiring。"), nil
	}
	outputNode, err := t.resolveOutputNode(ctx, runtime, input)
	if err != nil {
		return NaturalToolError(toolSubmitCompositionArtifact, err.Error(), "请确认 timeline_plan_id 或 output_node_id 后重试。"), nil
	}
	sandboxJobID, _ := pgUUIDFromString(input.SandboxJobID)
	mime := strings.TrimSpace(input.MimeType)
	if mime == "" {
		mime = "video/mp4"
	}
	storageURL := strings.TrimSpace(input.StorageURL)
	sizeBytes := input.SizeBytes
	if storageURL == "" {
		if t.uploader == nil {
			return NaturalToolError(toolSubmitCompositionArtifact, "composition output uploader 未配置。", "请配置 sandbox uploader，或提供已上传 storage_url。"), nil
		}
		upload, err := t.uploader.UploadCompositionOutput(ctx, sandbox.UploadCompositionOutputInput{
			WorkspaceID:        runtime.WorkspaceID,
			TargetNodeID:       outputNode.ID,
			SourceSandboxJobID: sandboxJobID,
			OutputPath:         input.OutputPath,
			MimeHint:           mime,
		})
		if err != nil {
			return NaturalToolError(toolSubmitCompositionArtifact, err.Error(), "请确认 output_path 指向 sandbox 内已生成的视频。"), nil
		}
		storageURL = upload.Asset.StorageURL
		if strings.TrimSpace(upload.MIME) != "" {
			mime = upload.MIME
		}
		if upload.Size > 0 {
			sizeBytes = upload.Size
		}
		if input.Result == nil {
			input.Result = map[string]any{}
		}
		input.Result["upload_sandbox_job_id"] = uuidString(upload.Job.ID)
	}
	if input.Result == nil {
		input.Result = map[string]any{}
	}
	input.Result["output_path"] = input.OutputPath
	input.Result["storage_url"] = storageURL
	input.Result["mime_type"] = mime
	if sizeBytes > 0 {
		input.Result["size_bytes"] = sizeBytes
	}
	providerResponse := map[string]any{
		"output_path": input.OutputPath,
		"storage_url": storageURL,
	}
	if input.TimelinePlanID != "" {
		providerResponse["timeline_plan_id"] = input.TimelinePlanID
	}
	result, err := t.persister.PersistComposerArtifact(ctx, production.ComposerArtifactInput{
		WorkspaceID:  runtime.WorkspaceID,
		OutputNodeID: outputNode.ID,
		SandboxJobID: sandboxJobID,
		TaskID:       runtime.TaskID,
		ProviderResult: production.ProviderResult{
			RenderedPrompt:    "Composer final video",
			AssetMIME:         mime,
			AssetStorageURL:   storageURL,
			AssetSizeBytes:    sizeBytes,
			AssetMetadata:     input.Result,
			ProviderRequest:   map[string]any{"output_path": input.OutputPath},
			ProviderResponse:  providerResponse,
			AssetThumbnailURL: "",
		},
	})
	if err != nil {
		return NaturalToolError(toolSubmitCompositionArtifact, err.Error(), "请确认 output_node_id、storage_url 和 sandbox_job_id 后重试。"), nil
	}
	if err := t.updateTimelineAfterSubmit(ctx, input, result, sandboxJobID, input.Result); err != nil {
		return NaturalToolError(toolSubmitCompositionArtifact, err.Error(), "最终 artifact 已持久化，但 timeline_plan 回填失败。"), nil
	}
	return jsonStringResult(toolSubmitCompositionArtifact, map[string]any{
		"output_node_id":       uuidString(result.Node.ID),
		"generation_job_id":    uuidString(result.Job.ID),
		"artifact_version_id":  uuidString(result.Version.ID),
		"timeline_plan_id":     strings.TrimSpace(input.TimelinePlanID),
		"sandbox_job_id":       uuidString(sandboxJobID),
		"generation_job_state": string(result.Job.Status),
	})
}

func (r SandboxTimelineTemplateRenderer) RenderTimelineTemplate(ctx context.Context, runtime NativeRuntimeContext, input RenderTimelineTemplateInput) (RenderTimelineTemplateResult, error) {
	if r.sandbox == nil {
		return RenderTimelineTemplateResult{}, errors.New("composition sandbox 未配置")
	}
	segments, err := timelineSegments(input.Plan)
	if err != nil {
		return RenderTimelineTemplateResult{}, err
	}
	fallbackOutputPath := "/workspace/output/final-" + shortToolID(input.TimelinePlanID) + ".mp4"
	outputPath, err := timelineOutputPath(input.Plan, fallbackOutputPath)
	if err != nil {
		return RenderTimelineTemplateResult{}, err
	}
	args, err := timelineFFmpegArgs(input.Plan, outputPath)
	if err != nil {
		return RenderTimelineTemplateResult{}, err
	}
	result, err := r.sandbox.RunFFmpegCommand(ctx, sandbox.RunFFmpegCommandInput{
		WorkspaceID:  runtime.WorkspaceID,
		TargetNodeID: runtime.ScopeID,
		Executable:   "ffmpeg",
		Args:         args,
		TimeoutSec:   180,
	})
	if err != nil {
		return RenderTimelineTemplateResult{}, err
	}
	return RenderTimelineTemplateResult{
		OutputPath:   outputPath,
		SandboxJobID: result.Job.ID,
		Summary:      fmt.Sprintf("rendered %d segments with %s", len(segments), input.TemplateKey),
	}, nil
}

func (t SubmitCompositionArtifactNativeTool) resolveOutputNode(ctx context.Context, runtime NativeRuntimeContext, input SubmitCompositionArtifactInput) (db.MediaNode, error) {
	if outputNodeID, ok := pgUUIDFromString(input.OutputNodeID); ok {
		return db.MediaNode{ID: outputNodeID, WorkspaceID: runtime.WorkspaceID, NodeType: db.NodeTypeVideo, OperationType: "compose_final_video"}, nil
	}
	if t.store == nil {
		return db.MediaNode{}, errors.New("composition artifact store 未配置，无法自动创建最终成片节点")
	}
	timelinePlanID, ok := pgUUIDFromString(input.TimelinePlanID)
	if !ok {
		return db.MediaNode{}, errors.New("缺少 timeline_plan_id，无法自动创建最终成片节点")
	}
	plan, err := t.store.GetTimelinePlan(ctx, timelinePlanID)
	if err != nil {
		return db.MediaNode{}, err
	}
	if plan.OutputNodeID.Valid {
		return db.MediaNode{ID: plan.OutputNodeID, WorkspaceID: runtime.WorkspaceID, NodeType: db.NodeTypeVideo, OperationType: "compose_final_video"}, nil
	}
	metadata, _ := json.Marshal(map[string]any{
		"timeline_plan_id": input.TimelinePlanID,
		"created_by":       "composer",
	})
	node, err := t.store.CreateAgentGenerationNode(ctx, db.CreateAgentGenerationNodeParams{
		WorkspaceID:   runtime.WorkspaceID,
		NodeType:      db.NodeTypeVideo,
		Title:         "Composer final video",
		Prompt:        "Composer final video",
		OperationType: "compose_final_video",
		CanvasX:       0,
		CanvasY:       0,
		CanvasW:       320,
		CanvasH:       180,
		ModelProvider: pgtype.Text{String: "internal_ffmpeg", Valid: true},
		ModelID:       pgtype.Text{String: "ffmpeg", Valid: true},
		ModelParams:   []byte("{}"),
		Metadata:      metadata,
		SemanticKey:   "composer.final_output." + shortToolID(input.TimelinePlanID) + ".node",
		DisplayName:   "Composer final video",
		ArtifactKind:  "final_video",
	})
	if err != nil {
		return db.MediaNode{}, err
	}
	return node, nil
}

func (t SubmitCompositionArtifactNativeTool) updateTimelineAfterSubmit(ctx context.Context, input SubmitCompositionArtifactInput, result production.RunResult, sandboxJobID pgtype.UUID, resultMetadata map[string]any) error {
	if t.store == nil || strings.TrimSpace(input.TimelinePlanID) == "" {
		return nil
	}
	timelinePlanID, ok := pgUUIDFromString(input.TimelinePlanID)
	if !ok {
		return nil
	}
	resultJSON, _ := json.Marshal(resultMetadata)
	_, err := t.store.UpdateTimelinePlanStatus(ctx, db.UpdateTimelinePlanStatusParams{
		ID:                timelinePlanID,
		Status:            "completed",
		OutputNodeID:      result.Node.ID,
		ProductionJobID:   result.Job.ID,
		ArtifactVersionID: result.Version.ID,
		SandboxJobID:      sandboxJobID,
		Result:            defaultComposerJSON(resultJSON),
	})
	if err != nil {
		return err
	}
	_, err = t.store.UpdateAudioPlanTimelinePlan(ctx, db.UpdateAudioPlanTimelinePlanParams{
		WorkspaceID:    result.Node.WorkspaceID,
		TimelinePlanID: timelinePlanID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

type timelineSegmentInput struct {
	WorkspacePath string
	DurationSec   float64
}

type timelineAudioInput struct {
	Role          string
	WorkspacePath string
	StartSec      float64
	DurationSec   float64
	Volume        float64
	FadeInSec     float64
	FadeOutSec    float64
	Ducking       timelineDuckingInput
}

type timelineDuckingInput struct {
	SidechainRole string
	Threshold     float64
	Ratio         float64
	AttackMS      int
	ReleaseMS     int
}

func timelineSegments(plan map[string]any) ([]timelineSegmentInput, error) {
	rawSegments, ok := plan["segments"].([]any)
	if !ok || len(rawSegments) == 0 {
		return nil, errors.New("plan.segments 至少需要 1 个 segment")
	}
	segments := make([]timelineSegmentInput, 0, len(rawSegments))
	for index, raw := range rawSegments {
		segment, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("plan.segments[%d] 必须是对象", index)
		}
		workspacePath := strings.TrimSpace(compositionStringValue(segment["workspace_path"]))
		if workspacePath == "" {
			return nil, fmt.Errorf("plan.segments[%d].workspace_path 必填", index)
		}
		if !strings.HasPrefix(path.Clean(workspacePath), "/workspace/") {
			return nil, fmt.Errorf("plan.segments[%d].workspace_path 必须位于 /workspace", index)
		}
		segments = append(segments, timelineSegmentInput{
			WorkspacePath: workspacePath,
			DurationSec:   compositionFloatValue(segment["duration_sec"], 0),
		})
	}
	return segments, nil
}

func timelineWorkspacePaths(plan map[string]any) ([]string, error) {
	segments, err := timelineSegments(plan)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(segments))
	for _, segment := range segments {
		paths = append(paths, segment.WorkspacePath)
	}
	return paths, nil
}

func timelineAudioTracks(plan map[string]any) ([]timelineAudioInput, error) {
	rawTracks, ok := plan["audio_tracks"].([]any)
	if !ok || len(rawTracks) == 0 {
		return nil, nil
	}
	tracks := make([]timelineAudioInput, 0, len(rawTracks))
	for index, raw := range rawTracks {
		track, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("plan.audio_tracks[%d] 必须是对象", index)
		}
		role := strings.TrimSpace(compositionStringValue(track["role"]))
		if role != "voiceover" && role != "bgm" {
			return nil, fmt.Errorf("plan.audio_tracks[%d].role 只支持 voiceover 或 bgm", index)
		}
		workspacePath := strings.TrimSpace(compositionStringValue(track["workspace_path"]))
		if workspacePath == "" {
			return nil, fmt.Errorf("plan.audio_tracks[%d].workspace_path 必填", index)
		}
		if !strings.HasPrefix(path.Clean(workspacePath), "/workspace/") {
			return nil, fmt.Errorf("plan.audio_tracks[%d].workspace_path 必须位于 /workspace", index)
		}
		audio := timelineAudioInput{
			Role:          role,
			WorkspacePath: workspacePath,
			StartSec:      compositionFloatValue(track["start_sec"], 0),
			DurationSec:   compositionFloatValue(track["duration_sec"], 0),
			Volume:        compositionFloatValue(track["volume"], 1),
			FadeInSec:     compositionFloatValue(track["fade_in_sec"], 0),
			FadeOutSec:    compositionFloatValue(track["fade_out_sec"], 0),
		}
		if ducking, ok := track["ducking"].(map[string]any); ok {
			audio.Ducking = timelineDuckingInput{
				SidechainRole: strings.TrimSpace(compositionStringValue(ducking["sidechain_role"])),
				Threshold:     compositionFloatValue(ducking["threshold"], 0.08),
				Ratio:         compositionFloatValue(ducking["ratio"], 8),
				AttackMS:      int(compositionFloatValue(ducking["attack_ms"], 20)),
				ReleaseMS:     int(compositionFloatValue(ducking["release_ms"], 250)),
			}
		}
		if audio.Volume <= 0 {
			audio.Volume = 1
		}
		tracks = append(tracks, audio)
	}
	return tracks, nil
}

func timelineOutputPath(plan map[string]any, fallback string) (string, error) {
	rawOutput, ok := plan["output"].(map[string]any)
	if !ok {
		return fallback, nil
	}
	workspacePath := strings.TrimSpace(compositionStringValue(rawOutput["workspace_path"]))
	if workspacePath == "" {
		return fallback, nil
	}
	if !strings.HasPrefix(path.Clean(workspacePath), "/workspace/") {
		return "", errors.New("plan.output.workspace_path 必须位于 /workspace")
	}
	return workspacePath, nil
}

func timelineFFmpegArgs(plan map[string]any, outputPath string) ([]string, error) {
	segments, err := timelineSegments(plan)
	if err != nil {
		return nil, err
	}
	audioTracks, err := timelineAudioTracks(plan)
	if err != nil {
		return nil, err
	}
	if len(audioTracks) == 0 {
		paths := make([]string, 0, len(segments))
		for _, segment := range segments {
			paths = append(paths, segment.WorkspacePath)
		}
		return concatFFmpegArgs(paths, outputPath), nil
	}
	args := []string{"-y"}
	for _, segment := range segments {
		args = append(args, "-i", segment.WorkspacePath)
	}
	for _, track := range audioTracks {
		if track.Role == "bgm" {
			args = append(args, "-stream_loop", "-1")
		}
		args = append(args, "-i", track.WorkspacePath)
	}
	filter := timelineFilterGraph(segments, audioTracks)
	args = append(args,
		"-filter_complex", filter,
		"-map", "[vout]",
		"-map", "[aout]",
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-shortest",
		outputPath,
	)
	return args, nil
}

func timelineFilterGraph(segments []timelineSegmentInput, audioTracks []timelineAudioInput) string {
	parts := []string{}
	if len(segments) == 1 {
		parts = append(parts, "[0:v]setpts=PTS-STARTPTS,format=yuv420p,setsar=1[vout]")
	} else {
		videoInputs := make([]string, 0, len(segments))
		for index := range segments {
			videoInputs = append(videoInputs, fmt.Sprintf("[%d:v]", index))
		}
		parts = append(parts, strings.Join(videoInputs, "")+fmt.Sprintf("concat=n=%d:v=1:a=0[vcat]", len(segments)))
		parts = append(parts, "[vcat]format=yuv420p,setsar=1[vout]")
	}

	audioLabels := make([]string, 0, len(audioTracks))
	voiceoverLabel := ""
	bgmLabelsNeedingDucking := map[string]timelineDuckingInput{}
	audioInputOffset := len(segments)
	for index, track := range audioTracks {
		label := fmt.Sprintf("a%d", index)
		inputIndex := audioInputOffset + index
		chain := []string{fmt.Sprintf("[%d:a]", inputIndex), "atrim=start=0"}
		if track.DurationSec > 0 {
			chain = append(chain, fmt.Sprintf(":duration=%.3f", track.DurationSec))
		}
		filter := strings.Join(chain, "") + ",asetpts=PTS-STARTPTS"
		if track.StartSec > 0 {
			delayMS := int(track.StartSec * 1000)
			filter += fmt.Sprintf(",adelay=%d|%d", delayMS, delayMS)
		}
		filter += fmt.Sprintf(",volume=%.3f", track.Volume)
		if track.FadeInSec > 0 {
			filter += fmt.Sprintf(",afade=t=in:st=0:d=%.3f", track.FadeInSec)
		}
		if track.FadeOutSec > 0 && track.DurationSec > 0 {
			start := track.DurationSec - track.FadeOutSec
			if start < 0 {
				start = 0
			}
			filter += fmt.Sprintf(",afade=t=out:st=%.3f:d=%.3f", start, track.FadeOutSec)
		}
		filter += fmt.Sprintf("[%s]", label)
		parts = append(parts, filter)
		if track.Role == "voiceover" && voiceoverLabel == "" {
			voiceoverLabel = label
		}
		if track.Role == "bgm" && track.Ducking.SidechainRole == "voiceover" {
			bgmLabelsNeedingDucking[label] = track.Ducking
			continue
		}
		audioLabels = append(audioLabels, label)
	}
	for label, ducking := range bgmLabelsNeedingDucking {
		if voiceoverLabel == "" {
			audioLabels = append(audioLabels, label)
			continue
		}
		out := label + "duck"
		parts = append(parts, fmt.Sprintf("[%s][%s]sidechaincompress=threshold=%.3f:ratio=%.3f:attack=%d:release=%d[%s]", label, voiceoverLabel, ducking.Threshold, ducking.Ratio, ducking.AttackMS, ducking.ReleaseMS, out))
		audioLabels = append(audioLabels, out)
	}
	if len(audioLabels) == 1 {
		parts = append(parts, fmt.Sprintf("[%s]anull[aout]", audioLabels[0]))
	} else {
		inputs := make([]string, 0, len(audioLabels))
		for _, label := range audioLabels {
			inputs = append(inputs, fmt.Sprintf("[%s]", label))
		}
		parts = append(parts, strings.Join(inputs, "")+fmt.Sprintf("amix=inputs=%d:duration=shortest:dropout_transition=0[aout]", len(audioLabels)))
	}
	return strings.Join(parts, ";")
}

func concatFFmpegArgs(segments []string, outputPath string) []string {
	args := []string{"-y"}
	for _, segment := range segments {
		args = append(args, "-i", segment)
	}
	if len(segments) == 1 {
		return append(args, "-c", "copy", outputPath)
	}
	parts := make([]string, 0, len(segments))
	for index := range segments {
		parts = append(parts, fmt.Sprintf("[%d:v][%d:a]", index, index))
	}
	filter := strings.Join(parts, "") + fmt.Sprintf("concat=n=%d:v=1:a=1[v][a]", len(segments))
	return append(args,
		"-filter_complex", filter,
		"-map", "[v]",
		"-map", "[a]",
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		outputPath,
	)
}

func shortToolID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 8 {
		return value[:8]
	}
	return "unknown"
}

func compositionStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func compositionFloatValue(value any, defaultValue float64) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return defaultValue
	}
}

func validateStageMediaInputs(input StageMediaInputsInput) error {
	if len(input.Assets) == 0 {
		return errors.New("assets 至少需要 1 个媒体输入")
	}
	for i, asset := range input.Assets {
		prefix := fmt.Sprintf("assets[%d]", i)
		if _, ok := pgUUIDFromString(asset.AssetID); !ok {
			return fmt.Errorf("%s.asset_id 必须从 get_composition_context 返回的 assets 原样复制", prefix)
		}
		if err := requireText(asset.SourceURL, prefix+".source_url"); err != nil {
			return err
		}
		if err := requireText(asset.FileName, prefix+".file_name"); err != nil {
			return err
		}
	}
	return nil
}

func validateCreateTimelinePlan(input CreateTimelinePlanInput) error {
	if err := requireMode(input.TemplateKey, composerTemplateSimpleConcat, composerTemplateConcatWithFade); err != nil {
		return err
	}
	if input.SourceStoryboardNodeID != "" {
		if _, ok := pgUUIDFromString(input.SourceStoryboardNodeID); !ok {
			return errors.New("source_storyboard_node_id 必须从当前 Composer task 或 get_composition_context 返回值原样复制")
		}
	}
	if len(input.Plan) == 0 {
		return errors.New("plan 必填")
	}
	return nil
}

func validateUpdateTimelinePlanStatus(input UpdateTimelinePlanStatusInput) error {
	if _, ok := pgUUIDFromString(input.TimelinePlanID); !ok {
		return errors.New("timeline_plan_id 必须从 create_timeline_plan 返回结果原样复制")
	}
	return requireMode(input.Status, "draft", "rendering", "completed", "blocked", "failed")
}

func validateRenderTimelineTemplate(input RenderTimelineTemplateInput) error {
	if _, ok := pgUUIDFromString(input.TimelinePlanID); !ok {
		return errors.New("timeline_plan_id 必须从 create_timeline_plan 返回结果原样复制")
	}
	if err := requireMode(input.TemplateKey, composerTemplateSimpleConcat, composerTemplateConcatWithFade); err != nil {
		return err
	}
	if len(input.Plan) == 0 {
		return errors.New("plan 必填")
	}
	return nil
}

func validateRunFFmpegCommand(input RunFFmpegCommandToolInput) error {
	if err := requireMode(input.Executable, "ffmpeg", "ffprobe"); err != nil {
		return err
	}
	if len(input.Args) == 0 {
		return errors.New("args 至少需要 1 个参数")
	}
	return nil
}

func validateSubmitCompositionArtifact(input SubmitCompositionArtifactInput) error {
	if err := requireText(input.OutputPath, "output_path"); err != nil {
		return err
	}
	if strings.TrimSpace(input.OutputNodeID) == "" && strings.TrimSpace(input.TimelinePlanID) == "" {
		return errors.New("output_node_id 或 timeline_plan_id 至少需要一个")
	}
	if strings.TrimSpace(input.OutputNodeID) != "" {
		if _, ok := pgUUIDFromString(input.OutputNodeID); !ok {
			return errors.New("output_node_id 必须从上下文返回值原样复制")
		}
	}
	if strings.TrimSpace(input.TimelinePlanID) != "" {
		if _, ok := pgUUIDFromString(input.TimelinePlanID); !ok {
			return errors.New("timeline_plan_id 必须从 create_timeline_plan 返回结果原样复制")
		}
	}
	return nil
}

func jsonStringResult(toolName string, value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return NaturalToolError(toolName, err.Error(), "请检查工具返回值序列化。"), nil
	}
	return string(raw), nil
}

func defaultComposerJSON(raw []byte) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte("{}")
	}
	return raw
}

func mergeComposerResultJSON(existing []byte, next []byte) []byte {
	merged := map[string]any{}
	_ = json.Unmarshal(defaultComposerJSON(existing), &merged)
	incoming := map[string]any{}
	_ = json.Unmarshal(defaultComposerJSON(next), &incoming)
	for key, value := range incoming {
		merged[key] = value
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return defaultComposerJSON(next)
	}
	return raw
}
