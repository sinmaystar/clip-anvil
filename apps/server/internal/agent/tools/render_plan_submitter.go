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
	GetKeyElementStateByID(ctx context.Context, params db.GetKeyElementStateByIDParams) (db.KeyElementState, error)
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
	input, err := s.workerInputForRenderPlan(ctx, workspaceID, plan)
	if err != nil {
		return db.AgentTask{}, db.RenderPlan{}, err
	}
	threadID := plan.CreatedByThreadID
	if !threadID.Valid {
		threadID = requestedThreadID
	}
	rawInput, err := json.Marshal(input)
	if err != nil {
		return db.AgentTask{}, db.RenderPlan{}, err
	}
	task, err := s.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID:  workspaceID,
		ThreadID:     threadID,
		Role:         "worker",
		ScopeType:    plan.ScopeType,
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
		Scope:       mustJSON(map[string]any{"render_plan_id": uuidString(plan.ID), "scope_type": plan.ScopeType, "scope_id": uuidString(plan.ScopeID)}),
		Payload:     mustJSON(map[string]any{"reason": reason, "target_phase": plan.TargetPhase, "worker_task_id": uuidString(task.ID)}),
	})
	if s.enqueuer != nil {
		s.enqueuer.EnqueueWorkerTask(ctx, task)
	}
	return task, submitted, nil
}

func (s *RenderPlanSubmitter) workerInputForRenderPlan(ctx context.Context, workspaceID pgtype.UUID, plan db.RenderPlan) (agentworker.GenerationInput, error) {
	switch plan.ScopeType {
	case "shot":
		if plan.TargetPhase != "preview_image" && plan.TargetPhase != "shot_video" {
			return agentworker.GenerationInput{}, fmt.Errorf("shot 级 RenderPlan 只支持 preview_image 或 shot_video")
		}
		shot, err := s.store.GetShotByID(ctx, plan.ScopeID)
		if err != nil {
			return agentworker.GenerationInput{}, err
		}
		if shot.WorkspaceID != workspaceID {
			return agentworker.GenerationInput{}, fmt.Errorf("shot 不属于当前 workspace")
		}
		return workerInputForShotRenderPlan(plan, shot), nil
	case "key_element_state":
		if plan.TargetPhase != "reference_image" {
			return agentworker.GenerationInput{}, fmt.Errorf("key_element_state 级 RenderPlan 只支持 reference_image")
		}
		state, err := s.store.GetKeyElementStateByID(ctx, db.GetKeyElementStateByIDParams{ID: plan.ScopeID, WorkspaceID: workspaceID})
		if err != nil {
			return agentworker.GenerationInput{}, err
		}
		return workerInputForKeyElementStateRenderPlan(plan, state), nil
	default:
		return agentworker.GenerationInput{}, fmt.Errorf("不支持提交 %s 级 RenderPlan", plan.ScopeType)
	}
}

func workerInputForShotRenderPlan(plan db.RenderPlan, shot db.Shot) agentworker.GenerationInput {
	params := map[string]any{}
	_ = json.Unmarshal(defaultJSON(plan.Params), &params)
	return agentworker.GenerationInput{
		Mode:              plan.TargetPhase,
		TargetPhase:       plan.TargetPhase,
		ScopeType:         plan.ScopeType,
		ScopeID:           uuidString(plan.ScopeID),
		ScopeKey:          shot.SemanticKey,
		RenderPlanKey:     plan.SemanticKey,
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

func workerInputForKeyElementStateRenderPlan(plan db.RenderPlan, state db.KeyElementState) agentworker.GenerationInput {
	params := map[string]any{}
	_ = json.Unmarshal(defaultJSON(plan.Params), &params)
	return agentworker.GenerationInput{
		Mode:                     plan.TargetPhase,
		TargetPhase:              plan.TargetPhase,
		ScopeType:                plan.ScopeType,
		ScopeID:                  uuidString(plan.ScopeID),
		ScopeKey:                 state.SemanticKey,
		RenderPlanKey:            plan.SemanticKey,
		KeyElementStateClientKey: state.ClientKey,
		CraftsmanThreadID:        uuidString(plan.CreatedByThreadID),
		CraftsmanTaskID:          uuidString(plan.CreatedByTaskID),
		Strategy:                 strings.TrimSpace(plan.Rationale),
		Prompt:                   strings.TrimSpace(plan.CompiledPrompt),
		InputNodeRefs:            renderPlanInputNodeRefs(plan.ReferenceBindings),
		OutputType:               outputTypeForRenderPlan(plan),
		OperationType:            plan.Operation,
		Model:                    modelForRenderPlan(plan),
		Params:                   params,
		MaxAttempts:              3,
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
		sourceType := strings.TrimSpace(ref.SourceType)
		if (sourceType == "media_node" || sourceType == "shot_output") && strings.TrimSpace(ref.SourceID) != "" {
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
