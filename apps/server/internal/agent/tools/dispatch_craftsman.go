package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var errShotNotFound = errors.New("shot reference not found")
var errScopeNotFound = errors.New("craftsman scope not found")

type CraftsmanDispatcherStore interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	GetShotByID(ctx context.Context, id pgtype.UUID) (db.Shot, error)
	GetShotByClientKey(ctx context.Context, params db.GetShotByClientKeyParams) (db.Shot, error)
	GetKeyElementStateByID(ctx context.Context, params db.GetKeyElementStateByIDParams) (db.KeyElementState, error)
	ListActiveKeyElementStatesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.KeyElementState, error)
	GetAudioPlan(ctx context.Context, params db.GetAudioPlanParams) (db.AudioPlan, error)
	GetActiveAudioPlanByWorkspace(ctx context.Context, workspaceID pgtype.UUID) (db.AudioPlan, error)
	SetShotCraftsmanThread(ctx context.Context, params db.SetShotCraftsmanThreadParams) (db.Shot, error)
	UpdateShotStatus(ctx context.Context, params db.UpdateShotStatusParams) (db.Shot, error)
}

type CraftsmanRuntime interface {
	GetOrCreateCraftsmanThread(ctx context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error)
	GetOrCreateCraftsmanThreadForScope(ctx context.Context, workspaceID pgtype.UUID, scopeType string, scopeID pgtype.UUID) (db.AgentThread, error)
	ListActiveAgentTasksByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
}

type CraftsmanTaskEnqueuer interface {
	EnqueueCraftsmanTask(ctx context.Context, task db.AgentTask)
}

type DispatchCraftsmanTool struct {
	store    CraftsmanDispatcherStore
	runtime  CraftsmanRuntime
	enqueuer CraftsmanTaskEnqueuer
}

func NewDispatchCraftsmanTool(store CraftsmanDispatcherStore, runtime CraftsmanRuntime, enqueuer CraftsmanTaskEnqueuer) DispatchCraftsmanTool {
	return DispatchCraftsmanTool{store: store, runtime: runtime, enqueuer: enqueuer}
}

func (t DispatchCraftsmanTool) Definition() Definition {
	return Definition{
		Name:        "dispatch_craftsman",
		Description: "派发 scope-aware 的 Craftsman 生成任务。该工具只创建持久化任务并入队，Craftsman 会继续创建 RenderPlan 并复用生产链路生成参考图、分镜预览图或分镜视频；Producer turn 内不会直接生成媒体。",
		Parameters: objectSchema(map[string]any{
			"brief": map[string]any{
				"type":        "string",
				"description": "一句话描述调用该工具的意图，例如生成机场晨光统一参考图或直接生成所有分镜预览图。",
			},
			"scope": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type": map[string]any{
						"type":        "string",
						"enum":        []string{"shot", "key_element_state", "audio_plan"},
						"description": "生产归属范围。shot 用于分镜图/视频；key_element_state 用于共享参考图；audio_plan 用于全片旁白或 BGM 音频。",
					},
					"id": map[string]any{
						"type":        "string",
						"description": "兼容旧字段。模型不要填写内部 ID；shot/key_element_state 请填写 read_project_context 返回的 semantic_key；audio_plan 请填写 audio_plan.active。",
					},
				},
			},
			"shot_refs": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "scope.type=shot 时的 Shot semantic_key 或稳定 client_key。为空表示所有可派发 active shots。scope.type=key_element_state 时不要填写 shot_refs。",
			},
			"target_phase": map[string]any{
				"type":        "string",
				"enum":        []string{"reference_image", "preview_image", "shot_video", "voiceover_audio", "bgm_audio"},
				"description": "要派发的生成阶段。reference_image 生成共享参考图；preview_image 生成分镜预览图；shot_video 生成分镜视频；voiceover_audio / bgm_audio 基于已批准 AudioPlan 创建音频 RenderPlan。",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"preview_image", "shot_video"},
				"description": "兼容旧参数；新调用必须使用 target_phase。",
			},
			"execution_policy": map[string]any{
				"type":        "string",
				"enum":        []string{"execute_immediately", "wait_for_producer"},
				"description": "execute_immediately 表示 Craftsman 编译 RenderPlan 后自动提交 Worker；wait_for_producer 表示编译后等待 Producer accept/reject。",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "为 true 时，即使已有完成结果也创建新的尝试；默认 false。注意：force 不能绕过正在排队或运行中的同 scope/target_phase Craftsman 任务。",
			},
			"max_attempts": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     3,
				"description": "Craftsman 最大尝试次数，范围 1 到 3；为空时默认 3。",
			},
			"review_record_id": map[string]any{
				"type":        "string",
				"description": "可选，触发本次修订的 review_record_id。",
			},
			"critique": map[string]any{
				"type":        "string",
				"description": "可选，Craftsman 必须回应的评审意见或用户修改意见。",
			},
			"fix_hints": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "可选，具体修复建议，例如保持行李箱银灰色、改成低机位跟拍。",
			},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			UsesProductionService: true,
			MaxCallsPerTurn:       5,
		},
		Visibility: VisibilitySpec{
			ShowCallMessage:   true,
			ShowResultMessage: true,
			UserLabel:         "派发生成任务",
		},
	}
}

func (t DispatchCraftsmanTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.store == nil || t.runtime == nil {
		return ExecuteOutput{}, errors.New("dispatch_craftsman service is not configured")
	}
	args, err := dispatchCraftsmanArgs(input.Arguments)
	if err != nil {
		return ExecuteOutput{}, err
	}
	workspace, err := t.store.GetWorkspaceByID(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	if workspace.Mode != db.WorkspaceModeAgent {
		return ExecuteOutput{}, errors.New("dispatch_craftsman requires an Agent workspace")
	}
	scopes, err := t.resolveScopes(ctx, input.WorkspaceID, args)
	if err != nil {
		return ExecuteOutput{}, err
	}
	if len(scopes) == 0 {
		return ExecuteOutput{}, fmt.Errorf("没有可派发的 Craftsman scope，请读取项目上下文确认状态后重试")
	}

	dispatched := make([]map[string]any, 0, len(scopes))
	skipped := []map[string]any{}
	activeTasks, err := t.runtime.ListActiveAgentTasksByWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	for _, scope := range scopes {
		if active, ok := activeCraftsmanTaskForScopePhase(activeTasks, scope.ScopeType, scope.ScopeID, args.TargetPhase); ok {
			item := map[string]any{
				"scope_type":                scope.ScopeType,
				"scope_key":                 scope.ScopeKey,
				"scope_ref":                 objectLabel(scope.ScopeType, scope.ScopeKey),
				"client_key":                scope.ClientKey,
				"target_phase":              args.TargetPhase,
				"reason":                    "active_craftsman_task_exists",
				"active_craftsman_task_key": active.SemanticKey,
				"active_status":             active.Status,
			}
			if strings.TrimSpace(active.SemanticKey) != "" {
				item["active_craftsman_task_ref"] = objectLabel("agent_task", active.SemanticKey)
			}
			skipped = append(skipped, item)
			continue
		}
		thread, err := t.runtime.GetOrCreateCraftsmanThreadForScope(ctx, input.WorkspaceID, scope.ScopeType, scope.ScopeID)
		if err != nil {
			return ExecuteOutput{}, err
		}
		if scope.ScopeType == "shot" {
			_, _ = t.store.SetShotCraftsmanThread(ctx, db.SetShotCraftsmanThreadParams{
				ID:                scope.ScopeID,
				CraftsmanThreadID: thread.ID,
				WorkspaceID:       input.WorkspaceID,
			})
		}
		taskInput := map[string]any{
			"mode":                   args.TargetPhase,
			"target_phase":           args.TargetPhase,
			"scope_type":             scope.ScopeType,
			"scope_id":               uuidString(scope.ScopeID),
			"scope_key":              scope.ScopeKey,
			"execution_policy":       args.ExecutionPolicy,
			"shot_id":                "",
			"shot_client_key":        "",
			"producer_thread_id":     uuidString(input.ThreadID),
			"producer_task_id":       uuidString(input.TaskID),
			"craftsman_thread_id":    uuidString(thread.ID),
			"requested_max_attempts": args.MaxAttempts,
		}
		if scope.ScopeType == "shot" {
			taskInput["shot_id"] = uuidString(scope.ScopeID)
			taskInput["shot_client_key"] = scope.ClientKey
		}
		if scope.ScopeType == "key_element_state" {
			taskInput["key_element_state_id"] = uuidString(scope.ScopeID)
			taskInput["key_element_state_client_key"] = scope.ClientKey
		}
		if args.Brief != "" {
			taskInput["brief"] = args.Brief
		}
		if args.ParentToolCallID != "" {
			taskInput["parent_tool_call_id"] = args.ParentToolCallID
		}
		if args.ReviewRecordID != "" {
			taskInput["review_record_id"] = args.ReviewRecordID
		}
		if args.Critique != "" {
			taskInput["review_critique"] = args.Critique
		}
		if len(args.FixHints) > 0 {
			taskInput["review_fix_hints"] = args.FixHints
		}
		if len(args.InputNodeRefs) > 0 {
			taskInput["input_node_refs"] = args.InputNodeRefs
		} else if args.TargetPhase == "shot_video" && strings.TrimSpace(scope.ClientKey) != "" {
			taskInput["input_node_refs"] = []string{scope.ClientKey + " preview image"}
		}
		rawInput, err := json.Marshal(taskInput)
		if err != nil {
			return ExecuteOutput{}, err
		}
		task, err := t.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    thread.ID,
			Role:        "craftsman",
			ScopeType:   scope.ScopeType,
			ScopeID:     scope.ScopeID,
			TaskType:    "craftsman_turn",
			MaxAttempts: args.MaxAttempts,
			Input:       rawInput,
		})
		if err != nil {
			return ExecuteOutput{}, err
		}
		if err := t.appendDelegationMessage(ctx, input.WorkspaceID, thread.ID, task.ID, scope, args, taskInput); err != nil {
			return ExecuteOutput{}, err
		}
		_, _ = t.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    thread.ID,
			TaskID:      task.ID,
			EventType:   "craftsman_dispatched",
			SourceRole:  "producer",
			TargetRole:  "craftsman",
			Scope:       mustJSON(map[string]any{"scope_type": scope.ScopeType, "scope_id": uuidString(scope.ScopeID), "scope_key": scope.ScopeKey, "client_key": scope.ClientKey}),
			Payload:     mustJSON(map[string]any{"target_phase": args.TargetPhase, "execution_policy": args.ExecutionPolicy, "max_attempts": args.MaxAttempts}),
		})
		if t.enqueuer != nil {
			t.enqueuer.EnqueueCraftsmanTask(ctx, task)
		}
		item := map[string]any{
			"scope_type":         scope.ScopeType,
			"scope_key":          scope.ScopeKey,
			"scope_ref":          objectLabel(scope.ScopeType, scope.ScopeKey),
			"client_key":         scope.ClientKey,
			"craftsman_task_key": task.SemanticKey,
			"status":             task.Status,
		}
		if strings.TrimSpace(task.SemanticKey) != "" {
			item["craftsman_task_ref"] = objectLabel("agent_task", task.SemanticKey)
		}
		dispatched = append(dispatched, item)
	}
	summary := dispatchCraftsmanSummary(len(dispatched), len(skipped), args.ScopeType, args.TargetPhase, args.ExecutionPolicy)
	return ExecuteOutput{Summary: summary, Result: map[string]any{
		"status":       dispatchStatus(len(dispatched), len(skipped)),
		"target_phase": args.TargetPhase,
		"summary":      summary,
		"dispatched":   dispatched,
		"skipped":      skipped,
	}}, nil
}

func dispatchStatus(dispatched int, skipped int) string {
	if dispatched == 0 && skipped > 0 {
		return "skipped"
	}
	return "queued"
}

func (t DispatchCraftsmanTool) appendDelegationMessage(ctx context.Context, workspaceID pgtype.UUID, threadID pgtype.UUID, taskID pgtype.UUID, scope craftsmanDispatchScope, args parsedDispatchCraftsmanArgs, taskInput map[string]any) error {
	text := craftsmanDelegationText(scope, args)
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
			"schema":       "clipanvil.agent.delegation.v1",
			"target_role":  "craftsman",
			"scope_type":   scope.ScopeType,
			"scope_id":     uuidString(scope.ScopeID),
			"scope_key":    scope.ScopeKey,
			"client_key":   scope.ClientKey,
			"target_phase": args.TargetPhase,
			"task_input":   taskInput,
		}),
		TaskID: taskID,
	})
	return err
}

func craftsmanDelegationText(scope craftsmanDispatchScope, args parsedDispatchCraftsmanArgs) string {
	lines := []string{
		"Producer 派发 Craftsman 任务。",
		"- scope: " + scope.ScopeType + "=" + scope.ScopeKey,
		"- client_key: " + scope.ClientKey,
		"- target_phase: " + args.TargetPhase,
		"- execution_policy: " + args.ExecutionPolicy,
	}
	if args.Brief != "" {
		lines = append(lines, "- brief: "+args.Brief)
	}
	if args.Critique != "" {
		lines = append(lines, "- critique: "+args.Critique)
	}
	if len(args.FixHints) > 0 {
		lines = append(lines, "- fix_hints: "+strings.Join(args.FixHints, "；"))
	}
	if len(args.InputNodeRefs) > 0 {
		lines = append(lines, "- input_node_refs: "+strings.Join(args.InputNodeRefs, "；"))
	}
	return strings.Join(lines, "\n")
}

func dispatchCraftsmanSummary(dispatched int, skipped int, scopeType string, targetPhase string, executionPolicy string) string {
	tail := "Craftsman 会先创建并编译 RenderPlan，随后等待 Producer accept/reject。"
	if executionPolicy == "execute_immediately" {
		tail = "Craftsman 编译 RenderPlan 后，工程会自动提交 Worker 生成任务。"
	}
	if dispatched == 0 && skipped > 0 {
		return fmt.Sprintf("没有创建新的 Craftsman 任务：%d 个 scope 已存在同一 target_phase 的 Craftsman 任务正在排队或运行。请等待对应 signal，或稍后调用 read_project_context 查看真实状态；不要重复派发相同任务。", skipped)
	}
	if scopeType == "key_element_state" {
		return fmt.Sprintf("已将 %d 个关键元素状态参考图 RenderPlan 任务加入队列。%s 当前仅表示 Craftsman 任务已排队，不表示参考图已经生成完成。", dispatched, tail)
	}
	if targetPhase == "shot_video" {
		if skipped > 0 {
			return fmt.Sprintf("已将 %d 个分镜视频 RenderPlan 任务加入队列，%d 个分镜被跳过。%s 当前仅表示 Craftsman 任务已排队，不表示视频已经生成完成。", dispatched, skipped, tail)
		}
		return fmt.Sprintf("已将 %d 个分镜视频 RenderPlan 任务加入队列。%s 当前仅表示 Craftsman 任务已排队，不表示视频已经生成完成。", dispatched, tail)
	}
	if skipped > 0 {
		return fmt.Sprintf("已将 %d 个分镜的预览图 RenderPlan 任务加入队列，%d 个分镜因已有预览或状态不匹配被跳过。%s 当前仅表示 Craftsman 任务已排队，不表示图片已经生成完成。", dispatched, skipped, tail)
	}
	return fmt.Sprintf("已将 %d 个分镜的预览图 RenderPlan 任务加入队列。%s 当前仅表示 Craftsman 任务已排队，不表示图片已经生成完成。", dispatched, tail)
}

func activeCraftsmanTaskForScopePhase(tasks []db.AgentTask, scopeType string, scopeID pgtype.UUID, targetPhase string) (db.AgentTask, bool) {
	for _, task := range tasks {
		if task.Role != "craftsman" || task.TaskType != "craftsman_turn" {
			continue
		}
		if task.ScopeType != scopeType || task.ScopeID != scopeID {
			continue
		}
		var input map[string]any
		if err := json.Unmarshal(task.Input, &input); err != nil {
			continue
		}
		phase := strings.TrimSpace(stringValue(input, "target_phase"))
		if phase == "" {
			phase = strings.TrimSpace(stringValue(input, "mode"))
		}
		if phase == targetPhase {
			return task, true
		}
	}
	return db.AgentTask{}, false
}

type parsedDispatchCraftsmanArgs struct {
	Brief            string
	ScopeType        string
	ScopeID          pgtype.UUID
	ScopeRef         string
	TargetPhase      string
	ExecutionPolicy  string
	ParentToolCallID string
	ShotRefs         []string
	Force            bool
	MaxAttempts      int32
	ReviewRecordID   string
	Critique         string
	FixHints         []string
	InputNodeRefs    []string
}

func dispatchCraftsmanArgs(raw map[string]any) (parsedDispatchCraftsmanArgs, error) {
	targetPhase := stringValue(raw, "target_phase")
	if targetPhase == "" {
		targetPhase = stringValue(raw, "mode")
	}
	if targetPhase == "" {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("invalid dispatch_craftsman target_phase")
	}
	if targetPhase != "reference_image" && targetPhase != "preview_image" && targetPhase != "shot_video" && targetPhase != "voiceover_audio" && targetPhase != "bgm_audio" {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("unsupported dispatch_craftsman target_phase %q", targetPhase)
	}
	executionPolicy := stringValue(raw, "execution_policy")
	if executionPolicy == "" {
		executionPolicy = "wait_for_producer"
	}
	if executionPolicy != "execute_immediately" && executionPolicy != "wait_for_producer" {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("unsupported dispatch_craftsman execution_policy %q", executionPolicy)
	}
	maxAttempts := int32Value(raw, "max_attempts", 3)
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if maxAttempts > 3 {
		maxAttempts = 3
	}
	scopeType := "shot"
	scopeID := pgtype.UUID{}
	if scopeRaw, ok := raw["scope"].(map[string]any); ok {
		if value := strings.TrimSpace(stringValue(scopeRaw, "type")); value != "" {
			scopeType = value
		}
		if id, ok := pgUUIDFromString(stringValue(scopeRaw, "id")); ok {
			scopeID = id
		}
	}
	scopeRef := ""
	if scopeRaw, ok := raw["scope"].(map[string]any); ok {
		scopeRef = strings.TrimSpace(stringValue(scopeRaw, "id"))
	}
	if scopeType != "shot" && scopeType != "key_element_state" && scopeType != "audio_plan" {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("unsupported dispatch_craftsman scope.type %q", scopeType)
	}
	if scopeType == "key_element_state" {
		if !scopeID.Valid && scopeRef == "" {
			return parsedDispatchCraftsmanArgs{}, fmt.Errorf("scope.id is required for key_element_state")
		}
		if targetPhase != "reference_image" {
			return parsedDispatchCraftsmanArgs{}, fmt.Errorf("key_element_state 只能派发 reference_image")
		}
	}
	if scopeType == "shot" && targetPhase == "reference_image" {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("shot 不能派发 reference_image")
	}
	if scopeType == "shot" && (targetPhase == "voiceover_audio" || targetPhase == "bgm_audio") {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("shot 不能派发音频阶段；请使用 scope.type=audio_plan")
	}
	if scopeType == "audio_plan" {
		if targetPhase != "voiceover_audio" && targetPhase != "bgm_audio" {
			return parsedDispatchCraftsmanArgs{}, fmt.Errorf("audio_plan 只能派发 voiceover_audio 或 bgm_audio")
		}
		if len(stringSliceValue(raw, "shot_refs")) > 0 {
			return parsedDispatchCraftsmanArgs{}, fmt.Errorf("audio_plan scope 不支持 shot_refs")
		}
	}
	shotRefs := stringSliceValue(raw, "shot_refs")
	return parsedDispatchCraftsmanArgs{
		Brief:            stringValue(raw, "brief"),
		ScopeType:        scopeType,
		ScopeID:          scopeID,
		ScopeRef:         scopeRef,
		TargetPhase:      targetPhase,
		ExecutionPolicy:  executionPolicy,
		ParentToolCallID: stringValue(raw, "parent_tool_call_id"),
		ShotRefs:         shotRefs,
		Force:            boolValue(raw, "force"),
		MaxAttempts:      maxAttempts,
		ReviewRecordID:   stringValue(raw, "review_record_id"),
		Critique:         stringValue(raw, "critique"),
		FixHints:         stringSliceValue(raw, "fix_hints"),
		InputNodeRefs:    stringSliceValue(raw, "input_node_refs"),
	}, nil
}

type craftsmanDispatchScope struct {
	ScopeType string
	ScopeID   pgtype.UUID
	ScopeKey  string
	ClientKey string
}

func (t DispatchCraftsmanTool) resolveScopes(ctx context.Context, workspaceID pgtype.UUID, args parsedDispatchCraftsmanArgs) ([]craftsmanDispatchScope, error) {
	if args.ScopeType == "key_element_state" {
		state, err := t.resolveKeyElementStateRef(ctx, workspaceID, args)
		if err != nil {
			return nil, err
		}
		if state.WorkspaceID != workspaceID || state.Status != "active" {
			return nil, errScopeNotFound
		}
		if state.ReferenceStatus != "needs_reference" && !args.Force {
			return nil, fmt.Errorf("key_element_state.reference_status=%s，不需要生成参考图；如需重做请设置 force=true", state.ReferenceStatus)
		}
		return []craftsmanDispatchScope{{ScopeType: "key_element_state", ScopeID: state.ID, ScopeKey: semanticScopeKey(state.SemanticKey, "key_element_state", state.ClientKey), ClientKey: state.ClientKey}}, nil
	}
	if args.ScopeType == "audio_plan" {
		audioPlan, err := t.resolveAudioPlanRef(ctx, workspaceID, args)
		if err != nil {
			return nil, err
		}
		if audioPlan.WorkspaceID != workspaceID {
			return nil, errScopeNotFound
		}
		if audioPlan.Status != "approved" {
			return nil, fmt.Errorf("audio_plan.status=%s，必须先批准 approved 后才能派发音频生成", audioPlan.Status)
		}
		return []craftsmanDispatchScope{{ScopeType: "audio_plan", ScopeID: audioPlan.ID, ScopeKey: semanticScopeKey(audioPlan.SemanticKey, "audio_plan", "active"), ClientKey: "audio_plan.active"}}, nil
	}
	shots, err := t.resolveShots(ctx, workspaceID, args)
	if err != nil {
		return nil, err
	}
	out := make([]craftsmanDispatchScope, 0, len(shots))
	for _, shot := range shots {
		out = append(out, craftsmanDispatchScope{ScopeType: "shot", ScopeID: shot.ID, ScopeKey: semanticScopeKey(shot.SemanticKey, "shot", shot.ClientKey), ClientKey: shot.ClientKey})
	}
	return out, nil
}

func semanticScopeKey(semanticKey string, scopeType string, clientKey string) string {
	if value := strings.TrimSpace(semanticKey); value != "" {
		return value
	}
	if value := strings.TrimSpace(clientKey); value != "" {
		return strings.TrimSpace(scopeType) + "." + value
	}
	return strings.TrimSpace(scopeType) + ".semantic_key_missing"
}

func (t DispatchCraftsmanTool) resolveKeyElementStateRef(ctx context.Context, workspaceID pgtype.UUID, args parsedDispatchCraftsmanArgs) (db.KeyElementState, error) {
	if args.ScopeID.Valid {
		return t.store.GetKeyElementStateByID(ctx, db.GetKeyElementStateByIDParams{ID: args.ScopeID, WorkspaceID: workspaceID})
	}
	if strings.TrimSpace(args.ScopeRef) == "" {
		return db.KeyElementState{}, fmt.Errorf("key_element_state scope.id 需要 read_project_context 返回的 semantic_key 或 state_client_key")
	}
	states, err := t.store.ListActiveKeyElementStatesByWorkspace(ctx, workspaceID)
	if err != nil {
		return db.KeyElementState{}, err
	}
	var matched *db.KeyElementState
	for _, state := range states {
		if state.ClientKey != args.ScopeRef && state.SemanticKey != args.ScopeRef {
			continue
		}
		current := state
		if matched != nil {
			return db.KeyElementState{}, fmt.Errorf("state_client_key=%s 不唯一，请改用 key_element_state semantic_key", args.ScopeRef)
		}
		matched = &current
	}
	if matched == nil {
		return db.KeyElementState{}, fmt.Errorf("找不到 key_element_state ref=%s，请先读取项目上下文确认真实 semantic_key", args.ScopeRef)
	}
	return *matched, nil
}

func (t DispatchCraftsmanTool) resolveAudioPlanRef(ctx context.Context, workspaceID pgtype.UUID, args parsedDispatchCraftsmanArgs) (db.AudioPlan, error) {
	if args.ScopeID.Valid {
		return t.store.GetAudioPlan(ctx, db.GetAudioPlanParams{ID: args.ScopeID, WorkspaceID: workspaceID})
	}
	ref := strings.TrimSpace(args.ScopeRef)
	audioPlan, err := t.store.GetActiveAudioPlanByWorkspace(ctx, workspaceID)
	if err != nil {
		return db.AudioPlan{}, err
	}
	if ref == "" || ref == "audio_plan.active" || ref == audioPlan.SemanticKey || ref == audioPlan.DisplayName {
		return audioPlan, nil
	}
	return db.AudioPlan{}, fmt.Errorf("找不到 audio_plan ref=%s，请使用 read_project_context 返回的 audio_plan.active", ref)
}

func (t DispatchCraftsmanTool) resolveShots(ctx context.Context, workspaceID pgtype.UUID, args parsedDispatchCraftsmanArgs) ([]db.Shot, error) {
	if args.ScopeID.Valid {
		shot, err := t.resolveShotRef(ctx, workspaceID, uuidString(args.ScopeID))
		if err != nil {
			return nil, err
		}
		if !shotDispatchableForPhase(shot.Status, args.Force, args.TargetPhase) {
			return nil, nil
		}
		return []db.Shot{shot}, nil
	}
	if ref := strings.TrimSpace(args.ScopeRef); ref != "" {
		shot, err := t.resolveShotRef(ctx, workspaceID, ref)
		if err != nil {
			return nil, err
		}
		if !shotDispatchableForPhase(shot.Status, args.Force, args.TargetPhase) {
			return nil, nil
		}
		return []db.Shot{shot}, nil
	}
	if len(args.ShotRefs) == 0 {
		shots, err := t.store.ListActiveShotsByWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		out := make([]db.Shot, 0, len(shots))
		for _, shot := range shots {
			if shotDispatchableForPhase(shot.Status, args.Force, args.TargetPhase) {
				out = append(out, shot)
			}
		}
		return out, nil
	}
	out := make([]db.Shot, 0, len(args.ShotRefs))
	seen := map[pgtype.UUID]bool{}
	for _, ref := range args.ShotRefs {
		shot, err := t.resolveShotRef(ctx, workspaceID, ref)
		if err != nil {
			return nil, err
		}
		if seen[shot.ID] {
			continue
		}
		if !shotDispatchableForPhase(shot.Status, args.Force, args.TargetPhase) {
			continue
		}
		seen[shot.ID] = true
		out = append(out, shot)
	}
	return out, nil
}

func (t DispatchCraftsmanTool) resolveShotRef(ctx context.Context, workspaceID pgtype.UUID, ref string) (db.Shot, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return db.Shot{}, errShotNotFound
	}
	if id, ok := pgUUIDFromString(ref); ok {
		shot, err := t.store.GetShotByID(ctx, id)
		if err != nil {
			return db.Shot{}, err
		}
		if shot.WorkspaceID != workspaceID {
			return db.Shot{}, errShotNotFound
		}
		return shot, nil
	}
	shots, err := t.store.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err == nil {
		var matched *db.Shot
		for _, shot := range shots {
			if shot.SemanticKey != ref {
				continue
			}
			current := shot
			if matched != nil {
				return db.Shot{}, fmt.Errorf("shot semantic_key=%s 不唯一，请读取项目上下文确认", ref)
			}
			matched = &current
		}
		if matched != nil {
			return *matched, nil
		}
	}
	return t.store.GetShotByClientKey(ctx, db.GetShotByClientKeyParams{
		WorkspaceID: workspaceID,
		ClientKey:   ref,
	})
}

func shotDispatchableForPhase(status string, force bool, targetPhase string) bool {
	if targetPhase == "shot_video" {
		switch strings.TrimSpace(status) {
		case "preview_ready", "failed":
			return true
		case "video_ready", "video_running":
			return force
		default:
			return false
		}
	}
	switch strings.TrimSpace(status) {
	case "planned", "draft", "failed":
		return true
	case "preview_ready":
		return force
	default:
		return false
	}
}

func boolValue(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
