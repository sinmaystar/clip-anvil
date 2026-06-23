package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var errShotNotFound = errors.New("shot reference not found")

type CraftsmanDispatcherStore interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	GetShotByID(ctx context.Context, id pgtype.UUID) (db.Shot, error)
	GetShotByClientKey(ctx context.Context, params db.GetShotByClientKeyParams) (db.Shot, error)
	SetShotCraftsmanThread(ctx context.Context, params db.SetShotCraftsmanThreadParams) (db.Shot, error)
}

type CraftsmanRuntime interface {
	GetOrCreateCraftsmanThread(ctx context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
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
		Description: "Dispatch shot-scoped preview generation work. This creates persistent shot execution tasks and reuses the production generation pipeline; it does not generate images directly inside the Producer turn.",
		Parameters: objectSchema(map[string]any{
			"shot_refs": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Shot UUIDs or stable client keys such as shot-01. Empty means all active planned shots.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"preview_image"},
				"description": "Production phase to dispatch. M6.6 only supports preview_image.",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "When true, create a new preview attempt even if a shot already has preview output.",
			},
			"max_attempts": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     3,
				"description": "Fixed retry cap. Defaults to 3.",
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
			UserLabel:         "开始生成预览图",
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
	shots, err := t.resolveShots(ctx, input.WorkspaceID, args)
	if err != nil {
		return ExecuteOutput{}, err
	}

	dispatched := make([]map[string]any, 0, len(shots))
	skipped := []map[string]any{}
	for _, shot := range shots {
		thread, err := t.runtime.GetOrCreateCraftsmanThread(ctx, input.WorkspaceID, shot.ID)
		if err != nil {
			return ExecuteOutput{}, err
		}
		_, _ = t.store.SetShotCraftsmanThread(ctx, db.SetShotCraftsmanThreadParams{
			ID:                shot.ID,
			CraftsmanThreadID: thread.ID,
			WorkspaceID:       input.WorkspaceID,
		})
		taskInput := map[string]any{
			"mode":                   args.Mode,
			"shot_id":                uuidString(shot.ID),
			"shot_client_key":        shot.ClientKey,
			"producer_thread_id":     uuidString(input.ThreadID),
			"producer_task_id":       uuidString(input.TaskID),
			"craftsman_thread_id":    uuidString(thread.ID),
			"requested_max_attempts": args.MaxAttempts,
		}
		rawInput, err := json.Marshal(taskInput)
		if err != nil {
			return ExecuteOutput{}, err
		}
		task, err := t.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    thread.ID,
			Role:        "craftsman",
			ScopeType:   "shot",
			ScopeID:     shot.ID,
			TaskType:    "craftsman_turn",
			MaxAttempts: args.MaxAttempts,
			Input:       rawInput,
		})
		if err != nil {
			return ExecuteOutput{}, err
		}
		_, _ = t.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    thread.ID,
			TaskID:      task.ID,
			EventType:   "craftsman_dispatched",
			SourceRole:  "producer",
			TargetRole:  "craftsman",
			Scope:       mustJSON(map[string]any{"shot_id": uuidString(shot.ID), "client_key": shot.ClientKey}),
			Payload:     mustJSON(map[string]any{"mode": args.Mode, "max_attempts": args.MaxAttempts}),
		})
		if t.enqueuer != nil {
			t.enqueuer.EnqueueCraftsmanTask(ctx, task)
		}
		dispatched = append(dispatched, map[string]any{
			"shot_id":             uuidString(shot.ID),
			"client_key":          shot.ClientKey,
			"craftsman_thread_id": uuidString(thread.ID),
			"craftsman_task_id":   uuidString(task.ID),
			"status":              task.Status,
		})
	}
	summary := dispatchCraftsmanSummary(len(dispatched), len(skipped), args.Mode)
	return ExecuteOutput{Summary: summary, Result: map[string]any{
		"status":     "queued",
		"mode":       args.Mode,
		"summary":    summary,
		"dispatched": dispatched,
		"skipped":    skipped,
	}}, nil
}

func dispatchCraftsmanSummary(dispatched int, skipped int, mode string) string {
	if mode != "preview_image" {
		return fmt.Sprintf("已调度 %d 个分镜任务，%d 个分镜被跳过。", dispatched, skipped)
	}
	if skipped > 0 {
		return fmt.Sprintf("已将 %d 个分镜的预览图生成任务加入队列，%d 个分镜因已有预览或状态不匹配被跳过。预览图会由后台 Craftsman/Worker 继续生成；节点和生成状态会通过画布同步与生产状态查询更新。当前仅表示任务已排队，不表示图片已经生成完成。", dispatched, skipped)
	}
	return fmt.Sprintf("已将 %d 个分镜的预览图生成任务加入队列。预览图会由后台 Craftsman/Worker 继续生成；节点和生成状态会通过画布同步与生产状态查询更新。当前仅表示任务已排队，不表示图片已经生成完成。", dispatched)
}

type parsedDispatchCraftsmanArgs struct {
	Mode        string
	ShotRefs    []string
	Force       bool
	MaxAttempts int32
}

func dispatchCraftsmanArgs(raw map[string]any) (parsedDispatchCraftsmanArgs, error) {
	mode := stringValue(raw, "mode")
	if mode == "" {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("invalid dispatch_craftsman mode")
	}
	if mode != "preview_image" {
		return parsedDispatchCraftsmanArgs{}, fmt.Errorf("unsupported dispatch_craftsman mode %q", mode)
	}
	maxAttempts := int32Value(raw, "max_attempts", 3)
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if maxAttempts > 3 {
		maxAttempts = 3
	}
	shotRefs := stringSliceValue(raw, "shot_refs")
	return parsedDispatchCraftsmanArgs{
		Mode:        mode,
		ShotRefs:    shotRefs,
		Force:       boolValue(raw, "force"),
		MaxAttempts: maxAttempts,
	}, nil
}

func (t DispatchCraftsmanTool) resolveShots(ctx context.Context, workspaceID pgtype.UUID, args parsedDispatchCraftsmanArgs) ([]db.Shot, error) {
	if len(args.ShotRefs) == 0 {
		shots, err := t.store.ListActiveShotsByWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		out := make([]db.Shot, 0, len(shots))
		for _, shot := range shots {
			if shotDispatchable(shot.Status, args.Force) {
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
		if !shotDispatchable(shot.Status, args.Force) {
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

func shotDispatchable(status string, force bool) bool {
	switch strings.TrimSpace(status) {
	case "planned", "draft", "failed":
		return true
	case "preview_ready":
		return force
	default:
		return false
	}
}

func pgUUIDFromString(value string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
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
