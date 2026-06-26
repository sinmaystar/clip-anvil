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
	SetShotCraftsmanThread(ctx context.Context, params db.SetShotCraftsmanThreadParams) (db.Shot, error)
	UpdateShotStatus(ctx context.Context, params db.UpdateShotStatusParams) (db.Shot, error)
}

type CraftsmanRuntime interface {
	GetOrCreateCraftsmanThread(ctx context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error)
	GetOrCreateCraftsmanThreadForScope(ctx context.Context, workspaceID pgtype.UUID, scopeType string, scopeID pgtype.UUID) (db.AgentThread, error)
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
						"enum":        []string{"shot", "key_element_state"},
						"description": "生产归属范围。shot 用于分镜图/视频；key_element_state 用于共享参考图。",
					},
					"id": map[string]any{
						"type":        "string",
						"description": "scope 对象 UUID。key_element_state 必填；shot 可为空并使用 shot_refs 批量派发。",
					},
				},
			},
			"shot_refs": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "scope.type=shot 时的 Shot UUID 或稳定 client_key。为空表示所有可派发 active shots。",
			},
			"target_phase": map[string]any{
				"type":        "string",
				"enum":        []string{"reference_image", "preview_image", "shot_video"},
				"description": "要派发的生成阶段。reference_image 生成共享参考图；preview_image 生成分镜预览图；shot_video 生成分镜视频。",
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
				"description": "为 true 时，即使已有结果也创建新的尝试；默认 false。",
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
	for _, scope := range scopes {
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
			Scope:       mustJSON(map[string]any{"scope_type": scope.ScopeType, "scope_id": uuidString(scope.ScopeID), "client_key": scope.ClientKey}),
			Payload:     mustJSON(map[string]any{"target_phase": args.TargetPhase, "execution_policy": args.ExecutionPolicy, "max_attempts": args.MaxAttempts}),
		})
		if t.enqueuer != nil {
			t.enqueuer.EnqueueCraftsmanTask(ctx, task)
		}
		dispatched = append(dispatched, map[string]any{
			"scope_type":          scope.ScopeType,
			"scope_id":            uuidString(scope.ScopeID),
			"client_key":          scope.ClientKey,
			"craftsman_thread_id": uuidString(thread.ID),
			"craftsman_task_id":   uuidString(task.ID),
			"status":              task.Status,
		})
	}
	summary := dispatchCraftsmanSummary(len(dispatched), len(skipped), args.ScopeType, args.TargetPhase, args.ExecutionPolicy)
	return ExecuteOutput{Summary: summary, Result: map[string]any{
		"status":       "queued",
		"target_phase": args.TargetPhase,
		"summary":      summary,
		"dispatched":   dispatched,
		"skipped":      skipped,
	}}, nil
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
		"- scope: " + scope.ScopeType + "=" + uuidString(scope.ScopeID),
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

type parsedDispatchCraftsmanArgs struct {
	Brief            string
	ScopeType        string
	ScopeID          pgtype.UUID
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
	if targetPhase != "reference_image" && targetPhase != "preview_image" && targetPhase != "shot_video" {
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
	if scopeType != "shot" && scopeType != "key_element_state" {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("unsupported dispatch_craftsman scope.type %q", scopeType)
	}
	if scopeType == "key_element_state" {
		if !scopeID.Valid {
			return parsedDispatchCraftsmanArgs{}, fmt.Errorf("scope.id is required for key_element_state")
		}
		if targetPhase != "reference_image" {
			return parsedDispatchCraftsmanArgs{}, fmt.Errorf("key_element_state 只能派发 reference_image")
		}
	}
	if scopeType == "shot" && targetPhase == "reference_image" {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("shot 不能派发 reference_image")
	}
	shotRefs := stringSliceValue(raw, "shot_refs")
	return parsedDispatchCraftsmanArgs{
		Brief:            stringValue(raw, "brief"),
		ScopeType:        scopeType,
		ScopeID:          scopeID,
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
	ClientKey string
}

func (t DispatchCraftsmanTool) resolveScopes(ctx context.Context, workspaceID pgtype.UUID, args parsedDispatchCraftsmanArgs) ([]craftsmanDispatchScope, error) {
	if args.ScopeType == "key_element_state" {
		state, err := t.store.GetKeyElementStateByID(ctx, db.GetKeyElementStateByIDParams{ID: args.ScopeID, WorkspaceID: workspaceID})
		if err != nil {
			return nil, err
		}
		if state.WorkspaceID != workspaceID || state.Status != "active" {
			return nil, errScopeNotFound
		}
		if state.ReferenceStatus != "needs_reference" && !args.Force {
			return nil, fmt.Errorf("key_element_state.reference_status=%s，不需要生成参考图；如需重做请设置 force=true", state.ReferenceStatus)
		}
		return []craftsmanDispatchScope{{ScopeType: "key_element_state", ScopeID: state.ID, ClientKey: state.ClientKey}}, nil
	}
	shots, err := t.resolveShots(ctx, workspaceID, args)
	if err != nil {
		return nil, err
	}
	out := make([]craftsmanDispatchScope, 0, len(shots))
	for _, shot := range shots {
		out = append(out, craftsmanDispatchScope{ScopeType: "shot", ScopeID: shot.ID, ClientKey: shot.ClientKey})
	}
	return out, nil
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
