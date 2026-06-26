package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agentworker "github.com/sinmaystar/clip-anvil/internal/agent/worker"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type RenderPlanSubmitStore interface {
	GetRenderPlanByID(ctx context.Context, params db.GetRenderPlanByIDParams) (db.RenderPlan, error)
	GetShotByID(ctx context.Context, id pgtype.UUID) (db.Shot, error)
	MarkRenderPlanSubmitted(ctx context.Context, params db.MarkRenderPlanSubmittedParams) (db.RenderPlan, error)
}

type RenderPlanSubmitRuntime interface {
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type WorkerTaskEnqueuer interface {
	EnqueueWorkerTask(ctx context.Context, task db.AgentTask)
}

type RenderPlanSubmitter struct {
	store    RenderPlanSubmitStore
	runtime  RenderPlanSubmitRuntime
	enqueuer WorkerTaskEnqueuer
}

func NewRenderPlanSubmitter(store RenderPlanSubmitStore, runtime RenderPlanSubmitRuntime, enqueuer WorkerTaskEnqueuer) *RenderPlanSubmitter {
	return &RenderPlanSubmitter{store: store, runtime: runtime, enqueuer: enqueuer}
}

func (s *RenderPlanSubmitter) SubmitRenderPlan(ctx context.Context, workspaceID pgtype.UUID, renderPlanID pgtype.UUID, requestedThreadID pgtype.UUID, reason string) (db.AgentTask, db.RenderPlan, error) {
	if s == nil || s.store == nil || s.runtime == nil || !workspaceID.Valid || !renderPlanID.Valid {
		return db.AgentTask{}, db.RenderPlan{}, fmt.Errorf("render plan submitter is not configured")
	}
	plan, err := s.store.GetRenderPlanByID(ctx, db.GetRenderPlanByIDParams{ID: renderPlanID, WorkspaceID: workspaceID})
	if err != nil {
		return db.AgentTask{}, db.RenderPlan{}, err
	}
	if plan.ScopeType != "shot" || !plan.ScopeID.Valid {
		return db.AgentTask{}, db.RenderPlan{}, fmt.Errorf("只支持提交 shot 级 RenderPlan")
	}
	if plan.TargetPhase != "preview_image" && plan.TargetPhase != "shot_video" {
		return db.AgentTask{}, db.RenderPlan{}, fmt.Errorf("只支持提交 preview_image 或 shot_video RenderPlan")
	}
	shot, err := s.store.GetShotByID(ctx, plan.ScopeID)
	if err != nil {
		return db.AgentTask{}, db.RenderPlan{}, err
	}
	threadID := plan.CreatedByThreadID
	if !threadID.Valid {
		threadID = requestedThreadID
	}
	input := workerInputForRenderPlan(plan, shot)
	rawInput, err := json.Marshal(input)
	if err != nil {
		return db.AgentTask{}, db.RenderPlan{}, err
	}
	task, err := s.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID:  workspaceID,
		ThreadID:     threadID,
		Role:         "worker",
		ScopeType:    "shot",
		ScopeID:      plan.ScopeID,
		TaskType:     "worker_generation",
		MaxAttempts:  3,
		Input:        rawInput,
		RenderPlanID: plan.ID,
	})
	if err != nil {
		return db.AgentTask{}, db.RenderPlan{}, err
	}
	submitted, err := s.store.MarkRenderPlanSubmitted(ctx, db.MarkRenderPlanSubmittedParams{
		ID:                    plan.ID,
		WorkspaceID:           workspaceID,
		SubmittedWorkerTaskID: task.ID,
		OutputNodeID:          pgtype.UUID{},
	})
	if err != nil {
		return db.AgentTask{}, db.RenderPlan{}, err
	}
	_, _ = s.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: workspaceID,
		ThreadID:    threadID,
		TaskID:      task.ID,
		EventType:   "render_plan_submitted",
		SourceRole:  "producer",
		TargetRole:  "worker",
		Scope:       mustJSON(map[string]any{"render_plan_id": uuidString(plan.ID), "shot_id": uuidString(plan.ScopeID)}),
		Payload:     mustJSON(map[string]any{"reason": reason, "target_phase": plan.TargetPhase, "worker_task_id": uuidString(task.ID)}),
	})
	if s.enqueuer != nil {
		s.enqueuer.EnqueueWorkerTask(ctx, task)
	}
	return task, submitted, nil
}

func workerInputForRenderPlan(plan db.RenderPlan, shot db.Shot) agentworker.GenerationInput {
	params := map[string]any{}
	_ = json.Unmarshal(defaultJSON(plan.Params), &params)
	return agentworker.GenerationInput{
		Mode:              plan.TargetPhase,
		TargetPhase:       plan.TargetPhase,
		ShotID:            uuidString(plan.ScopeID),
		ShotClientKey:     shot.ClientKey,
		ShotSortOrder:     int(shot.SortOrder),
		CraftsmanThreadID: uuidString(plan.CreatedByThreadID),
		CraftsmanTaskID:   uuidString(plan.CreatedByTaskID),
		Strategy:          strings.TrimSpace(plan.Rationale),
		Prompt:            strings.TrimSpace(plan.CompiledPrompt),
		InputNodeRefs:     renderPlanInputNodeRefs(plan.ReferenceBindings),
		OutputType:        outputTypeForRenderPlan(plan),
		OperationType:     plan.Operation,
		Model:             modelForRenderPlan(plan),
		Params:            params,
		MaxAttempts:       3,
	}
}

func outputTypeForRenderPlan(plan db.RenderPlan) string {
	if plan.TargetPhase == "shot_video" {
		return "video"
	}
	return "image"
}

func modelForRenderPlan(plan db.RenderPlan) agentworker.ModelSpec {
	return agentworker.ModelSpec{}
}

type renderPlanReferenceBinding struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
}

func renderPlanInputNodeRefs(raw []byte) []string {
	var refs []renderPlanReferenceBinding
	if err := json.Unmarshal(defaultJSON(raw), &refs); err != nil {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.SourceType == "media_node" && strings.TrimSpace(ref.SourceID) != "" {
			out = append(out, strings.TrimSpace(ref.SourceID))
		}
	}
	return out
}

func defaultJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}
