package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	agentidentity "github.com/sinmaystar/clip-anvil/internal/agent/identity"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type DispatchComposerNativeTool struct {
	runtime  ComposeRuntime
	enqueuer ComposerTaskEnqueuer
	resolver ComposerSourceResolver
}

type ComposeRuntime interface {
	GetOrCreateComposerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
}

type ComposerTaskEnqueuer interface {
	EnqueueComposerTask(ctx context.Context, task db.AgentTask)
}

type ComposerSourceResolver interface {
	GetAgentObjectBySemanticKey(ctx context.Context, params db.GetAgentObjectBySemanticKeyParams) (db.AgentObjectIndex, error)
}

type DispatchComposerInput struct {
	SourceStoryboardRef    ToolObjectRef `json:"source_storyboard_ref" jsonschema_description:"需要成片的来源媒体/故事板节点语义引用。使用 read_project_context 返回的 type=media_node,key=...。"`
	SourceStoryboardNodeID string        `json:"source_storyboard_node_id" jsonschema_description:"兼容旧字段：内部 ID 或历史语义键。模型不要填写；请优先使用 source_storyboard_ref。"`
	Instructions           string        `json:"instructions" jsonschema:"required" jsonschema_description:"给 Composer 的成片说明，例如拼接策略、节奏、淡入淡出要求。"`
	TemplateKey            string        `json:"template_key,omitempty" jsonschema:"enum=simple_concat,enum=concat_with_fades,enum=remotion_timeline_v1" jsonschema_description:"可选 timeline 模版，默认 simple_concat；低成本 Remotion final composer 使用 remotion_timeline_v1。"`
}

func NewDispatchComposerNativeTool(runtime ComposeRuntime, enqueuer ComposerTaskEnqueuer, resolver ...ComposerSourceResolver) DispatchComposerNativeTool {
	var selected ComposerSourceResolver
	if len(resolver) > 0 {
		selected = resolver[0]
	}
	return DispatchComposerNativeTool{runtime: runtime, enqueuer: enqueuer, resolver: selected}
}

func (t DispatchComposerNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[DispatchComposerInput](
		"dispatch_composer",
		"把 Producer 确认的最终成片任务派发给 Composer。工具只创建持久化任务并入队，不直接执行 ffmpeg，也不表示最终视频已经完成。",
	)
}

func (t DispatchComposerNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs("dispatch_composer", raw, validateDispatchComposerInput)
	if !ok {
		return msg, nil
	}
	runtimeContext, msg, ok := runtimeOrError(ctx, "dispatch_composer")
	if !ok {
		return msg, nil
	}
	if t.runtime == nil {
		return NaturalToolError("dispatch_composer", "composer runtime 未配置。", "请检查服务端 wiring 后重试。"), nil
	}
	thread, err := t.runtime.GetOrCreateComposerThread(ctx, runtimeContext.WorkspaceID)
	if err != nil {
		return NaturalToolError("dispatch_composer", err.Error(), "请确认 Agent runtime 可用后重试。"), nil
	}
	sourceID, sourceKey, resolveErr := t.resolveSourceStoryboard(ctx, runtimeContext.WorkspaceID, input)
	if resolveErr != nil {
		return NaturalToolError("dispatch_composer", resolveErr.Error(), "请读取项目上下文，使用 ObjectIndex 中真实存在的 source_storyboard_ref。"), nil
	}
	templateKey := strings.TrimSpace(input.TemplateKey)
	if templateKey == "" {
		templateKey = composerTemplateSimpleConcat
	}
	taskInput := map[string]any{
		"source_storyboard_ref":     ensureToolObjectRef(input.SourceStoryboardRef, agentidentity.ObjectMediaNode, sourceKey),
		"source_storyboard_node_id": uuidString(sourceID),
		"instructions":              strings.TrimSpace(input.Instructions),
		"template_key":              templateKey,
		"producer_thread_id":        uuidString(runtimeContext.ThreadID),
		"producer_task_id":          uuidString(runtimeContext.TaskID),
		"parent_tool_call_id":       runtimeContext.ToolCallID,
	}
	rawInput, err := json.Marshal(taskInput)
	if err != nil {
		return NaturalToolError("dispatch_composer", err.Error(), "请检查 Composer task input 序列化。"), nil
	}
	task, err := t.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: runtimeContext.WorkspaceID,
		ThreadID:    thread.ID,
		Role:        "composer",
		ScopeType:   "final_output",
		ScopeID:     sourceID,
		TaskType:    "composer_turn",
		MaxAttempts: 1,
		Input:       rawInput,
	})
	if err != nil {
		return NaturalToolError("dispatch_composer", err.Error(), "请确认 Composer task 可创建后重试。"), nil
	}
	if err := t.appendDelegationMessage(ctx, runtimeContext.WorkspaceID, thread.ID, task.ID, sourceID, sourceKey, taskInput); err != nil {
		return NaturalToolError("dispatch_composer", err.Error(), "请确认 Composer thread message 可写入后重试。"), nil
	}
	_, _ = t.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: runtimeContext.WorkspaceID,
		ThreadID:    thread.ID,
		TaskID:      task.ID,
		EventType:   "composer_dispatched",
		SourceRole:  "producer",
		TargetRole:  "composer",
		Payload:     mustJSON(taskInput),
	})
	if t.enqueuer != nil {
		t.enqueuer.EnqueueComposerTask(ctx, task)
	}
	items := []NaturalResultItem{
		{Label: "状态", Value: "queued"},
		{Label: "模版", Value: templateKey},
		{Label: "source_ref", Value: objectLabel(agentidentity.ObjectMediaNode, sourceKey)},
	}
	if strings.TrimSpace(task.SemanticKey) != "" {
		items = append(items, NaturalResultItem{Label: "task_ref", Value: objectLabel("agent_task", task.SemanticKey)})
	}
	return NaturalResult{
		Title: "Composer 派发结果",
		Items: items,
		Next:  "Composer 任务已入队。请等待 composition_completed、composition_blocked 或 composition_failed signal，不要把派发结果当作最终视频已完成。",
	}.String(), nil
}

func (t DispatchComposerNativeTool) appendDelegationMessage(ctx context.Context, workspaceID pgtype.UUID, threadID pgtype.UUID, taskID pgtype.UUID, sourceID pgtype.UUID, sourceKey string, taskInput map[string]any) error {
	text := composerDelegationText(sourceID, sourceKey, taskInput)
	content, err := uimessage.BuildUserMessageContent(uimessage.UserMessageInput{Text: text})
	if err != nil {
		return err
	}
	_, err = t.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: workspaceID,
		ThreadID:    threadID,
		Role:        "user",
		MessageType: "text",
		Content:     content,
		RawMessage: mustJSON(map[string]any{
			"schema":      "clipanvil.agent.delegation.v1",
			"target_role": "composer",
			"scope_type":  "final_output",
			"scope_id":    uuidString(sourceID),
			"scope_key":   sourceKey,
			"task_input":  taskInput,
		}),
		TaskID: taskID,
	})
	return err
}

func composerDelegationText(sourceID pgtype.UUID, sourceKey string, taskInput map[string]any) string {
	lines := []string{
		"Producer 派发 Composer 任务。",
		"- scope: final_output",
	}
	if strings.TrimSpace(sourceKey) != "" {
		lines = append(lines, "- source_ref: "+objectLabel(agentidentity.ObjectMediaNode, sourceKey))
	} else {
		lines = append(lines, "- source_node_id: "+uuidString(sourceID))
	}
	if templateKey, _ := taskInput["template_key"].(string); strings.TrimSpace(templateKey) != "" {
		lines = append(lines, "- template: "+strings.TrimSpace(templateKey))
	}
	if instructions, _ := taskInput["instructions"].(string); strings.TrimSpace(instructions) != "" {
		lines = append(lines, "- instructions: "+strings.TrimSpace(instructions))
	}
	return strings.Join(lines, "\n")
}

func validateDispatchComposerInput(input DispatchComposerInput) error {
	if strings.TrimSpace(input.SourceStoryboardRef.Key) != "" {
		if err := validateObjectRef(input.SourceStoryboardRef, "source_storyboard_ref"); err != nil {
			return err
		}
		if strings.TrimSpace(input.SourceStoryboardRef.Type) != agentidentity.ObjectMediaNode {
			return fmt.Errorf("source_storyboard_ref.type 必须是 media_node")
		}
	} else if strings.TrimSpace(input.SourceStoryboardNodeID) == "" {
		return fmt.Errorf("source_storyboard_ref 必填，请使用 read_project_context 返回的 media_node semantic_key")
	}
	if err := requireText(input.Instructions, "instructions"); err != nil {
		return err
	}
	if strings.TrimSpace(input.TemplateKey) != "" {
		return requireMode(input.TemplateKey, composerTemplateSimpleConcat, composerTemplateConcatWithFade, composerTemplateRemotionV1)
	}
	return nil
}

func (t DispatchComposerNativeTool) resolveSourceStoryboard(ctx context.Context, workspaceID pgtype.UUID, input DispatchComposerInput) (pgtype.UUID, string, error) {
	if id, ok := pgUUIDFromString(input.SourceStoryboardNodeID); ok && strings.TrimSpace(input.SourceStoryboardRef.Key) == "" {
		return id, "", nil
	}
	key := strings.TrimSpace(input.SourceStoryboardRef.Key)
	if key == "" {
		key = strings.TrimSpace(input.SourceStoryboardNodeID)
	}
	if key == "" {
		return pgtype.UUID{}, "", fmt.Errorf("source_storyboard_ref 必填，请使用 read_project_context 返回的 media_node semantic_key")
	}
	if t.resolver == nil {
		return pgtype.UUID{}, "", fmt.Errorf("source_storyboard_ref 需要 semantic resolver；请检查 dispatch_composer wiring")
	}
	object, err := t.resolver.GetAgentObjectBySemanticKey(ctx, db.GetAgentObjectBySemanticKeyParams{
		WorkspaceID: workspaceID,
		ObjectType:  agentidentity.ObjectMediaNode,
		SemanticKey: key,
	})
	if err != nil {
		return pgtype.UUID{}, "", fmt.Errorf("找不到 media_node/%s，请先读取项目上下文确认真实 semantic_key", key)
	}
	return object.ObjectID, object.SemanticKey, nil
}
