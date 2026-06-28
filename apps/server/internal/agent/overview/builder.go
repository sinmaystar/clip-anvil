package overview

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Store interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
	ListAgentTasksByWorkspace(ctx context.Context, arg db.ListAgentTasksByWorkspaceParams) ([]db.AgentTask, error)
	ListAgentEventsByWorkspace(ctx context.Context, arg db.ListAgentEventsByWorkspaceParams) ([]db.AgentEvent, error)
	ListReviewRecordsByWorkspace(ctx context.Context, arg db.ListReviewRecordsByWorkspaceParams) ([]db.ReviewRecord, error)
	ListSandboxJobsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.SandboxJob, error)
	GetActiveAudioPlanByWorkspace(ctx context.Context, workspaceID pgtype.UUID) (db.AudioPlan, error)
	ListGenerationJobsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.GenerationJob, error)
	ListArtifactVersionsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.ArtifactVersion, error)
}

type Builder struct {
	store Store
	now   func() time.Time
}

func NewBuilder(store Store) *Builder {
	return &Builder{store: store, now: time.Now}
}

func (b *Builder) Build(ctx context.Context, workspaceID pgtype.UUID) (ProductionOverview, error) {
	workspace, err := b.store.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return ProductionOverview{}, err
	}
	shots, err := b.store.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return ProductionOverview{}, err
	}
	nodes, err := b.store.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		return ProductionOverview{}, err
	}
	tasks, err := b.store.ListAgentTasksByWorkspace(ctx, db.ListAgentTasksByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return ProductionOverview{}, err
	}
	events, err := b.store.ListAgentEventsByWorkspace(ctx, db.ListAgentEventsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return ProductionOverview{}, err
	}
	reviews, err := b.store.ListReviewRecordsByWorkspace(ctx, db.ListReviewRecordsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return ProductionOverview{}, err
	}
	sandboxes, err := b.store.ListSandboxJobsByWorkspace(ctx, workspaceID)
	if err != nil {
		return ProductionOverview{}, err
	}
	audioPlan, hasAudioPlan, err := b.activeAudioPlan(ctx, workspaceID)
	if err != nil {
		return ProductionOverview{}, err
	}

	versionsByNode := make(map[pgtype.UUID][]db.ArtifactVersion)
	jobsByNode := make(map[pgtype.UUID][]db.GenerationJob)
	for _, node := range nodes {
		jobs, err := b.store.ListGenerationJobsByNode(ctx, node.ID)
		if err != nil {
			return ProductionOverview{}, err
		}
		versions, err := b.store.ListArtifactVersionsByNode(ctx, node.ID)
		if err != nil {
			return ProductionOverview{}, err
		}
		jobsByNode[node.ID] = jobs
		versionsByNode[node.ID] = versions
	}

	shotSummaries, counts := buildShots(shots, nodes, reviews)
	timeline := buildTimeline(tasks, events, sandboxes)
	finalOutputs := buildFinalOutputs(nodes, versionsByNode)
	var audioPlanSummary *AudioPlan
	if hasAudioPlan {
		audioPlanSummary = buildAudioPlan(audioPlan, nodes)
		countAudioPlan(audioPlanSummary, &counts)
	}
	counts.FinalOutputs = len(finalOutputs)
	counts.FinalReviews = countFinalReviews(reviews)
	counts.FailedTasks += countFailedNodes(nodes)
	for _, task := range tasks {
		switch task.Status {
		case "queued", "running", "waiting_for_user":
			counts.RunningTasks++
		case "failed":
			counts.FailedTasks++
		}
	}
	for _, event := range events {
		if event.Status == "pending" && event.EventType == "decision_requested" {
			counts.WaitingDecisions++
		}
	}

	return ProductionOverview{
		WorkspaceID:  uuidString(workspace.ID),
		Phase:        inferPhase(counts, nodes, tasks, events),
		Counts:       counts,
		AudioPlan:    audioPlanSummary,
		Shots:        shotSummaries,
		Timeline:     timeline,
		FinalOutputs: finalOutputs,
		Diagnostics: map[string]any{
			"workspace_name":  workspace.Name,
			"nodes":           len(nodes),
			"generation_jobs": countJobs(jobsByNode),
			"reviews":         len(reviews),
			"sandbox_jobs":    len(sandboxes),
		},
		UpdatedAt: timestamp(b.now()),
	}, nil
}

func (b *Builder) activeAudioPlan(ctx context.Context, workspaceID pgtype.UUID) (db.AudioPlan, bool, error) {
	plan, err := b.store.GetActiveAudioPlanByWorkspace(ctx, workspaceID)
	if err == nil {
		return plan, plan.ID.Valid, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AudioPlan{}, false, nil
	}
	return db.AudioPlan{}, false, err
}

func buildShots(shots []db.Shot, nodes []db.MediaNode, reviews []db.ReviewRecord) ([]ShotSummary, Counts) {
	nodesByShot := make(map[pgtype.UUID][]db.MediaNode)
	for _, node := range nodes {
		if node.ShotID.Valid {
			nodesByShot[node.ShotID] = append(nodesByShot[node.ShotID], node)
		}
	}
	reviewsByShot := make(map[pgtype.UUID][]db.ReviewRecord)
	for _, review := range reviews {
		if review.ShotID.Valid {
			reviewsByShot[review.ShotID] = append(reviewsByShot[review.ShotID], review)
		}
	}

	out := make([]ShotSummary, 0, len(shots))
	counts := Counts{ShotsTotal: len(shots)}
	for _, shot := range shots {
		summary := ShotSummary{
			ID:            uuidString(shot.ID),
			ClientKey:     shot.ClientKey,
			SortOrder:     shot.SortOrder,
			Title:         shot.Title,
			Status:        shot.Status,
			PreviewStatus: StatusNone,
			ReviewStatus:  StatusNone,
			VideoStatus:   StatusNone,
		}
		if shot.DurationSec.Valid {
			summary.DurationSec = shot.DurationSec.Float64
		}
		for _, node := range nodesByShot[shot.ID] {
			kind := artifactKind(node)
			if kind == "preview_image" || (kind == "" && node.NodeType == db.NodeTypeImage) {
				status := nodeStatus(node.Status)
				summary.PreviewStatus = higherStatus(summary.PreviewStatus, status)
				if summary.PreviewNodeID == "" {
					summary.PreviewNodeID = uuidString(node.ID)
				}
				if status == StatusReady {
					counts.PreviewsReady++
				}
			}
			if kind == "shot_video" || node.NodeType == db.NodeTypeVideo {
				status := nodeStatus(node.Status)
				summary.VideoStatus = higherStatus(summary.VideoStatus, status)
				if summary.VideoNodeID == "" {
					summary.VideoNodeID = uuidString(node.ID)
				}
				if status == StatusReady {
					counts.VideosReady++
				}
			}
		}
		for _, review := range reviewsByShot[shot.ID] {
			status := reviewStatus(review.Status)
			summary.ReviewStatus = higherStatus(summary.ReviewStatus, status)
			if review.Status == "accepted" {
				counts.ReviewsAccepted++
			}
			if review.OverallScore.Valid {
				score := review.OverallScore.Float32
				summary.ReviewScore = &score
			}
		}
		out = append(out, summary)
	}
	return out, counts
}

func buildAudioPlan(plan db.AudioPlan, nodes []db.MediaNode) *AudioPlan {
	if !plan.ID.Valid {
		return nil
	}
	nodesByID := make(map[pgtype.UUID]db.MediaNode, len(nodes))
	for _, node := range nodes {
		nodesByID[node.ID] = node
	}
	out := &AudioPlan{
		ID:                    uuidString(plan.ID),
		Status:                plan.Status,
		Title:                 plan.Title,
		PlanKind:              plan.PlanKind,
		Language:              plan.Language,
		VoiceoverScript:       plan.VoiceoverScript,
		VoiceProfile:          jsonMap(plan.VoiceProfile),
		BGMPlan:               jsonMap(plan.BgmPlan),
		VoiceoverNodeID:       uuidString(plan.VoiceoverNodeID),
		VoiceoverStatus:       linkedAudioNodeStatus(plan.VoiceoverNodeID, nodesByID),
		BGMNodeID:             uuidString(plan.BgmNodeID),
		BGMStatus:             linkedAudioNodeStatus(plan.BgmNodeID, nodesByID),
		TimelinePlanID:        uuidString(plan.TimelinePlanID),
		VoiceoverRenderPlanID: uuidString(plan.VoiceoverRenderPlanID),
		BGMRenderPlanID:       uuidString(plan.BgmRenderPlanID),
	}
	if plan.TargetDurationSec.Valid {
		value := plan.TargetDurationSec.Float64
		out.TargetDurationSec = &value
	}
	return out
}

func linkedAudioNodeStatus(nodeID pgtype.UUID, nodes map[pgtype.UUID]db.MediaNode) Status {
	if !nodeID.Valid {
		return StatusNone
	}
	node, ok := nodes[nodeID]
	if !ok {
		return StatusNone
	}
	return nodeStatus(node.Status)
}

func countAudioPlan(plan *AudioPlan, counts *Counts) {
	if plan == nil || counts == nil {
		return
	}
	countAudioStatus(plan.VoiceoverNodeID, plan.VoiceoverStatus, counts)
	countAudioStatus(plan.BGMNodeID, plan.BGMStatus, counts)
}

func countAudioStatus(nodeID string, status Status, counts *Counts) {
	if nodeID == "" && status == StatusNone {
		return
	}
	if status == StatusReady {
		counts.AudioReady++
		return
	}
	counts.AudioMissing++
}

func countFinalReviews(reviews []db.ReviewRecord) int {
	count := 0
	for _, review := range reviews {
		if review.ReviewTask == "final_video_review" || review.TargetPhase == "final_video" {
			count++
		}
	}
	return count
}

func buildTimeline(tasks []db.AgentTask, events []db.AgentEvent, sandboxes []db.SandboxJob) []TimelineItem {
	items := make([]TimelineItem, 0, len(tasks)+len(events)+len(sandboxes))
	for _, task := range tasks {
		items = append(items, TimelineItem{
			ID:          uuidString(task.ID),
			Type:        task.TaskType,
			Label:       taskLabel(task.TaskType),
			Status:      task.Status,
			Role:        userRoleLabel(task.Role),
			Diagnostics: map[string]any{"attempt": task.Attempt, "max_attempts": task.MaxAttempts},
			CreatedAt:   timestamptz(task.CreatedAt),
			CompletedAt: timestamptz(task.CompletedAt),
		})
	}
	for _, event := range events {
		items = append(items, TimelineItem{
			ID:          uuidString(event.ID),
			Type:        event.EventType,
			Label:       eventLabel(event.EventType),
			Status:      event.Status,
			Role:        userRoleLabel(event.SourceRole),
			Scope:       jsonMap(event.Scope),
			Diagnostics: map[string]any{"task_id": uuidString(event.TaskID)},
			CreatedAt:   timestamptz(event.CreatedAt),
			CompletedAt: timestamptz(event.HandledAt),
		})
	}
	for _, job := range sandboxes {
		items = append(items, TimelineItem{
			ID:          uuidString(job.ID),
			Type:        job.JobType,
			Label:       sandboxLabel(job.JobType, job.OperationType),
			Status:      string(job.Status),
			Diagnostics: map[string]any{"target_node_id": uuidString(job.TargetNodeID), "duration_ms": job.DurationMs},
			CreatedAt:   timestamptz(job.CreatedAt),
			CompletedAt: timestamptz(job.CompletedAt),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items
}

func buildFinalOutputs(nodes []db.MediaNode, versionsByNode map[pgtype.UUID][]db.ArtifactVersion) []FinalOutput {
	out := make([]FinalOutput, 0)
	for _, node := range nodes {
		kind := artifactKind(node)
		if kind != "final_video" && node.OperationType != "compose_final_video" {
			continue
		}
		item := FinalOutput{
			NodeID:    uuidString(node.ID),
			Title:     node.Title,
			Status:    nodeStatus(node.Status),
			Operation: node.OperationType,
		}
		for _, version := range versionsByNode[node.ID] {
			if version.Winner || uuidEqual(version.ID, node.CurrentVersionID) {
				item.VersionID = uuidString(version.ID)
				item.AssetID = uuidString(version.AssetID)
				item.CompletedAt = timestamptz(version.CompletedAt)
				break
			}
		}
		out = append(out, item)
	}
	return out
}

func inferPhase(counts Counts, nodes []db.MediaNode, tasks []db.AgentTask, events []db.AgentEvent) Phase {
	if counts.FailedTasks > 0 {
		return PhaseNeedsAttention
	}
	if counts.WaitingDecisions > 0 {
		return PhaseWaitingConfirmation
	}
	for _, task := range tasks {
		if task.Status == "queued" || task.Status == "running" || task.Status == "waiting_for_user" {
			switch task.Role {
			case "composer":
				return PhaseFinal
			case "reviewer":
				return PhaseReview
			case "worker":
				return PhaseVideo
			default:
				return PhasePreview
			}
		}
	}
	if counts.FinalOutputs > 0 {
		return PhaseComplete
	}
	if counts.VideosReady > 0 {
		return PhaseFinal
	}
	if counts.ReviewsAccepted > 0 {
		return PhaseVideo
	}
	if counts.PreviewsReady > 0 {
		return PhaseReview
	}
	if len(nodes) > 0 || len(events) > 0 {
		return PhasePreview
	}
	return PhasePlanning
}

func artifactKind(node db.MediaNode) string {
	var payload map[string]any
	if len(node.Metadata) == 0 || json.Unmarshal(node.Metadata, &payload) != nil {
		return ""
	}
	if value, ok := payload["agent_artifact_kind"].(string); ok {
		return value
	}
	if value, ok := payload["artifact_kind"].(string); ok {
		return value
	}
	return ""
}

func nodeStatus(status db.NodeStatus) Status {
	switch status {
	case db.NodeStatusSucceeded:
		return StatusReady
	case db.NodeStatusRunning:
		return StatusRunning
	case db.NodeStatusFailed:
		return StatusFailed
	case db.NodeStatusQueued:
		return StatusQueued
	default:
		return StatusNone
	}
}

func reviewStatus(status string) Status {
	switch status {
	case "accepted":
		return StatusReady
	case "running":
		return StatusRunning
	case "rejected", "failed":
		return StatusFailed
	default:
		return StatusQueued
	}
}

func higherStatus(left, right Status) Status {
	rank := map[Status]int{StatusNone: 0, StatusQueued: 1, StatusRunning: 2, StatusFailed: 3, StatusReady: 4}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func taskLabel(taskType string) string {
	switch taskType {
	case "producer_turn":
		return "分析需求"
	case "craftsman_turn":
		return "细化分镜"
	case "worker_generation":
		return "生成素材"
	case "reviewer_turn":
		return "质量复核"
	case "composer_turn":
		return "合成成片"
	default:
		return "处理任务"
	}
}

func eventLabel(eventType string) string {
	switch eventType {
	case "decision_requested":
		return "请求用户确认"
	case "decision_resolved":
		return "用户确认完成"
	case "tool_call_started":
		return "开始执行工具"
	case "tool_call_completed":
		return "工具执行完成"
	case "composition_succeeded":
		return "成片合成完成"
	default:
		return "记录进展"
	}
}

func sandboxLabel(jobType, operation string) string {
	if operation == "compose_final_video" {
		return "渲染成片"
	}
	if jobType != "" {
		return "执行沙箱任务"
	}
	return "处理媒体任务"
}

func userRoleLabel(role string) string {
	switch role {
	case "producer":
		return "ClipAnvil"
	case "craftsman":
		return "分镜规划"
	case "worker":
		return "素材生成"
	case "reviewer":
		return "质量复核"
	case "composer":
		return "成片合成"
	default:
		return ""
	}
}

func jsonMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func countJobs(jobsByNode map[pgtype.UUID][]db.GenerationJob) int {
	total := 0
	for _, jobs := range jobsByNode {
		total += len(jobs)
	}
	return total
}

func countFailedNodes(nodes []db.MediaNode) int {
	total := 0
	for _, node := range nodes {
		if node.Status == db.NodeStatusFailed {
			total++
		}
	}
	return total
}

func timestamptz(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return timestamp(value.Time)
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func uuidEqual(left, right pgtype.UUID) bool {
	return left.Valid && right.Valid && left.Bytes == right.Bytes
}
