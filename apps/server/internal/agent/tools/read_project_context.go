package tools

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/creative"
	agentidentity "github.com/sinmaystar/clip-anvil/internal/agent/identity"
	agentpss "github.com/sinmaystar/clip-anvil/internal/agent/pss"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ProjectContextPSSBuilder interface {
	BuildProducerPSS(ctx context.Context, workspaceID pgtype.UUID) (agentpss.ProducerPSS, error)
}

type ReadProjectContextNativeTool struct {
	service    *creative.Service
	pssBuilder ProjectContextPSSBuilder
}

type ReadProjectContextToolInput struct {
	Brief       string        `json:"brief" jsonschema:"required" jsonschema_description:"本次读取上下文的目的，例如判断是否需要创建 brief、memory、关键元素或 storyboard。不要超过 160 个中文字符。"`
	ScopeRef    ToolObjectRef `json:"scope_ref" jsonschema_description:"可选读取范围。为空表示整个 workspace；局部读取时填写 read_project_context 返回的 semantic_key，例如 type=shot,key=shot_03。不要填写 UUID、shot_id、node_id 或 artifact_version_id。"`
	Include     []string      `json:"include" jsonschema_description:"要返回的对象类型。可选值包括 brief、memory、elements、scenes、shots、dependencies、render_plans、object_index、canvas_projection、production_state。production_state 会返回生产状态总览，包括媒体节点、生成任务、artifact、review、issue、pending decision 和 running task；只在需要生产视图时请求，避免无谓撑大上下文。为空时返回 Producer 默认上下文。"`
	DetailLevel string        `json:"detail_level" jsonschema:"enum=summary,enum=full" jsonschema_description:"summary 返回摘要和可操作语义索引，适合普通规划；full 返回更完整事实，适合写入前决策。默认 summary。"`
}

func NewReadProjectContextNativeTool(service *creative.Service, builders ...ProjectContextPSSBuilder) *ReadProjectContextNativeTool {
	var builder ProjectContextPSSBuilder
	if len(builders) > 0 {
		builder = builders[0]
	}
	return &ReadProjectContextNativeTool{service: service, pssBuilder: builder}
}

func (t *ReadProjectContextNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[ReadProjectContextToolInput](toolReadProjectContext, "读取当前 ClipAnvil Agent workspace 的创作事实源和按需生产状态，用于 Producer 在行动前理解 CreativeBrief、ProjectMemory、KeyElement、KeyElementState、Scene、Shot、shot_key_element、shot_dependency、RenderPlan、媒体节点、生成任务、review、issue 和只读画布投影。")
}

func (t *ReadProjectContextNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolReadProjectContext, argumentsInJSON, validateReadProjectContextInput)
	if !ok {
		return msg, nil
	}
	if msg, ok := serviceOrError(t.service, toolReadProjectContext); !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolReadProjectContext)
	if !ok {
		return msg, nil
	}
	packet, err := t.service.ReadProjectContext(ctx, creative.ReadContextInput{
		WorkspaceID: runtime.WorkspaceID,
		ScopeType:   "workspace",
		Include:     input.Include,
		DetailLevel: input.DetailLevel,
	})
	if err != nil {
		return naturalErrorFromErr(toolReadProjectContext, err), nil
	}
	scopeNote := ""
	if completeObjectRef(input.ScopeRef) {
		object, ok := findObjectByRef(packet.ObjectIndex, input.ScopeRef)
		if !ok {
			scopeNote = fmt.Sprintf("scope_ref %s/%s 未匹配到 ObjectIndex，已返回 workspace 上下文。", input.ScopeRef.Type, input.ScopeRef.Key)
		} else if object.ObjectType == agentidentity.ObjectShot {
			packet, err = t.service.ReadProjectContext(ctx, creative.ReadContextInput{
				WorkspaceID: runtime.WorkspaceID,
				ScopeType:   object.ObjectType,
				ScopeID:     uuidString(object.ObjectID),
				Include:     input.Include,
				DetailLevel: input.DetailLevel,
			})
			if err != nil {
				return naturalErrorFromErr(toolReadProjectContext, err), nil
			}
		} else {
			scopeNote = fmt.Sprintf("scope_ref %s/%s 当前按 workspace 上下文返回；只有 shot 支持局部过滤。", input.ScopeRef.Type, input.ScopeRef.Key)
		}
	} else if hasObjectRef(input.ScopeRef) {
		scopeNote = "scope_ref 不完整，已忽略并返回 workspace 上下文。局部读取请同时填写 type 和 key。"
	}
	missingReferences := 0
	for _, state := range packet.ElementStates {
		if state.ReferenceStatus == "needs_reference" {
			missingReferences++
		}
	}
	briefStatus := "没有 active CreativeBrief"
	if packet.Brief != nil {
		briefStatus = packet.Brief.Title + " / " + packet.Brief.Status
	}
	memoryStatus := "没有 active ProjectMemory"
	if packet.Memory != nil {
		memoryStatus = fmt.Sprintf("v%d / %s", packet.Memory.Version, packet.Memory.Status)
	}
	renderPlanStatus := summarizeRenderPlans(packet.RenderPlans)
	items := []NaturalResultItem{
		{Label: "CreativeBrief", Value: briefStatus},
		{Label: "ProjectMemory", Value: memoryStatus},
		{Label: "关键元素", Value: fmt.Sprintf("%d 个，缺少参考 %d 个", len(packet.Elements), missingReferences)},
		{Label: "Storyboard", Value: fmt.Sprintf("%d 个场景，%d 个分镜，%d 个依赖", len(packet.Scenes), len(packet.Shots), len(packet.Dependencies))},
		{Label: "RenderPlan", Value: renderPlanStatus},
		{Label: "ObjectIndex", Value: agentidentity.RenderObjectIndex(packet.ObjectIndex)},
	}
	if includeProductionState(input.Include) {
		if t.pssBuilder == nil {
			items = append(items, NaturalResultItem{Label: "ProductionState", Value: "未配置生产状态读取器。请不要重复调用；如必须读取生产状态，请检查服务端工具 wiring。"})
		} else {
			pss, err := t.pssBuilder.BuildProducerPSS(ctx, runtime.WorkspaceID)
			if err != nil {
				return NaturalToolError(toolReadProjectContext, err.Error(), "读取生产状态失败。请稍后重试，或先基于 CreativeBrief/ProjectMemory/Storyboard 做决策。"), nil
			}
			items = append(items, NaturalResultItem{Label: "ProductionState", Value: strings.TrimSpace(pss.Text + "\n" + productionStateDecisionText(pss.Structured))})
		}
	}
	if scopeNote != "" {
		items = append(items, NaturalResultItem{Label: "ScopeRef", Value: scopeNote})
	}
	return NaturalResult{
		Title: "已读取项目创作上下文",
		Items: items,
		Next:  "如果存在 waiting_for_approval RenderPlan，Producer 应调用 decide_render_plan accept/reject，或先派 Reviewer；否则根据缺失对象选择创作状态工具。",
	}.String(), nil
}

func productionStateDecisionText(state map[string]any) string {
	shots := anySlice(state["shots"])
	if len(shots) == 0 {
		return ""
	}
	lines := []string{"生产状态语义引用："}
	for _, item := range shots {
		shot := anyMap(item)
		clientKey := stringAny(shot["client_key"])
		if clientKey == "" {
			continue
		}
		for _, previewItem := range anySlice(shot["preview_nodes"]) {
			preview := anyMap(previewItem)
			lines = append(lines, fmt.Sprintf("- PreviewArtifact: ref=%s.preview_image.current version_status=%s", clientKey, stringAny(preview["version_status"])))
		}
		shotVideoState := stringAny(shot["shot_video_state"])
		if shotVideoState != "" {
			lines = append(lines, fmt.Sprintf("- ShotVideoState: ref=%s.shot_video.current state=%s", clientKey, shotVideoState))
		}
		for _, videoItem := range anySlice(shot["shot_video_nodes"]) {
			video := anyMap(videoItem)
			lines = append(lines, fmt.Sprintf("- ShotVideoArtifact: ref=%s.shot_video.current state=%s version_status=%s job_status=%s", clientKey, stringAny(video["state"]), stringAny(video["version_status"]), stringAny(video["job_status"])))
		}
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func anySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func anyMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func stringAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func includeProductionState(include []string) bool {
	for _, item := range include {
		switch strings.TrimSpace(item) {
		case "production_state", "canvas_projection":
			return true
		}
	}
	return false
}

func hasObjectRef(ref ToolObjectRef) bool {
	return strings.TrimSpace(ref.Type) != "" || strings.TrimSpace(ref.Key) != ""
}

func completeObjectRef(ref ToolObjectRef) bool {
	return strings.TrimSpace(ref.Type) != "" && strings.TrimSpace(ref.Key) != ""
}

func findObjectByRef(rows []db.AgentObjectIndex, ref ToolObjectRef) (db.AgentObjectIndex, bool) {
	objectType := strings.TrimSpace(ref.Type)
	key := strings.TrimSpace(ref.Key)
	for _, row := range rows {
		if row.ObjectType == objectType && row.SemanticKey == key {
			return row, true
		}
	}
	return db.AgentObjectIndex{}, false
}

func summarizeRenderPlans(plans []db.RenderPlan) string {
	if len(plans) == 0 {
		return "0 个"
	}
	byStatus := map[string]int{}
	byPhaseStatus := map[string]map[string]int{}
	pending := make([]string, 0, 5)
	for _, plan := range plans {
		byStatus[plan.Status]++
		phase := strings.TrimSpace(plan.TargetPhase)
		if phase == "" {
			phase = "unknown_phase"
		}
		if byPhaseStatus[phase] == nil {
			byPhaseStatus[phase] = map[string]int{}
		}
		byPhaseStatus[phase][plan.Status]++
		if plan.Status == "waiting_for_approval" && len(pending) < 5 {
			ref := strings.TrimSpace(plan.SemanticKey)
			if ref == "" {
				ref = strings.TrimSpace(plan.RenderPlanKey)
			}
			if ref == "" {
				ref = "semantic_key_missing"
			}
			pending = append(pending, fmt.Sprintf("%s phase=%s operation=%s", ref, plan.TargetPhase, plan.Operation))
		}
	}
	parts := []string{fmt.Sprintf("%d 个", len(plans))}
	for _, phase := range []string{"reference_image", "preview_image", "shot_video", "final_video", "unknown_phase"} {
		counts := byPhaseStatus[phase]
		if len(counts) == 0 {
			continue
		}
		phaseParts := []string{}
		for _, status := range []string{"waiting_for_approval", "compiled", "submitted", "running", "succeeded", "failed", "rejected", "blocked", "draft"} {
			if count := counts[status]; count > 0 {
				phaseParts = append(phaseParts, fmt.Sprintf("%s=%d", status, count))
			}
		}
		if len(phaseParts) > 0 {
			parts = append(parts, fmt.Sprintf("%s: %s", phase, strings.Join(phaseParts, " ")))
		}
	}
	for _, status := range []string{"waiting_for_approval", "compiled", "submitted", "running", "succeeded", "failed", "rejected", "blocked", "draft"} {
		if count := byStatus[status]; count > 0 {
			parts = append(parts, fmt.Sprintf("total_%s=%d", status, count))
		}
	}
	if len(pending) > 0 {
		parts = append(parts, "待决策："+strings.Join(pending, "；"))
	}
	return strings.Join(parts, "，")
}

func validateReadProjectContextInput(input ReadProjectContextToolInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if completeObjectRef(input.ScopeRef) {
		if err := validateObjectRef(input.ScopeRef, "scope_ref"); err != nil {
			return err
		}
		if err := requireMode(input.ScopeRef.Type, "creative_brief", "project_memory", "scene", "shot", "key_element", "key_element_state", "render_plan", "media_node", "artifact_version", "review_record", "artifact_issue"); err != nil {
			return err
		}
	}
	if input.DetailLevel != "" {
		return requireMode(input.DetailLevel, "summary", "full")
	}
	return nil
}
