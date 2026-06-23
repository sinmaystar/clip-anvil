package pss

import (
	"context"
	"encoding/json"
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
	ListGenerationJobsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.GenerationJob, error)
	ListArtifactVersionsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.ArtifactVersion, error)
	ListReviewRecordsByWorkspace(ctx context.Context, params db.ListReviewRecordsByWorkspaceParams) ([]db.ReviewRecord, error)
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
	previewByShot, err := b.previewNodesByShot(ctx, nodes)
	if err != nil {
		return ProducerPSS{}, err
	}
	shotVideoByShot, err := b.shotVideoNodesByShot(ctx, nodes)
	if err != nil {
		return ProducerPSS{}, err
	}
	finalOutputs, err := b.finalOutputNodes(ctx, nodes)
	if err != nil {
		return ProducerPSS{}, err
	}
	reviews, err := b.store.ListReviewRecordsByWorkspace(ctx, db.ListReviewRecordsByWorkspaceParams{
		WorkspaceID: workspaceID,
		Limit:       100,
	})
	if err != nil {
		return ProducerPSS{}, err
	}

	sort.Slice(shots, func(i, j int) bool { return shots[i].SortOrder < shots[j].SortOrder })
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Title < nodes[j].Title })

	dependencyStatus := dependencyStatusSummaries(events, shots)
	text := renderPSS(workspace, nodes, shots, deps, tasks, events, previewByShot, shotVideoByShot, finalOutputs, reviews, dependencyStatus)
	return ProducerPSS{
		Text: text,
		Structured: map[string]any{
			"workspace":         workspaceSummary(workspace),
			"source_materials":  nodeSummaries(nodes),
			"nodes":             nodeSummaries(nodes),
			"shots":             shotSummaries(shots, previewByShot),
			"shot_videos":       shotVideoSummaries(shotVideoByShot, shots),
			"final_outputs":     finalOutputSummaries(finalOutputs),
			"shot_dependencies": dependencySummaries(deps, shots),
			"dependency_status": dependencyStatus,
			"reviews":           reviewSummaries(reviews, shots),
			"pending_decisions": eventSummaries(events),
			"running_tasks":     taskSummaries(tasks),
		},
	}, nil
}

type previewNodeState struct {
	Node     db.MediaNode
	Jobs     []db.GenerationJob
	Versions []db.ArtifactVersion
}

func (b *Builder) previewNodesByShot(ctx context.Context, nodes []db.MediaNode) (map[pgtype.UUID][]previewNodeState, error) {
	out := map[pgtype.UUID][]previewNodeState{}
	for _, node := range nodes {
		if !node.ShotID.Valid || !isPreviewNode(node) {
			continue
		}
		jobs, err := b.store.ListGenerationJobsByNode(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		versions, err := b.store.ListArtifactVersionsByNode(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		out[node.ShotID] = append(out[node.ShotID], previewNodeState{Node: node, Jobs: jobs, Versions: versions})
	}
	for shotID := range out {
		sort.Slice(out[shotID], func(i, j int) bool {
			return out[shotID][i].Node.CreatedAt.Time.Before(out[shotID][j].Node.CreatedAt.Time)
		})
	}
	return out, nil
}

func (b *Builder) shotVideoNodesByShot(ctx context.Context, nodes []db.MediaNode) (map[pgtype.UUID][]previewNodeState, error) {
	out := map[pgtype.UUID][]previewNodeState{}
	for _, node := range nodes {
		if !node.ShotID.Valid || !isShotVideoNode(node) {
			continue
		}
		jobs, err := b.store.ListGenerationJobsByNode(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		versions, err := b.store.ListArtifactVersionsByNode(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		out[node.ShotID] = append(out[node.ShotID], previewNodeState{Node: node, Jobs: jobs, Versions: versions})
	}
	for shotID := range out {
		sort.Slice(out[shotID], func(i, j int) bool {
			return out[shotID][i].Node.CreatedAt.Time.Before(out[shotID][j].Node.CreatedAt.Time)
		})
	}
	return out, nil
}

func (b *Builder) finalOutputNodes(ctx context.Context, nodes []db.MediaNode) ([]previewNodeState, error) {
	out := []previewNodeState{}
	for _, node := range nodes {
		if !isFinalVideoNode(node) {
			continue
		}
		jobs, err := b.store.ListGenerationJobsByNode(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		versions, err := b.store.ListArtifactVersionsByNode(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, previewNodeState{Node: node, Jobs: jobs, Versions: versions})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Node.CreatedAt.Time.Before(out[j].Node.CreatedAt.Time)
	})
	return out, nil
}

func renderPSS(workspace db.Workspace, nodes []db.MediaNode, shots []db.Shot, deps []db.ShotDependency, tasks []db.AgentTask, events []db.AgentEvent, previewByShot map[pgtype.UUID][]previewNodeState, shotVideoByShot map[pgtype.UUID][]previewNodeState, finalOutputs []previewNodeState, reviews []db.ReviewRecord, dependencyStatus []map[string]any) string {
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
			for _, preview := range previewByShot[shot.ID] {
				fmt.Fprintf(&b, "  Preview: %s, node_status=%s", preview.Node.Title, preview.Node.Status)
				if len(preview.Jobs) > 0 {
					latestJob := preview.Jobs[len(preview.Jobs)-1]
					fmt.Fprintf(&b, ", job=%s", latestJob.Status)
				}
				if len(preview.Versions) > 0 {
					latestVersion := preview.Versions[len(preview.Versions)-1]
					fmt.Fprintf(&b, ", version=%s", latestVersion.Status)
				}
				b.WriteString("\n")
			}
			for _, video := range shotVideoByShot[shot.ID] {
				fmt.Fprintf(&b, "  ShotVideo: %s, node_status=%s", video.Node.Title, video.Node.Status)
				if len(video.Jobs) > 0 {
					latestJob := video.Jobs[len(video.Jobs)-1]
					fmt.Fprintf(&b, ", job=%s", latestJob.Status)
				}
				if len(video.Versions) > 0 {
					latestVersion := video.Versions[len(video.Versions)-1]
					fmt.Fprintf(&b, ", version=%s", latestVersion.Status)
				}
				b.WriteString("\n")
			}
		}
	}
	b.WriteString("\n分镜视频\n")
	if len(shotVideoByShot) == 0 {
		b.WriteString("- 无\n")
	} else {
		shotNames := shotKeyByID(shots)
		for _, shot := range shots {
			for _, video := range shotVideoByShot[shot.ID] {
				fmt.Fprintf(&b, "- %s ShotVideo: %s, node_status=%s", shotNames[shot.ID], video.Node.Title, video.Node.Status)
				if len(video.Jobs) > 0 {
					fmt.Fprintf(&b, ", job=%s", video.Jobs[len(video.Jobs)-1].Status)
				}
				if len(video.Versions) > 0 {
					fmt.Fprintf(&b, ", version=%s", video.Versions[len(video.Versions)-1].Status)
				}
				b.WriteString("\n")
			}
		}
	}
	b.WriteString("\n成片\n")
	if len(finalOutputs) == 0 {
		b.WriteString("- 无\n")
	} else {
		for _, output := range finalOutputs {
			fmt.Fprintf(&b, "- FinalVideo: %s, node_status=%s", output.Node.Title, output.Node.Status)
			if len(output.Jobs) > 0 {
				fmt.Fprintf(&b, ", job=%s", output.Jobs[len(output.Jobs)-1].Status)
			}
			if len(output.Versions) > 0 {
				fmt.Fprintf(&b, ", version=%s", output.Versions[len(output.Versions)-1].Status)
			}
			b.WriteString("\n")
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
	b.WriteString("\n依赖状态\n")
	if len(dependencyStatus) == 0 {
		b.WriteString("- 无阻塞状态记录\n")
	} else {
		for _, status := range dependencyStatus {
			fmt.Fprintf(&b, "- %s: %s -> %s, phase=%s", status["event_type"], status["from"], status["to"], status["phase"])
			if ready, ok := status["ready"].(bool); ok {
				fmt.Fprintf(&b, ", ready=%t", ready)
			}
			b.WriteString("\n")
			if reasons, ok := status["blocked_reasons"].([]any); ok && len(reasons) > 0 {
				b.WriteString("  Blocked reasons: " + briefSummary(mustJSON(reasons)) + "\n")
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
	b.WriteString("\n评审记录\n")
	if len(reviews) == 0 {
		b.WriteString("- 无\n")
	} else {
		shotNames := shotKeyByID(shots)
		for _, review := range reviews {
			shotName := shotNames[review.ShotID]
			if strings.TrimSpace(shotName) == "" {
				shotName = uuidString(review.ShotID)
			}
			fmt.Fprintf(&b, "- Review: %s %s %s", shotName, review.TargetPhase, review.Status)
			if review.OverallScore.Valid {
				fmt.Fprintf(&b, ", score=%.2f", review.OverallScore.Float32)
			}
			if review.MaxAttempts > 0 {
				fmt.Fprintf(&b, ", retry=%d/%d", review.AttemptNo, review.MaxAttempts)
			}
			b.WriteString("\n")
			if strings.TrimSpace(review.Critique) != "" {
				b.WriteString("  Critique: " + strings.TrimSpace(review.Critique) + "\n")
			}
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

func shotSummaries(shots []db.Shot, previewByShot map[pgtype.UUID][]previewNodeState) []map[string]any {
	out := make([]map[string]any, 0, len(shots))
	for _, shot := range shots {
		out = append(out, map[string]any{
			"id":                uuidString(shot.ID),
			"client_key":        shot.ClientKey,
			"sort_order":        shot.SortOrder,
			"title":             shot.Title,
			"status":            shot.Status,
			"narrative_purpose": shot.NarrativePurpose,
			"preview_nodes":     previewSummaries(previewByShot[shot.ID]),
		})
	}
	return out
}

func previewSummaries(previews []previewNodeState) []map[string]any {
	out := make([]map[string]any, 0, len(previews))
	for _, preview := range previews {
		summary := map[string]any{
			"node_id":   uuidString(preview.Node.ID),
			"title":     preview.Node.Title,
			"status":    string(preview.Node.Status),
			"operation": preview.Node.OperationType,
		}
		if len(preview.Jobs) > 0 {
			latestJob := preview.Jobs[len(preview.Jobs)-1]
			summary["job_id"] = uuidString(latestJob.ID)
			summary["job_status"] = string(latestJob.Status)
		}
		if len(preview.Versions) > 0 {
			latestVersion := preview.Versions[len(preview.Versions)-1]
			summary["version_id"] = uuidString(latestVersion.ID)
			summary["version_status"] = string(latestVersion.Status)
		}
		out = append(out, summary)
	}
	return out
}

func shotVideoSummaries(shotVideoByShot map[pgtype.UUID][]previewNodeState, shots []db.Shot) []map[string]any {
	shotNames := shotKeyByID(shots)
	out := []map[string]any{}
	for _, shot := range shots {
		for _, video := range shotVideoByShot[shot.ID] {
			summary := productionNodeSummary(video)
			summary["shot_id"] = uuidString(shot.ID)
			summary["shot"] = shotNames[shot.ID]
			out = append(out, summary)
		}
	}
	return out
}

func finalOutputSummaries(outputs []previewNodeState) []map[string]any {
	out := make([]map[string]any, 0, len(outputs))
	for _, output := range outputs {
		out = append(out, productionNodeSummary(output))
	}
	return out
}

func productionNodeSummary(state previewNodeState) map[string]any {
	summary := map[string]any{
		"node_id":   uuidString(state.Node.ID),
		"title":     state.Node.Title,
		"status":    string(state.Node.Status),
		"operation": state.Node.OperationType,
	}
	if len(state.Jobs) > 0 {
		latestJob := state.Jobs[len(state.Jobs)-1]
		summary["job_id"] = uuidString(latestJob.ID)
		summary["job_status"] = string(latestJob.Status)
	}
	if len(state.Versions) > 0 {
		latestVersion := state.Versions[len(state.Versions)-1]
		summary["version_id"] = uuidString(latestVersion.ID)
		summary["version_status"] = string(latestVersion.Status)
	}
	return summary
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

func dependencyStatusSummaries(events []db.AgentEvent, shots []db.Shot) []map[string]any {
	keysByID := shotKeyByID(shots)
	keysByString := map[string]string{}
	for id, key := range keysByID {
		keysByString[uuidString(id)] = key
	}
	out := []map[string]any{}
	for _, event := range events {
		if event.EventType != "shot_blocked" && event.EventType != "shot_unblocked" && event.EventType != "dependency_ready" {
			continue
		}
		var scope map[string]any
		var payload map[string]any
		_ = json.Unmarshal(defaultJSON(event.Scope), &scope)
		_ = json.Unmarshal(defaultJSON(event.Payload), &payload)
		fromID, _ := scope["from_shot_id"].(string)
		toID, _ := scope["to_shot_id"].(string)
		summary := map[string]any{
			"id":           uuidString(event.ID),
			"event_type":   event.EventType,
			"from_shot_id": fromID,
			"from":         defaultString(keysByString[fromID], fromID),
			"to_shot_id":   toID,
			"to":           defaultString(keysByString[toID], toID),
			"phase":        payload["phase"],
			"ready":        payload["ready"],
		}
		if reasons, exists := payload["blocked_reasons"]; exists {
			summary["blocked_reasons"] = reasons
		}
		out = append(out, summary)
	}
	return out
}

func reviewSummaries(reviews []db.ReviewRecord, shots []db.Shot) []map[string]any {
	keys := shotKeyByID(shots)
	out := make([]map[string]any, 0, len(reviews))
	for _, review := range reviews {
		summary := map[string]any{
			"id":                  uuidString(review.ID),
			"shot_id":             uuidString(review.ShotID),
			"shot":                keys[review.ShotID],
			"node_id":             uuidString(review.NodeID),
			"artifact_version_id": uuidString(review.ArtifactVersionID),
			"target_phase":        review.TargetPhase,
			"status":              review.Status,
			"attempt_no":          review.AttemptNo,
			"max_attempts":        review.MaxAttempts,
			"critique":            review.Critique,
		}
		if review.OverallScore.Valid {
			summary["overall_score"] = review.OverallScore.Float32
		}
		out = append(out, summary)
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

func defaultJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func isPreviewNode(node db.MediaNode) bool {
	if node.NodeType != db.NodeTypeImage || node.Source != "agent" || node.OperationType != "text_to_image" {
		return false
	}
	if artifactKind(node) == "preview_image" {
		return true
	}
	return strings.Contains(strings.ToLower(node.Title), "preview")
}

func isShotVideoNode(node db.MediaNode) bool {
	if node.NodeType != db.NodeTypeVideo || node.Source != "agent" {
		return false
	}
	if artifactKind(node) == "shot_video" {
		return true
	}
	return node.OperationType == "image_to_video" && node.ShotID.Valid
}

func isFinalVideoNode(node db.MediaNode) bool {
	if node.NodeType != db.NodeTypeVideo || node.Source != "agent" {
		return false
	}
	if artifactKind(node) == "final_video" {
		return true
	}
	return node.OperationType == "compose_final_video"
}

func artifactKind(node db.MediaNode) string {
	var metadata map[string]any
	if len(node.Metadata) > 0 && json.Unmarshal(node.Metadata, &metadata) == nil {
		if kind, ok := metadata["agent_artifact_kind"].(string); ok {
			return kind
		}
	}
	return ""
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
