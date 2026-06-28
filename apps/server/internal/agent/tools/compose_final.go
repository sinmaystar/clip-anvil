package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ComposeFinalStore interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	ListMediaNodesByShot(ctx context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
}

type ComposeRuntime interface {
	GetOrCreateComposerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type ComposerTaskEnqueuer interface {
	EnqueueComposerTask(ctx context.Context, task db.AgentTask)
}

type ComposeFinalTool struct {
	store    ComposeFinalStore
	runtime  ComposeRuntime
	enqueuer ComposerTaskEnqueuer
}

func NewComposeFinalTool(store ComposeFinalStore, runtime ComposeRuntime, enqueuer ComposerTaskEnqueuer) ComposeFinalTool {
	return ComposeFinalTool{store: store, runtime: runtime, enqueuer: enqueuer}
}

func (t ComposeFinalTool) Definition() Definition {
	return Definition{
		Name:        "compose_final",
		Description: "Compose selected shot videos into a final video. This creates a persistent ComposerGraph task and submits final composition through the internal_ffmpeg production provider.",
		Parameters: objectSchema(map[string]any{
			"strategy": map[string]any{
				"type":        "string",
				"description": "Optional final composition notes.",
			},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			UsesProductionService: true,
			MaxCallsPerTurn:       2,
		},
		Visibility: VisibilitySpec{ShowCallMessage: true, ShowResultMessage: true, UserLabel: "合成成片"},
	}
}

func (t ComposeFinalTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.store == nil || t.runtime == nil {
		return ExecuteOutput{}, errors.New("compose_final service is not configured")
	}
	workspace, err := t.store.GetWorkspaceByID(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	if workspace.Mode != db.WorkspaceModeAgent {
		return ExecuteOutput{}, errors.New("compose_final requires an Agent workspace")
	}
	videoRefs, err := t.currentShotVideoRefs(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	if len(videoRefs) == 0 {
		return ExecuteOutput{}, errors.New("compose_final requires shot video winners")
	}
	thread, err := t.runtime.GetOrCreateComposerThread(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	taskInput := map[string]any{
		"video_node_refs": videoRefs,
		"strategy":        stringValue(input.Arguments, "strategy"),
	}
	rawInput, err := json.Marshal(taskInput)
	if err != nil {
		return ExecuteOutput{}, err
	}
	task, err := t.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    thread.ID,
		Role:        "composer",
		ScopeType:   "final_output",
		TaskType:    "composer_turn",
		MaxAttempts: 1,
		Input:       rawInput,
	})
	if err != nil {
		return ExecuteOutput{}, err
	}
	_, _ = t.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    thread.ID,
		TaskID:      task.ID,
		EventType:   "composer_dispatched",
		SourceRole:  "producer",
		TargetRole:  "composer",
		Payload:     mustJSON(map[string]any{"video_count": len(videoRefs)}),
	})
	if t.enqueuer != nil {
		t.enqueuer.EnqueueComposerTask(ctx, task)
	}
	summary := fmt.Sprintf("已将 %d 个分镜视频加入成片合成队列。成片会由后台 Composer/production 继续处理；当前仅表示任务已排队，不表示视频已经合成完成。", len(videoRefs))
	return ExecuteOutput{Summary: summary, Result: map[string]any{
		"status":           "queued",
		"composer_task_id": uuidString(task.ID),
		"video_node_refs":  videoRefs,
		"summary":          summary,
	}}, nil
}

func (t ComposeFinalTool) currentShotVideoRefs(ctx context.Context, workspaceID pgtype.UUID) ([]string, error) {
	shots, err := t.store.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(shots, func(i, j int) bool {
		return shots[i].SortOrder < shots[j].SortOrder
	})
	out := []string{}
	for _, shot := range shots {
		node, ok, err := t.currentShotVideoWinner(ctx, workspaceID, shot)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, composeFinalNodeRef(node))
		}
	}
	return out, nil
}

func composeFinalNodeRef(node db.MediaNode) string {
	if ref := strings.TrimSpace(node.SemanticKey); ref != "" {
		return ref
	}
	return uuidString(node.ID)
}

func (t ComposeFinalTool) currentShotVideoWinner(ctx context.Context, workspaceID pgtype.UUID, shot db.Shot) (db.MediaNode, bool, error) {
	nodes, err := t.store.ListMediaNodesByShot(ctx, db.ListMediaNodesByShotParams{WorkspaceID: workspaceID, ShotID: shot.ID})
	if err != nil {
		return db.MediaNode{}, false, err
	}
	for _, node := range nodes {
		if node.NodeType != db.NodeTypeVideo || !node.CurrentVersionID.Valid {
			continue
		}
		if kind := artifactKind(node.Metadata); kind != "" && kind != "shot_video" {
			continue
		}
		version, err := t.store.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil || version.Status != db.JobStatusSucceeded {
			continue
		}
		return node, true, nil
	}
	return db.MediaNode{}, false, nil
}
