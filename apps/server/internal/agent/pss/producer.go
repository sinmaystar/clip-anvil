package pss

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Store interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	ListShotDependenciesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.ShotDependency, error)
	ListActiveAgentTasksByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error)
	ListAgentEventsByWorkspaceStatus(ctx context.Context, params db.ListAgentEventsByWorkspaceStatusParams) ([]db.AgentEvent, error)
}

type Builder struct {
	store Store
}

func NewBuilder(store Store) *Builder {
	return &Builder{store: store}
}

type ProducerPSS struct {
	Text       string
	Structured map[string]any
}

func (b *Builder) BuildProducerPSS(ctx context.Context, workspaceID pgtype.UUID) (ProducerPSS, error) {
	if b == nil || b.store == nil || !workspaceID.Valid {
		return ProducerPSS{}, fmt.Errorf("invalid producer pss builder")
	}
	workspace, err := b.store.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return ProducerPSS{}, err
	}
	nodes, err := b.store.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		return ProducerPSS{}, err
	}
	shots, err := b.store.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return ProducerPSS{}, err
	}
	deps, err := b.store.ListShotDependenciesByWorkspace(ctx, workspaceID)
	if err != nil {
		return ProducerPSS{}, err
	}
	tasks, err := b.store.ListActiveAgentTasksByWorkspace(ctx, workspaceID)
	if err != nil {
		return ProducerPSS{}, err
	}
	events, err := b.store.ListAgentEventsByWorkspaceStatus(ctx, db.ListAgentEventsByWorkspaceStatusParams{
		WorkspaceID: workspaceID,
		Status:      "pending",
	})
	if err != nil {
		return ProducerPSS{}, err
	}

	sort.Slice(shots, func(i, j int) bool { return shots[i].SortOrder < shots[j].SortOrder })
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Title < nodes[j].Title })

	text := renderPSS(workspace, nodes, shots, deps, tasks, events)
	return ProducerPSS{
		Text: text,
		Structured: map[string]any{
			"workspace":         workspaceSummary(workspace),
			"source_materials":  nodeSummaries(nodes),
			"nodes":             nodeSummaries(nodes),
			"shots":             shotSummaries(shots),
			"shot_dependencies": dependencySummaries(deps, shots),
			"pending_decisions": eventSummaries(events),
			"running_tasks":     taskSummaries(tasks),
		},
	}, nil
}

func renderPSS(workspace db.Workspace, nodes []db.MediaNode, shots []db.Shot, deps []db.ShotDependency, tasks []db.AgentTask, events []db.AgentEvent) string {
	var b strings.Builder
	b.WriteString("当前项目\n")
	b.WriteString("- Workspace: " + workspace.Name + "\n")
	b.WriteString("- Mode: " + string(workspace.Mode) + "\n\n")

	b.WriteString("用户素材\n")
	if len(nodes) == 0 {
		b.WriteString("- 无\n")
	} else {
		for _, node := range nodes {
			fmt.Fprintf(&b, "- [node:%s] %s, %s, source=%s, status=%s\n", uuidString(node.ID), node.Title, node.NodeType, node.Source, node.Status)
		}
	}
	b.WriteString("\nStoryboard\n")
	if len(shots) == 0 {
		b.WriteString("- 当前还没有 storyboard。\n")
	} else {
		for _, shot := range shots {
			duration := ""
			if shot.DurationSec.Valid {
				duration = fmt.Sprintf(", %.1fs", shot.DurationSec.Float64)
			}
			fmt.Fprintf(&b, "- [%s] %s%s, status=%s\n", shot.ClientKey, shot.Title, duration, shot.Status)
			if strings.TrimSpace(shot.NarrativePurpose) != "" {
				b.WriteString("  目标: " + strings.TrimSpace(shot.NarrativePurpose) + "\n")
			}
			if summary := briefSummary(shot.Brief); summary != "" {
				b.WriteString("  Brief: " + summary + "\n")
			}
		}
	}
	b.WriteString("\n分镜依赖\n")
	if len(deps) == 0 {
		b.WriteString("- 无\n")
	} else {
		shotNames := shotKeyByID(shots)
		for _, dep := range deps {
			fmt.Fprintf(&b, "- %s -> %s: %s", shotNames[dep.FromShotID], shotNames[dep.ToShotID], dep.DependencyType)
			if dep.BlockingPhase != "" {
				b.WriteString(", blocking_phase=" + dep.BlockingPhase)
			}
			b.WriteString("\n")
			if strings.TrimSpace(dep.Reason) != "" {
				b.WriteString("  Reason: " + strings.TrimSpace(dep.Reason) + "\n")
			}
		}
	}
	b.WriteString("\n生产节点\n")
	if len(nodes) == 0 {
		b.WriteString("- 无\n")
	} else {
		for _, node := range nodes {
			fmt.Fprintf(&b, "- %s node %s: source=%s, status=%s\n", node.NodeType, node.Title, node.Source, node.Status)
		}
	}
	b.WriteString("\n待处理决策\n")
	if len(events) == 0 {
		b.WriteString("- 无\n")
	} else {
		for _, event := range events {
			fmt.Fprintf(&b, "- %s: %s\n", uuidString(event.ID), event.EventType)
		}
	}
	b.WriteString("\n正在运行\n")
	if len(tasks) == 0 {
		b.WriteString("- 无\n")
	} else {
		for _, task := range tasks {
			fmt.Fprintf(&b, "- %s %s status=%s\n", task.TaskType, uuidString(task.ID), task.Status)
		}
	}
	return b.String()
}

func workspaceSummary(workspace db.Workspace) map[string]any {
	return map[string]any{"id": uuidString(workspace.ID), "name": workspace.Name, "mode": string(workspace.Mode)}
}

func nodeSummaries(nodes []db.MediaNode) []map[string]any {
	out := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, map[string]any{
			"id":      uuidString(node.ID),
			"title":   node.Title,
			"type":    string(node.NodeType),
			"source":  node.Source,
			"status":  string(node.Status),
			"shot_id": uuidString(node.ShotID),
		})
	}
	return out
}

func shotSummaries(shots []db.Shot) []map[string]any {
	out := make([]map[string]any, 0, len(shots))
	for _, shot := range shots {
		out = append(out, map[string]any{
			"id":                uuidString(shot.ID),
			"client_key":        shot.ClientKey,
			"sort_order":        shot.SortOrder,
			"title":             shot.Title,
			"status":            shot.Status,
			"narrative_purpose": shot.NarrativePurpose,
		})
	}
	return out
}

func dependencySummaries(deps []db.ShotDependency, shots []db.Shot) []map[string]any {
	keys := shotKeyByID(shots)
	out := make([]map[string]any, 0, len(deps))
	for _, dep := range deps {
		out = append(out, map[string]any{
			"id":             uuidString(dep.ID),
			"from_shot_id":   uuidString(dep.FromShotID),
			"from":           keys[dep.FromShotID],
			"to_shot_id":     uuidString(dep.ToShotID),
			"to":             keys[dep.ToShotID],
			"type":           dep.DependencyType,
			"blocking_phase": dep.BlockingPhase,
			"reason":         dep.Reason,
		})
	}
	return out
}

func taskSummaries(tasks []db.AgentTask) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, map[string]any{"id": uuidString(task.ID), "task_type": task.TaskType, "status": task.Status})
	}
	return out
}

func eventSummaries(events []db.AgentEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		out = append(out, map[string]any{"id": uuidString(event.ID), "event_type": event.EventType, "status": event.Status})
	}
	return out
}

func shotKeyByID(shots []db.Shot) map[pgtype.UUID]string {
	out := map[pgtype.UUID]string{}
	for _, shot := range shots {
		key := strings.TrimSpace(shot.ClientKey)
		if key == "" {
			key = uuidString(shot.ID)
		}
		out[shot.ID] = key
	}
	return out
}

func briefSummary(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "{}" || text == "null" {
		return ""
	}
	return text
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
