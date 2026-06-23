package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ReviewShotStore interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	GetShotByID(ctx context.Context, id pgtype.UUID) (db.Shot, error)
	GetShotByClientKey(ctx context.Context, params db.GetShotByClientKeyParams) (db.Shot, error)
	ListMediaNodesByShot(ctx context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
}

type ReviewRuntime interface {
	GetOrCreateReviewerThread(ctx context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type ReviewerTaskEnqueuer interface {
	EnqueueReviewerTask(ctx context.Context, task db.AgentTask)
}

type ReviewShotTool struct {
	store    ReviewShotStore
	runtime  ReviewRuntime
	enqueuer ReviewerTaskEnqueuer
}

func NewReviewShotTool(store ReviewShotStore, runtime ReviewRuntime, enqueuer ReviewerTaskEnqueuer) ReviewShotTool {
	return ReviewShotTool{store: store, runtime: runtime, enqueuer: enqueuer}
}

func (t ReviewShotTool) Definition() Definition {
	return Definition{
		Name:        "review_shot",
		Description: "Review generated preview images or shot videos for storyboard shots. This creates persistent ReviewerGraph tasks for current artifact winners; it does not generate new media directly.",
		Parameters: objectSchema(map[string]any{
			"shot_refs":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"target_phase": map[string]any{"type": "string", "enum": []string{"preview_image", "shot_video"}},
			"max_attempts": map[string]any{"type": "integer", "minimum": 1, "maximum": 3},
			"auto_retry":   map[string]any{"type": "boolean"},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			UsesProductionService: true,
			MaxCallsPerTurn:       5,
		},
		Visibility: VisibilitySpec{ShowCallMessage: true, ShowResultMessage: true, UserLabel: "评审分镜产物"},
	}
}

func (t ReviewShotTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.store == nil || t.runtime == nil {
		return ExecuteOutput{}, errors.New("review_shot service is not configured")
	}
	args, err := reviewShotArgs(input.Arguments)
	if err != nil {
		return ExecuteOutput{}, err
	}
	workspace, err := t.store.GetWorkspaceByID(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	if workspace.Mode != db.WorkspaceModeAgent {
		return ExecuteOutput{}, errors.New("review_shot requires an Agent workspace")
	}
	shots, err := t.resolveReviewShots(ctx, input.WorkspaceID, args.ShotRefs)
	if err != nil {
		return ExecuteOutput{}, err
	}
	queued := []map[string]any{}
	skipped := []map[string]any{}
	for _, shot := range shots {
		node, version, ok, reason := t.currentPhaseWinner(ctx, input.WorkspaceID, shot, args.TargetPhase)
		if !ok {
			skipped = append(skipped, map[string]any{"shot_id": uuidString(shot.ID), "client_key": shot.ClientKey, "reason": reason})
			continue
		}
		thread, err := t.runtime.GetOrCreateReviewerThread(ctx, input.WorkspaceID, shot.ID)
		if err != nil {
			return ExecuteOutput{}, err
		}
		taskInput := map[string]any{
			"target_phase":        args.TargetPhase,
			"shot_id":             uuidString(shot.ID),
			"node_id":             uuidString(node.ID),
			"artifact_version_id": uuidString(version.ID),
			"generation_job_id":   uuidString(version.JobID),
			"attempt_no":          int32(1),
			"max_attempts":        args.MaxAttempts,
			"auto_retry":          args.AutoRetry,
		}
		rawInput, _ := json.Marshal(taskInput)
		task, err := t.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    thread.ID,
			Role:        "reviewer",
			ScopeType:   "shot",
			ScopeID:     shot.ID,
			TaskType:    "reviewer_turn",
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
			EventType:   "review_queued",
			SourceRole:  "producer",
			TargetRole:  "reviewer",
			Scope:       mustJSON(map[string]any{"shot_id": uuidString(shot.ID), "node_id": uuidString(node.ID), "version_id": uuidString(version.ID)}),
			Payload:     mustJSON(map[string]any{"target_phase": args.TargetPhase, "auto_retry": args.AutoRetry}),
		})
		if t.enqueuer != nil {
			t.enqueuer.EnqueueReviewerTask(ctx, task)
		}
		queued = append(queued, map[string]any{"shot_id": uuidString(shot.ID), "client_key": shot.ClientKey, "reviewer_task_id": uuidString(task.ID)})
	}
	label := reviewPhaseLabel(args.TargetPhase)
	summary := fmt.Sprintf("已将 %d 个分镜的%s评审任务加入队列。", len(queued), label)
	if len(skipped) > 0 {
		summary = fmt.Sprintf("已将 %d 个分镜的%s评审任务加入队列，%d 个分镜因没有可评审版本被跳过。", len(queued), label, len(skipped))
	}
	return ExecuteOutput{Summary: summary, Result: map[string]any{"status": "queued", "queued": queued, "skipped": skipped, "summary": summary}}, nil
}

type parsedReviewShotArgs struct {
	ShotRefs    []string
	TargetPhase string
	MaxAttempts int32
	AutoRetry   bool
}

func reviewShotArgs(raw map[string]any) (parsedReviewShotArgs, error) {
	phase := stringValue(raw, "target_phase")
	if phase == "" {
		phase = "preview_image"
	}
	if phase != "preview_image" && phase != "shot_video" {
		return parsedReviewShotArgs{}, fmt.Errorf("unsupported review_shot target_phase %q", phase)
	}
	maxAttempts := int32Value(raw, "max_attempts", 3)
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if maxAttempts > 3 {
		maxAttempts = 3
	}
	return parsedReviewShotArgs{
		ShotRefs:    stringSliceValue(raw, "shot_refs"),
		TargetPhase: phase,
		MaxAttempts: maxAttempts,
		AutoRetry:   boolValue(raw, "auto_retry"),
	}, nil
}

func (t ReviewShotTool) resolveReviewShots(ctx context.Context, workspaceID pgtype.UUID, refs []string) ([]db.Shot, error) {
	if len(refs) == 0 {
		shots, err := t.store.ListActiveShotsByWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		out := []db.Shot{}
		for _, shot := range shots {
			if shot.Status == "preview_ready" || shot.Status == "video_ready" {
				out = append(out, shot)
			}
		}
		return out, nil
	}
	out := []db.Shot{}
	for _, ref := range refs {
		shot, err := t.resolveReviewShotRef(ctx, workspaceID, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, shot)
	}
	return out, nil
}

func (t ReviewShotTool) resolveReviewShotRef(ctx context.Context, workspaceID pgtype.UUID, ref string) (db.Shot, error) {
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
	return t.store.GetShotByClientKey(ctx, db.GetShotByClientKeyParams{WorkspaceID: workspaceID, ClientKey: strings.TrimSpace(ref)})
}

func (t ReviewShotTool) currentPhaseWinner(ctx context.Context, workspaceID pgtype.UUID, shot db.Shot, targetPhase string) (db.MediaNode, db.ArtifactVersion, bool, string) {
	nodes, err := t.store.ListMediaNodesByShot(ctx, db.ListMediaNodesByShotParams{WorkspaceID: workspaceID, ShotID: shot.ID})
	if err != nil {
		return db.MediaNode{}, db.ArtifactVersion{}, false, err.Error()
	}
	wantNodeType, wantKind := reviewPhaseNodeMatcher(targetPhase)
	for _, node := range nodes {
		if !node.CurrentVersionID.Valid || node.NodeType != wantNodeType {
			continue
		}
		if kind := artifactKind(node.Metadata); kind != "" && kind != wantKind {
			continue
		}
		version, err := t.store.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil || version.Status != db.JobStatusSucceeded {
			continue
		}
		return node, version, true, ""
	}
	return db.MediaNode{}, db.ArtifactVersion{}, false, "no_current_" + targetPhase + "_winner"
}

func reviewPhaseNodeMatcher(targetPhase string) (db.NodeType, string) {
	if targetPhase == "shot_video" {
		return db.NodeTypeVideo, "shot_video"
	}
	return db.NodeTypeImage, "preview_image"
}

func artifactKind(raw []byte) string {
	var metadata map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &metadata)
	}
	kind, _ := metadata["agent_artifact_kind"].(string)
	return strings.TrimSpace(kind)
}

func reviewPhaseLabel(targetPhase string) string {
	if targetPhase == "shot_video" {
		return "视频"
	}
	return "预览图"
}
