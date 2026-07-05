package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/sandbox"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const (
	toolCreateRemotionRendererAttempt   = "create_remotion_renderer_attempt"
	toolValidateRemotionRendererAttempt = "validate_remotion_renderer_attempt"
	toolRenderAgentRemotionRenderer     = "render_agent_remotion_renderer"
)

type RemotionRendererStore interface {
	GetTimelinePlan(ctx context.Context, id pgtype.UUID) (db.TimelinePlan, error)
	CreateRemotionRendererArtifact(ctx context.Context, arg db.CreateRemotionRendererArtifactParams) (db.RemotionRendererArtifact, error)
	GetRemotionRendererArtifact(ctx context.Context, id pgtype.UUID) (db.RemotionRendererArtifact, error)
	CreateRemotionRendererAttempt(ctx context.Context, arg db.CreateRemotionRendererAttemptParams) (db.RemotionRendererAttempt, error)
	GetRemotionRendererAttempt(ctx context.Context, id pgtype.UUID) (db.RemotionRendererAttempt, error)
	UpdateRemotionRendererAttemptSnapshot(ctx context.Context, arg db.UpdateRemotionRendererAttemptSnapshotParams) (db.RemotionRendererAttempt, error)
	UpdateRemotionRendererAttemptRenderResult(ctx context.Context, arg db.UpdateRemotionRendererAttemptRenderResultParams) (db.RemotionRendererAttempt, error)
	SetCurrentRemotionRendererAttempt(ctx context.Context, arg db.SetCurrentRemotionRendererAttemptParams) (db.RemotionRendererArtifact, error)
}

type CreateRemotionRendererAttemptNativeTool struct {
	store   RemotionRendererStore
	manager SandboxEnsurer
	client  sandbox.Client
}

type ValidateRemotionRendererAttemptNativeTool struct {
	store   RemotionRendererStore
	manager SandboxEnsurer
	client  sandbox.Client
}

type RenderAgentRemotionRendererNativeTool struct {
	store    RemotionRendererStore
	renderer RemotionRendererSandbox
}

type RemotionRendererSandbox interface {
	RenderAgentRemotionCode(ctx context.Context, input sandbox.RenderAgentRemotionCodeInput) (sandbox.SandboxJobResult, error)
}

type CreateRemotionRendererAttemptInput struct {
	TimelinePlanID      string            `json:"timeline_plan_id" jsonschema:"required" jsonschema_description:"timeline_plan 内部 ID。必须从 create_timeline_plan 返回结果原样复制。"`
	RendererArtifactID  string            `json:"renderer_artifact_id,omitempty" jsonschema_description:"可选。修复已有 renderer 时必须填写 create_remotion_renderer_attempt 返回的 renderer_artifact_id；首次创建留空。"`
	AttemptNo           int32             `json:"attempt_no" jsonschema:"required" jsonschema_description:"attempt 序号，从 1 开始；修复同一个 renderer_artifact_id 时递增。"`
	RoutePolicy         map[string]any    `json:"route_policy,omitempty" jsonschema_description:"动态 route 选择理由和 fallback policy，会写入 renderer artifact。"`
	Summary             string            `json:"summary,omitempty" jsonschema_description:"本次动态 renderer 的简短说明。"`
	Files               map[string]string `json:"files" jsonschema:"required" jsonschema_description:"renderer 源码文件，必须包含 GeneratedComposition.tsx；路径必须是安全相对路径；Remotion 媒体 src 必须使用 staticFile('input/foo.ext') 等 public-relative 路径，不能直接传 /workspace/input/foo.ext。"`
	Props               map[string]any    `json:"props" jsonschema:"required" jsonschema_description:"写入 props.json 的渲染 props；必须包含 output；staged 媒体可记录 /workspace/input 路径，但 React 组件使用时需转换为 staticFile('input/...')。"`
	RepairFromAttemptID string            `json:"repair_from_attempt_id,omitempty" jsonschema_description:"可选。当前 attempt 修复自哪个旧 attempt。"`
	RepairNotes         string            `json:"repair_notes,omitempty" jsonschema_description:"可选。修复说明。"`
}

type ValidateRemotionRendererAttemptInput struct {
	RendererAttemptID string `json:"renderer_attempt_id" jsonschema:"required" jsonschema_description:"remotion_renderer_attempt 内部 ID。"`
}

type RenderAgentRemotionRendererInput struct {
	RendererAttemptID string `json:"renderer_attempt_id" jsonschema:"required" jsonschema_description:"已经 validate passed 的 remotion_renderer_attempt 内部 ID。"`
	OutputPath        string `json:"output_path,omitempty" jsonschema_description:"可选输出路径，必须位于 /workspace/output；为空时自动生成。"`
}

func NewCreateRemotionRendererAttemptNativeTool(store RemotionRendererStore, manager SandboxEnsurer, client sandbox.Client) *CreateRemotionRendererAttemptNativeTool {
	return &CreateRemotionRendererAttemptNativeTool{store: store, manager: manager, client: client}
}

func NewValidateRemotionRendererAttemptNativeTool(store RemotionRendererStore, manager SandboxEnsurer, client sandbox.Client) *ValidateRemotionRendererAttemptNativeTool {
	return &ValidateRemotionRendererAttemptNativeTool{store: store, manager: manager, client: client}
}

func NewRenderAgentRemotionRendererNativeTool(store RemotionRendererStore, renderer RemotionRendererSandbox) *RenderAgentRemotionRendererNativeTool {
	return &RenderAgentRemotionRendererNativeTool{store: store, renderer: renderer}
}

func (t *CreateRemotionRendererAttemptNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[CreateRemotionRendererAttemptInput](
		toolCreateRemotionRendererAttempt,
		"创建 Agent-authored Remotion renderer artifact/attempt，写入 sandbox attempt 工作区，并保存初始代码快照。",
	)
}

func (t *CreateRemotionRendererAttemptNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolCreateRemotionRendererAttempt, raw, validateCreateRemotionRendererAttempt)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolCreateRemotionRendererAttempt)
	if !ok {
		return msg, nil
	}
	if t.store == nil || t.manager == nil || t.client == nil {
		return NaturalToolError(toolCreateRemotionRendererAttempt, "remotion renderer attempt tool 未配置。", "请检查 Composer graph wiring。"), nil
	}
	planID, _ := pgUUIDFromString(input.TimelinePlanID)
	plan, err := t.store.GetTimelinePlan(ctx, planID)
	if err != nil {
		return NaturalToolError(toolCreateRemotionRendererAttempt, err.Error(), "请确认 timeline_plan_id 存在。"), nil
	}
	if plan.WorkspaceID != runtime.WorkspaceID {
		return NaturalToolError(toolCreateRemotionRendererAttempt, "timeline_plan 不属于当前 workspace。", "请使用当前 Composer task 创建的 timeline_plan_id。"), nil
	}
	artifact, err := t.resolveOrCreateRemotionRendererArtifact(ctx, runtime, plan, input)
	if err != nil {
		return NaturalToolError(toolCreateRemotionRendererAttempt, err.Error(), "请确认 renderer_artifact_id 和 route_policy 可写。"), nil
	}
	workspaceSandbox, err := t.manager.EnsureSandbox(ctx, runtime.WorkspaceID)
	if err != nil {
		return NaturalToolError(toolCreateRemotionRendererAttempt, err.Error(), "请确认 workspace sandbox 可用。"), nil
	}
	propsJSON, _ := json.Marshal(input.Props)
	snapshot, err := sandbox.WriteAgentRemotionAttemptWorkspace(ctx, t.client, workspaceSandbox.SandboxID, sandbox.AgentRemotionWorkspaceInput{
		RendererArtifactID: uuidString(artifact.ID),
		AttemptNo:          input.AttemptNo,
		Files:              input.Files,
		PropsJSON:          propsJSON,
	})
	if err != nil {
		return NaturalToolError(toolCreateRemotionRendererAttempt, err.Error(), "请检查 renderer 文件路径、数量、大小和 props。"), nil
	}
	attempt, err := t.createRemotionRendererAttempt(ctx, runtime, plan, artifact, input, snapshot)
	if err != nil {
		return NaturalToolError(toolCreateRemotionRendererAttempt, err.Error(), "请确认 attempt_no 未重复。"), nil
	}
	return jsonStringResult(toolCreateRemotionRendererAttempt, map[string]any{
		"renderer_artifact_id": uuidString(artifact.ID),
		"renderer_attempt_id":  uuidString(attempt.ID),
		"timeline_plan_id":     uuidString(plan.ID),
		"attempt_no":           attempt.AttemptNo,
		"workspace_dir":        attempt.WorkspaceDir,
		"source_hash":          attempt.SourceHash,
		"props_hash":           attempt.PropsHash,
		"status":               attempt.Status,
	})
}

func (t *ValidateRemotionRendererAttemptNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[ValidateRemotionRendererAttemptInput](
		toolValidateRemotionRendererAttempt,
		"读取 sandbox attempt 工作区，执行 Agent-authored Remotion 静态校验，并把 snapshot/hash/validation result 固化到 DB。",
	)
}

func (t *ValidateRemotionRendererAttemptNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolValidateRemotionRendererAttempt, raw, validateRemotionRendererAttemptInput)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolValidateRemotionRendererAttempt)
	if !ok {
		return msg, nil
	}
	if t.store == nil || t.manager == nil || t.client == nil {
		return NaturalToolError(toolValidateRemotionRendererAttempt, "remotion renderer validation tool 未配置。", "请检查 Composer graph wiring。"), nil
	}
	attemptID, _ := pgUUIDFromString(input.RendererAttemptID)
	attempt, err := t.store.GetRemotionRendererAttempt(ctx, attemptID)
	if err != nil {
		return NaturalToolError(toolValidateRemotionRendererAttempt, err.Error(), "请确认 renderer_attempt_id 存在。"), nil
	}
	if attempt.WorkspaceID != runtime.WorkspaceID {
		return NaturalToolError(toolValidateRemotionRendererAttempt, "renderer attempt 不属于当前 workspace。", "请使用当前 Composer task 创建的 renderer_attempt_id。"), nil
	}
	workspaceSandbox, err := t.manager.EnsureSandbox(ctx, runtime.WorkspaceID)
	if err != nil {
		return NaturalToolError(toolValidateRemotionRendererAttempt, err.Error(), "请确认 workspace sandbox 可用。"), nil
	}
	snapshot, err := sandbox.ReadAgentRemotionAttemptWorkspace(ctx, t.client, workspaceSandbox.SandboxID, attempt.WorkspaceDir)
	if err != nil {
		return NaturalToolError(toolValidateRemotionRendererAttempt, err.Error(), "请用 read_file/edit_file 检查并修复 attempt 工作区。"), nil
	}
	validation := sandbox.ValidateAgentRemotionSnapshot(snapshot)
	updated, err := sandbox.PersistAgentRemotionValidation(ctx, t.store, attempt.ID, snapshot, validation)
	if err != nil {
		return NaturalToolError(toolValidateRemotionRendererAttempt, err.Error(), "请确认 renderer attempt snapshot 可写。"), nil
	}
	return jsonStringResult(toolValidateRemotionRendererAttempt, map[string]any{
		"renderer_artifact_id": uuidString(updated.RendererArtifactID),
		"renderer_attempt_id":  uuidString(updated.ID),
		"timeline_plan_id":     uuidString(updated.TimelinePlanID),
		"status":               updated.Status,
		"passed":               validation.Passed,
		"workspace_dir":        updated.WorkspaceDir,
		"source_hash":          updated.SourceHash,
		"props_hash":           updated.PropsHash,
		"errors":               validation.Errors,
		"warnings":             validation.Warnings,
	})
}

func (t *RenderAgentRemotionRendererNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[RenderAgentRemotionRendererInput](
		toolRenderAgentRemotionRenderer,
		"渲染已通过 validation 的 Agent-authored Remotion attempt，并记录 render_result / sandbox_job_id。",
	)
}

func (t *RenderAgentRemotionRendererNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolRenderAgentRemotionRenderer, raw, validateRenderAgentRemotionRendererInput)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolRenderAgentRemotionRenderer)
	if !ok {
		return msg, nil
	}
	if t.store == nil || t.renderer == nil {
		return NaturalToolError(toolRenderAgentRemotionRenderer, "remotion renderer render tool 未配置。", "请检查 Composer graph wiring。"), nil
	}
	attemptID, _ := pgUUIDFromString(input.RendererAttemptID)
	attempt, err := t.store.GetRemotionRendererAttempt(ctx, attemptID)
	if err != nil {
		return NaturalToolError(toolRenderAgentRemotionRenderer, err.Error(), "请确认 renderer_attempt_id 存在。"), nil
	}
	if attempt.WorkspaceID != runtime.WorkspaceID {
		return NaturalToolError(toolRenderAgentRemotionRenderer, "renderer attempt 不属于当前 workspace。", "请使用当前 Composer task 创建的 renderer_attempt_id。"), nil
	}
	if attempt.Status != "validated" {
		return NaturalToolError(toolRenderAgentRemotionRenderer, "renderer attempt 必须先 validate passed 才能 render。", "请先调用 validate_remotion_renderer_attempt；失败时用 read_file/edit_file 修复后重新 validate。"), nil
	}
	outputPath, err := normalizeAgentRemotionOutputPath(input.OutputPath, attempt.TimelinePlanID)
	if err != nil {
		return NaturalToolError(toolRenderAgentRemotionRenderer, err.Error(), "请使用 /workspace/output 下的 mp4 输出路径。"), nil
	}
	result, err := t.renderer.RenderAgentRemotionCode(ctx, sandbox.RenderAgentRemotionCodeInput{
		WorkspaceID:         attempt.WorkspaceID,
		TargetNodeID:        runtime.ScopeID,
		TimelinePlanID:      attempt.TimelinePlanID,
		RendererArtifactID:  attempt.RendererArtifactID,
		RendererAttemptID:   attempt.ID,
		AttemptWorkspaceDir: attempt.WorkspaceDir,
		OutputPath:          outputPath,
	})
	if err != nil {
		return NaturalToolError(toolRenderAgentRemotionRenderer, err.Error(), "请读取 sandbox stderr，修复 renderer code 后创建新 attempt。"), nil
	}
	renderResult := normalizeAgentRemotionRenderResult(result.Job.Output, outputPath, result)
	updated, err := t.store.UpdateRemotionRendererAttemptRenderResult(ctx, db.UpdateRemotionRendererAttemptRenderResultParams{
		Status:       "rendered",
		RenderResult: renderResult,
		SandboxJobID: result.Job.ID,
		ID:           attempt.ID,
	})
	if err != nil {
		return NaturalToolError(toolRenderAgentRemotionRenderer, err.Error(), "请确认 renderer attempt render_result 可写。"), nil
	}
	if _, err := t.store.SetCurrentRemotionRendererAttempt(ctx, db.SetCurrentRemotionRendererAttemptParams{
		CurrentAttemptID: updated.ID,
		Status:           "rendered",
		ID:               updated.RendererArtifactID,
	}); err != nil {
		return NaturalToolError(toolRenderAgentRemotionRenderer, err.Error(), "请确认 renderer artifact current_attempt 可更新。"), nil
	}
	var renderResultMap map[string]any
	_ = json.Unmarshal(renderResult, &renderResultMap)
	resultForTimelinePlan := map[string]any{
		"template_key":          composerTemplateAgentRemotion,
		"renderer_artifact_id":  uuidString(updated.RendererArtifactID),
		"renderer_attempt_id":   uuidString(updated.ID),
		"renderer_attempt_no":   updated.AttemptNo,
		"sandbox_job_id":        uuidString(result.Job.ID),
		"output_path":           outputPath,
		"render_result":         renderResultMap,
		"fallback_template_key": composerTemplateRemotionV1,
		"fallback_available":    true,
	}
	return jsonStringResult(toolRenderAgentRemotionRenderer, map[string]any{
		"renderer_artifact_id":     uuidString(updated.RendererArtifactID),
		"renderer_attempt_id":      uuidString(updated.ID),
		"timeline_plan_id":         uuidString(updated.TimelinePlanID),
		"status":                   updated.Status,
		"output_path":              outputPath,
		"sandbox_job_id":           uuidString(result.Job.ID),
		"result_for_timeline_plan": resultForTimelinePlan,
	})
}

func (t *CreateRemotionRendererAttemptNativeTool) resolveOrCreateRemotionRendererArtifact(ctx context.Context, runtime NativeRuntimeContext, plan db.TimelinePlan, input CreateRemotionRendererAttemptInput) (db.RemotionRendererArtifact, error) {
	if strings.TrimSpace(input.RendererArtifactID) != "" {
		artifactID, ok := pgUUIDFromString(input.RendererArtifactID)
		if !ok {
			return db.RemotionRendererArtifact{}, errors.New("renderer_artifact_id 必须从 create_remotion_renderer_attempt 返回结果原样复制")
		}
		return t.store.GetRemotionRendererArtifact(ctx, artifactID)
	}
	routePolicyJSON, _ := json.Marshal(input.RoutePolicy)
	return t.store.CreateRemotionRendererArtifact(ctx, db.CreateRemotionRendererArtifactParams{
		WorkspaceID:     runtime.WorkspaceID,
		TimelinePlanID:  plan.ID,
		Status:          "draft",
		RoutePolicy:     defaultComposerJSON(routePolicyJSON),
		Summary:         strings.TrimSpace(input.Summary),
		CreatedByRole:   "composer",
		CreatedByTaskID: runtime.TaskID,
	})
}

func (t *CreateRemotionRendererAttemptNativeTool) createRemotionRendererAttempt(ctx context.Context, runtime NativeRuntimeContext, plan db.TimelinePlan, artifact db.RemotionRendererArtifact, input CreateRemotionRendererAttemptInput, snapshot sandbox.AgentRemotionSnapshot) (db.RemotionRendererAttempt, error) {
	sourceSnapshotJSON, err := json.Marshal(snapshot.SourceSnapshot)
	if err != nil {
		return db.RemotionRendererAttempt{}, err
	}
	notRunJSON, _ := json.Marshal(map[string]string{"status": "not_run"})
	emptyJSON := []byte("{}")
	repairFromAttemptID := pgtype.UUID{}
	if strings.TrimSpace(input.RepairFromAttemptID) != "" {
		parsed, ok := pgUUIDFromString(input.RepairFromAttemptID)
		if !ok {
			return db.RemotionRendererAttempt{}, errors.New("repair_from_attempt_id 必须从 renderer attempt 返回结果原样复制")
		}
		repairFromAttemptID = parsed
	}
	return t.store.CreateRemotionRendererAttempt(ctx, db.CreateRemotionRendererAttemptParams{
		WorkspaceID:         runtime.WorkspaceID,
		TimelinePlanID:      plan.ID,
		RendererArtifactID:  artifact.ID,
		AttemptNo:           input.AttemptNo,
		Status:              "draft",
		SourceSnapshot:      sourceSnapshotJSON,
		PropsJson:           append([]byte(nil), snapshot.PropsJSON...),
		SourceHash:          snapshot.SourceHash,
		PropsHash:           snapshot.PropsHash,
		WorkspaceDir:        snapshot.WorkspaceDir,
		ValidationResult:    notRunJSON,
		CompileResult:       notRunJSON,
		RenderResult:        emptyJSON,
		QaResult:            emptyJSON,
		RepairFromAttemptID: repairFromAttemptID,
		RepairNotes:         strings.TrimSpace(input.RepairNotes),
	})
}

func validateCreateRemotionRendererAttempt(input CreateRemotionRendererAttemptInput) error {
	if _, ok := pgUUIDFromString(input.TimelinePlanID); !ok {
		return errors.New("timeline_plan_id 必须从 create_timeline_plan 返回结果原样复制")
	}
	if strings.TrimSpace(input.RendererArtifactID) != "" {
		if _, ok := pgUUIDFromString(input.RendererArtifactID); !ok {
			return errors.New("renderer_artifact_id 必须从 create_remotion_renderer_attempt 返回结果原样复制")
		}
	}
	if input.AttemptNo <= 0 {
		return errors.New("attempt_no 必须大于 0")
	}
	if len(input.Files) == 0 {
		return errors.New("files 必填")
	}
	if len(input.Props) == 0 {
		return errors.New("props 必填")
	}
	if strings.TrimSpace(input.RepairFromAttemptID) != "" {
		if _, ok := pgUUIDFromString(input.RepairFromAttemptID); !ok {
			return errors.New("repair_from_attempt_id 必须从 renderer attempt 返回结果原样复制")
		}
	}
	return nil
}

func validateRemotionRendererAttemptInput(input ValidateRemotionRendererAttemptInput) error {
	if _, ok := pgUUIDFromString(input.RendererAttemptID); !ok {
		return errors.New("renderer_attempt_id 必须从 create_remotion_renderer_attempt 返回结果原样复制")
	}
	return nil
}

func validateRenderAgentRemotionRendererInput(input RenderAgentRemotionRendererInput) error {
	if _, ok := pgUUIDFromString(input.RendererAttemptID); !ok {
		return errors.New("renderer_attempt_id 必须从 validate_remotion_renderer_attempt 返回结果原样复制")
	}
	if strings.TrimSpace(input.OutputPath) != "" {
		if _, err := sandbox.ValidateOutputPath(input.OutputPath); err != nil {
			return err
		}
	}
	return nil
}

func normalizeAgentRemotionOutputPath(outputPath string, timelinePlanID pgtype.UUID) (string, error) {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		id := strings.ReplaceAll(uuidString(timelinePlanID), "-", "")
		if len(id) > 8 {
			id = id[:8]
		}
		outputPath = "/workspace/output/final-agent-" + id + ".mp4"
	}
	return sandbox.ValidateOutputPath(outputPath)
}

func normalizeAgentRemotionRenderResult(raw []byte, outputPath string, result sandbox.SandboxJobResult) []byte {
	payload := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	if _, ok := payload["output_path"]; !ok {
		payload["output_path"] = outputPath
	}
	if _, ok := payload["mime"]; !ok && result.MIME != "" {
		payload["mime"] = result.MIME
	}
	if _, ok := payload["size_bytes"]; !ok && result.Size > 0 {
		payload["size_bytes"] = result.Size
	}
	payload["sandbox_job_id"] = uuidString(result.Job.ID)
	out, _ := json.Marshal(payload)
	return out
}
