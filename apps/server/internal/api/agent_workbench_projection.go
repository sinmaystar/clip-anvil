package api

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type agentWorkbenchResponse struct {
	Overview    agentWorkbenchOverviewResponse     `json:"overview"`
	Scenes      []agentWorkbenchSceneResponse      `json:"scenes"`
	Counts      agentWorkbenchCountsResponse       `json:"counts"`
	FinalOutput *agentWorkbenchFinalOutputResponse `json:"final_output,omitempty"`
}

type agentWorkbenchOverviewResponse struct {
	WorkspaceID      string                                  `json:"workspace_id"`
	Brief            *agentWorkbenchBriefResponse            `json:"brief,omitempty"`
	Memory           *agentWorkbenchMemoryResponse           `json:"memory,omitempty"`
	AudioPlan        *agentWorkbenchAudioPlanResponse        `json:"audio_plan,omitempty"`
	KeyElements      []agentWorkbenchKeyElementResponse      `json:"key_elements"`
	KeyElementStates []agentWorkbenchKeyElementStateResponse `json:"key_element_states"`
	SourceMaterials  []agentWorkbenchSourceMaterialResponse  `json:"source_materials"`
}

type agentWorkbenchBriefResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Concept string `json:"concept"`
	Status  string `json:"status"`
}

type agentWorkbenchMemoryResponse struct {
	ID      string `json:"id"`
	Version int32  `json:"version"`
	Soul    string `json:"soul"`
	Status  string `json:"status"`
}

type agentWorkbenchKeyElementResponse struct {
	ID        string `json:"id"`
	ClientKey string `json:"client_key"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Status    string `json:"status"`
}

type agentWorkbenchKeyElementStateResponse struct {
	ID              string `json:"id"`
	KeyElementID    string `json:"key_element_id"`
	ClientKey       string `json:"client_key"`
	Label           string `json:"label"`
	ReferenceStatus string `json:"reference_status"`
}

type agentWorkbenchSourceMaterialResponse struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	NodeType string `json:"node_type"`
	Status   string `json:"status"`
}

type agentWorkbenchAudioPlanResponse struct {
	ID                    string         `json:"id"`
	Status                string         `json:"status"`
	Title                 string         `json:"title"`
	PlanKind              string         `json:"plan_kind,omitempty"`
	Language              string         `json:"language,omitempty"`
	TargetDurationSec     *float64       `json:"target_duration_sec,omitempty"`
	VoiceoverScript       string         `json:"voiceover_script,omitempty"`
	VoiceProfile          map[string]any `json:"voice_profile,omitempty"`
	BGMPlan               map[string]any `json:"bgm_plan,omitempty"`
	CuePlan               any            `json:"cue_plan,omitempty"`
	VoiceoverNodeID       string         `json:"voiceover_node_id,omitempty"`
	VoiceoverStatus       string         `json:"voiceover_status,omitempty"`
	BGMNodeID             string         `json:"bgm_node_id,omitempty"`
	BGMStatus             string         `json:"bgm_status,omitempty"`
	TimelinePlanID        string         `json:"timeline_plan_id,omitempty"`
	VoiceoverRenderPlanID string         `json:"voiceover_render_plan_id,omitempty"`
	BGMRenderPlanID       string         `json:"bgm_render_plan_id,omitempty"`
}

type agentWorkbenchSceneResponse struct {
	ID       string                       `json:"id"`
	Title    string                       `json:"title"`
	Status   string                       `json:"status"`
	Summary  string                       `json:"summary,omitempty"`
	Location string                       `json:"location,omitempty"`
	Shots    []agentWorkbenchShotResponse `json:"shots"`
}

type agentWorkbenchShotResponse struct {
	ID            string                                    `json:"id"`
	ClientKey     string                                    `json:"client_key"`
	Title         string                                    `json:"title"`
	Status        string                                    `json:"status"`
	SequenceIndex int32                                     `json:"sequence_index"`
	CreativeText  string                                    `json:"creative_text"`
	Dependencies  []agentWorkbenchShotDependencyResponse    `json:"dependencies"`
	KeyElements   []agentWorkbenchShotKeyElementRefResponse `json:"key_elements"`
	Preview       agentWorkbenchArtifactSlotResponse        `json:"preview"`
	Video         agentWorkbenchArtifactSlotResponse        `json:"video"`
	Artifacts     []agentWorkbenchArtifactSlotResponse      `json:"artifacts"`
	RenderPlans   []agentWorkbenchRenderPlanSummaryResponse `json:"render_plans"`
	Review        *agentWorkbenchReviewSummaryResponse      `json:"review,omitempty"`
	Issues        []agentWorkbenchIssueSummaryResponse      `json:"issues"`
}

type agentWorkbenchShotDependencyResponse struct {
	ID             string `json:"id"`
	FromShotID     string `json:"from_shot_id"`
	ToShotID       string `json:"to_shot_id"`
	DependencyType string `json:"dependency_type"`
}

type agentWorkbenchShotKeyElementRefResponse struct {
	ID                string `json:"id"`
	KeyElementID      string `json:"key_element_id"`
	KeyElementStateID string `json:"key_element_state_id,omitempty"`
	Role              string `json:"role"`
}

type agentWorkbenchArtifactSlotResponse struct {
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	NodeID       string `json:"node_id,omitempty"`
	Title        string `json:"title,omitempty"`
	VersionID    string `json:"version_id,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	AccessURL    string `json:"access_url,omitempty"`
	Width        int32  `json:"width,omitempty"`
	Height       int32  `json:"height,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type agentWorkbenchRenderPlanSummaryResponse struct {
	ID          string `json:"id"`
	Revision    int32  `json:"revision"`
	TargetPhase string `json:"target_phase"`
	Operation   string `json:"operation"`
	Status      string `json:"status"`
}

type agentWorkbenchReviewSummaryResponse struct {
	ID          string  `json:"id"`
	ReviewTask  string  `json:"review_task"`
	TargetPhase string  `json:"target_phase"`
	Status      string  `json:"status"`
	Verdict     string  `json:"verdict"`
	Score       float32 `json:"score,omitempty"`
}

type agentWorkbenchIssueSummaryResponse struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Severity     string `json:"severity"`
	Dimension    string `json:"dimension"`
	SuggestedFix string `json:"suggested_fix"`
}

type agentWorkbenchAudioSummaryResponse struct {
	HasVoiceover bool   `json:"has_voiceover"`
	HasBGM       bool   `json:"has_bgm"`
	AudioCodec   string `json:"audio_codec,omitempty"`
	TrackCount   int    `json:"track_count"`
	Ducking      bool   `json:"ducking"`
}

type agentWorkbenchAudioTrackResponse struct {
	Role          string  `json:"role"`
	AssetID       string  `json:"asset_id,omitempty"`
	WorkspacePath string  `json:"workspace_path,omitempty"`
	StartSec      float64 `json:"start_sec,omitempty"`
	DurationSec   float64 `json:"duration_sec,omitempty"`
	Volume        float64 `json:"volume,omitempty"`
	Ducking       bool    `json:"ducking,omitempty"`
}

type agentWorkbenchFinalOutputResponse struct {
	ID                string                               `json:"id"`
	TimelinePlanID    string                               `json:"timeline_plan_id"`
	OutputNodeID      string                               `json:"output_node_id,omitempty"`
	ArtifactVersionID string                               `json:"artifact_version_id,omitempty"`
	SandboxJobID      string                               `json:"sandbox_job_id,omitempty"`
	Status            string                               `json:"status"`
	TemplateKey       string                               `json:"template_key"`
	Summary           string                               `json:"summary,omitempty"`
	AssetURL          string                               `json:"asset_url,omitempty"`
	ThumbnailURL      string                               `json:"thumbnail_url,omitempty"`
	AssetID           string                               `json:"asset_id,omitempty"`
	Mime              string                               `json:"mime,omitempty"`
	AudioSummary      *agentWorkbenchAudioSummaryResponse  `json:"audio_summary,omitempty"`
	AudioTracks       []agentWorkbenchAudioTrackResponse   `json:"audio_tracks,omitempty"`
	FinalReview       *agentWorkbenchReviewSummaryResponse `json:"final_review,omitempty"`
	Plan              map[string]any                       `json:"plan,omitempty"`
	Result            map[string]any                       `json:"result,omitempty"`
	UpdatedAt         time.Time                            `json:"updated_at"`
}

type agentWorkbenchCountsResponse struct {
	Scenes           int `json:"scenes"`
	Shots            int `json:"shots"`
	PreviewSucceeded int `json:"preview_succeeded"`
	PreviewFailed    int `json:"preview_failed"`
	VideoSucceeded   int `json:"video_succeeded"`
	VideoFailed      int `json:"video_failed"`
	OpenIssues       int `json:"open_issues"`
	NeedsReference   int `json:"needs_reference"`
	AudioReady       int `json:"audio_ready"`
	AudioMissing     int `json:"audio_missing"`
	FinalReviews     int `json:"final_reviews"`
}

func buildAgentWorkbenchProjection(ctx context.Context, queries *db.Queries, signer assetURLSigner, workspaceID pgtype.UUID) (agentWorkbenchResponse, error) {
	if queries == nil || !workspaceID.Valid {
		return agentWorkbenchResponse{}, nil
	}

	response := agentWorkbenchResponse{
		Overview: agentWorkbenchOverviewResponse{
			WorkspaceID: uuidToString(workspaceID),
		},
	}

	if brief, ok, err := activeCreativeBrief(ctx, queries, workspaceID); err != nil {
		return response, err
	} else if ok {
		response.Overview.Brief = &agentWorkbenchBriefResponse{
			ID:      uuidToString(brief.ID),
			Title:   brief.Title,
			Concept: brief.Concept,
			Status:  brief.Status,
		}
	}

	if memory, ok, err := activeProjectMemory(ctx, queries, workspaceID); err != nil {
		return response, err
	} else if ok {
		response.Overview.Memory = &agentWorkbenchMemoryResponse{
			ID:      uuidToString(memory.ID),
			Version: memory.Version,
			Soul:    memory.Soul,
			Status:  memory.Status,
		}
	}

	elements, err := queries.ListActiveKeyElementsByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	for _, element := range elements {
		response.Overview.KeyElements = append(response.Overview.KeyElements, agentWorkbenchKeyElementResponse{
			ID:        uuidToString(element.ID),
			ClientKey: element.ClientKey,
			Name:      element.Name,
			Type:      element.ElementType,
			Status:    element.Status,
		})
	}

	states, err := queries.ListActiveKeyElementStatesByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	for _, state := range states {
		if state.ReferenceStatus == "needs_reference" {
			response.Counts.NeedsReference++
		}
		response.Overview.KeyElementStates = append(response.Overview.KeyElementStates, agentWorkbenchKeyElementStateResponse{
			ID:              uuidToString(state.ID),
			KeyElementID:    uuidToString(state.KeyElementID),
			ClientKey:       state.ClientKey,
			Label:           state.Label,
			ReferenceStatus: state.ReferenceStatus,
		})
	}

	nodes, err := queries.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	assets, err := queries.ListMediaAssetsByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	assetsByID := make(map[pgtype.UUID]db.MediaAsset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.ID] = asset
	}
	versionsByID := make(map[pgtype.UUID]db.ArtifactVersion)
	for _, node := range nodes {
		if !node.CurrentVersionID.Valid {
			continue
		}
		version, err := queries.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil {
			return response, err
		}
		versionsByID[node.CurrentVersionID] = version
	}
	nodesByID := make(map[pgtype.UUID]db.MediaNode, len(nodes))
	for _, node := range nodes {
		nodesByID[node.ID] = node
	}
	response.Overview.SourceMaterials = agentWorkbenchSourceMaterials(nodes)

	if audioPlan, ok, err := activeWorkbenchAudioPlan(ctx, queries, workspaceID); err != nil {
		return response, err
	} else if ok {
		response.Overview.AudioPlan = agentWorkbenchAudioPlanSummary(audioPlan, nodesByID)
		countWorkbenchAudioPlan(*response.Overview.AudioPlan, &response.Counts)
	}

	reviews, err := queries.ListReviewRecordsByWorkspace(ctx, db.ListReviewRecordsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return response, err
	}
	response.Counts.FinalReviews = countFinalVideoReviews(reviews)

	if timelinePlan, ok, err := latestWorkbenchTimelinePlan(ctx, queries, workspaceID); err != nil {
		return response, err
	} else if ok {
		response.FinalOutput = agentWorkbenchFinalOutputFromTimelinePlan(ctx, signer, timelinePlan, nodesByID, versionsByID, assetsByID, reviews)
	} else if finalNode, ok := latestWorkbenchFinalVideoNode(nodes); ok {
		response.FinalOutput = agentWorkbenchFinalOutputFromNode(ctx, signer, finalNode, versionsByID, assetsByID, reviews)
	}

	scenes, err := queries.ListActiveScenesByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	shots, err := queries.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	shotElements, err := queries.ListShotKeyElementsByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	deps, err := queries.ListShotDependenciesByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	renderPlans, err := queries.ListRenderPlansByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	issues, err := queries.ListOpenArtifactIssuesByWorkspace(ctx, db.ListOpenArtifactIssuesByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return response, err
	}

	response.Scenes = agentWorkbenchScenes(ctx, signer, scenes, shots, nodes, assetsByID, versionsByID, shotElements, deps, renderPlans, reviews, issues, &response.Counts)
	response.Counts.Scenes = len(response.Scenes)
	response.Counts.Shots = len(shots)
	return response, nil
}

func activeWorkbenchAudioPlan(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID) (db.AudioPlan, bool, error) {
	plan, err := queries.GetActiveAudioPlanByWorkspace(ctx, workspaceID)
	if err == nil {
		return plan, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AudioPlan{}, false, nil
	}
	return db.AudioPlan{}, false, err
}

func latestWorkbenchTimelinePlan(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID) (db.TimelinePlan, bool, error) {
	plan, err := queries.GetLatestCompletedTimelinePlanByWorkspace(ctx, workspaceID)
	if err == nil {
		return plan, true, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return db.TimelinePlan{}, false, err
	}
	plan, err = queries.GetLatestTimelinePlanByWorkspace(ctx, workspaceID)
	if err == nil {
		return plan, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return db.TimelinePlan{}, false, nil
	}
	return db.TimelinePlan{}, false, err
}

func agentWorkbenchFinalOutputFromTimelinePlan(
	ctx context.Context,
	signer assetURLSigner,
	plan db.TimelinePlan,
	nodes map[pgtype.UUID]db.MediaNode,
	versions map[pgtype.UUID]db.ArtifactVersion,
	assets map[pgtype.UUID]db.MediaAsset,
	reviews []db.ReviewRecord,
) *agentWorkbenchFinalOutputResponse {
	if !plan.ID.Valid {
		return nil
	}
	out := &agentWorkbenchFinalOutputResponse{
		ID:             uuidToString(plan.ID),
		TimelinePlanID: uuidToString(plan.ID),
		OutputNodeID:   uuidString(plan.OutputNodeID),
		SandboxJobID:   uuidString(plan.SandboxJobID),
		Status:         plan.Status,
		TemplateKey:    plan.TemplateKey,
		Plan:           jsonObjectValue(plan.PlanJson),
		Result:         jsonObjectValue(plan.Result),
		UpdatedAt:      timestamptzTime(plan.UpdatedAt),
	}
	out.AudioTracks = agentWorkbenchAudioTracks(plan.PlanJson)
	if audioSummary, ok := agentWorkbenchAudioSummary(plan.PlanJson, plan.Result); ok {
		out.AudioSummary = audioSummary
	}
	if summary, ok := out.Result["summary"].(string); ok {
		out.Summary = summary
	}
	versionID := plan.ArtifactVersionID
	if !versionID.Valid {
		if node, ok := nodes[plan.OutputNodeID]; ok {
			versionID = node.CurrentVersionID
		}
	}
	if versionID.Valid {
		out.ArtifactVersionID = uuidToString(versionID)
		if version, ok := versions[versionID]; ok {
			if preview, err := toCanvasProductionPreview(ctx, signer, version, assets); err == nil && preview != nil {
				out.AssetID = preview.AssetID
				out.AssetURL = preview.AccessURL
				out.ThumbnailURL = preview.ThumbnailURL
				out.Mime = preview.Mime
			}
		}
	}
	out.FinalReview = latestFinalVideoReview(reviews, plan.OutputNodeID, versionID)
	return out
}

func latestWorkbenchFinalVideoNode(nodes []db.MediaNode) (db.MediaNode, bool) {
	var out db.MediaNode
	for _, node := range nodes {
		if !isWorkbenchFinalVideoNode(node) {
			continue
		}
		if !out.ID.Valid || timestamptzTime(node.UpdatedAt).After(timestamptzTime(out.UpdatedAt)) {
			out = node
		}
	}
	return out, out.ID.Valid
}

func isWorkbenchFinalVideoNode(node db.MediaNode) bool {
	if node.NodeType != db.NodeTypeVideo {
		return false
	}
	if agentWorkbenchArtifactKind(node.Metadata) == "final_video" {
		return true
	}
	return node.OperationType == "compose_final_video"
}

func agentWorkbenchFinalOutputFromNode(
	ctx context.Context,
	signer assetURLSigner,
	node db.MediaNode,
	versions map[pgtype.UUID]db.ArtifactVersion,
	assets map[pgtype.UUID]db.MediaAsset,
	reviews []db.ReviewRecord,
) *agentWorkbenchFinalOutputResponse {
	if !node.ID.Valid {
		return nil
	}
	out := &agentWorkbenchFinalOutputResponse{
		ID:           uuidToString(node.ID),
		OutputNodeID: uuidToString(node.ID),
		Status:       agentSlotStatus(node),
		TemplateKey:  finalVideoTemplateKey(node),
		Summary:      node.Title,
		Result:       jsonObjectValue(node.Metadata),
		UpdatedAt:    timestamptzTime(node.UpdatedAt),
	}
	if node.CurrentVersionID.Valid {
		out.ArtifactVersionID = uuidToString(node.CurrentVersionID)
		if version, ok := versions[node.CurrentVersionID]; ok {
			if preview, err := toCanvasProductionPreview(ctx, signer, version, assets); err == nil && preview != nil {
				out.AssetID = preview.AssetID
				out.AssetURL = preview.AccessURL
				out.ThumbnailURL = preview.ThumbnailURL
				out.Mime = preview.Mime
			}
		}
	}
	out.FinalReview = latestFinalVideoReview(reviews, node.ID, node.CurrentVersionID)
	return out
}

func finalVideoTemplateKey(node db.MediaNode) string {
	metadata := jsonObjectValue(node.Metadata)
	value, _ := metadata["composer_template_key"].(string)
	if value == "" {
		value = "simple_concat"
	}
	return value
}

func jsonObjectValue(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func jsonAnyValue(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func agentWorkbenchAudioPlanSummary(audioPlan db.AudioPlan, nodes map[pgtype.UUID]db.MediaNode) *agentWorkbenchAudioPlanResponse {
	if !audioPlan.ID.Valid {
		return nil
	}
	out := &agentWorkbenchAudioPlanResponse{
		ID:                    uuidToString(audioPlan.ID),
		Status:                audioPlan.Status,
		Title:                 audioPlan.Title,
		PlanKind:              audioPlan.PlanKind,
		Language:              audioPlan.Language,
		VoiceoverScript:       audioPlan.VoiceoverScript,
		VoiceProfile:          jsonObjectValue(audioPlan.VoiceProfile),
		BGMPlan:               jsonObjectValue(audioPlan.BgmPlan),
		CuePlan:               jsonAnyValue(audioPlan.CuePlan),
		VoiceoverNodeID:       uuidStringForWorkbench(audioPlan.VoiceoverNodeID),
		VoiceoverStatus:       agentWorkbenchLinkedNodeStatus(audioPlan.VoiceoverNodeID, nodes),
		BGMNodeID:             uuidStringForWorkbench(audioPlan.BgmNodeID),
		BGMStatus:             agentWorkbenchLinkedNodeStatus(audioPlan.BgmNodeID, nodes),
		TimelinePlanID:        uuidStringForWorkbench(audioPlan.TimelinePlanID),
		VoiceoverRenderPlanID: uuidStringForWorkbench(audioPlan.VoiceoverRenderPlanID),
		BGMRenderPlanID:       uuidStringForWorkbench(audioPlan.BgmRenderPlanID),
	}
	if audioPlan.TargetDurationSec.Valid {
		value := audioPlan.TargetDurationSec.Float64
		out.TargetDurationSec = &value
	}
	return out
}

func agentWorkbenchLinkedNodeStatus(nodeID pgtype.UUID, nodes map[pgtype.UUID]db.MediaNode) string {
	if !nodeID.Valid {
		return "missing"
	}
	node, ok := nodes[nodeID]
	if !ok {
		return "missing"
	}
	return agentSlotStatus(node)
}

func countWorkbenchAudioPlan(audioPlan agentWorkbenchAudioPlanResponse, counts *agentWorkbenchCountsResponse) {
	if counts == nil {
		return
	}
	countWorkbenchAudioStatus(audioPlan.VoiceoverNodeID, audioPlan.VoiceoverStatus, counts)
	countWorkbenchAudioStatus(audioPlan.BGMNodeID, audioPlan.BGMStatus, counts)
}

func countWorkbenchAudioStatus(nodeID string, status string, counts *agentWorkbenchCountsResponse) {
	if nodeID == "" && status == "" {
		return
	}
	if status == "succeeded" {
		counts.AudioReady++
		return
	}
	counts.AudioMissing++
}

func agentWorkbenchAudioTracks(planJSON []byte) []agentWorkbenchAudioTrackResponse {
	plan := jsonObjectValue(planJSON)
	values, ok := plan["audio_tracks"].([]any)
	if !ok {
		return nil
	}
	out := make([]agentWorkbenchAudioTrackResponse, 0, len(values))
	for _, value := range values {
		track, ok := value.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, agentWorkbenchAudioTrackResponse{
			Role:          stringValue(track["role"]),
			AssetID:       stringValue(track["asset_id"]),
			WorkspacePath: stringValue(track["workspace_path"]),
			StartSec:      numberValue(track["start_sec"]),
			DurationSec:   numberValue(track["duration_sec"]),
			Volume:        numberValue(track["volume"]),
			Ducking:       track["ducking"] != nil,
		})
	}
	return out
}

func agentWorkbenchAudioSummary(planJSON []byte, resultJSON []byte) (*agentWorkbenchAudioSummaryResponse, bool) {
	tracks := agentWorkbenchAudioTracks(planJSON)
	plan := jsonObjectValue(planJSON)
	result := jsonObjectValue(resultJSON)
	summary := &agentWorkbenchAudioSummaryResponse{TrackCount: len(tracks)}
	for _, track := range tracks {
		switch track.Role {
		case "voiceover":
			summary.HasVoiceover = true
		case "bgm", "music":
			summary.HasBGM = true
		}
		if track.Ducking {
			summary.Ducking = true
		}
	}
	if output, ok := plan["output"].(map[string]any); ok {
		summary.AudioCodec = stringValue(output["audio_codec"])
	}
	if summary.AudioCodec == "" {
		summary.AudioCodec = stringValue(result["audio_codec"])
	}
	return summary, summary.TrackCount > 0 || summary.AudioCodec != ""
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func latestFinalVideoReview(reviews []db.ReviewRecord, outputNodeID pgtype.UUID, versionID pgtype.UUID) *agentWorkbenchReviewSummaryResponse {
	var latest db.ReviewRecord
	for _, review := range reviews {
		if review.ReviewTask != "final_video_review" && review.TargetPhase != "final_video" {
			continue
		}
		if review.NodeID.Valid && outputNodeID.Valid && review.NodeID != outputNodeID {
			continue
		}
		if review.ArtifactVersionID.Valid && versionID.Valid && review.ArtifactVersionID != versionID {
			continue
		}
		if !latest.ID.Valid || timestamptzTime(review.CreatedAt).After(timestamptzTime(latest.CreatedAt)) {
			latest = review
		}
	}
	return agentWorkbenchReviewSummary(latest)
}

func countFinalVideoReviews(reviews []db.ReviewRecord) int {
	count := 0
	for _, review := range reviews {
		if review.ReviewTask == "final_video_review" || review.TargetPhase == "final_video" {
			count++
		}
	}
	return count
}

func agentWorkbenchSourceMaterials(nodes []db.MediaNode) []agentWorkbenchSourceMaterialResponse {
	out := []agentWorkbenchSourceMaterialResponse{}
	for _, node := range nodes {
		if agentWorkbenchArtifactKind(node.Metadata) != "" {
			continue
		}
		if node.OperationType != "upload" && node.OperationType != "manual" {
			continue
		}
		out = append(out, agentWorkbenchSourceMaterialResponse{
			ID:       uuidToString(node.ID),
			Title:    node.Title,
			NodeType: string(node.NodeType),
			Status:   string(node.Status),
		})
	}
	return out
}

func agentWorkbenchScenes(
	ctx context.Context,
	signer assetURLSigner,
	scenes []db.Scene,
	shots []db.Shot,
	nodes []db.MediaNode,
	assets map[pgtype.UUID]db.MediaAsset,
	versions map[pgtype.UUID]db.ArtifactVersion,
	shotElements []db.ShotKeyElement,
	deps []db.ShotDependency,
	renderPlans []db.RenderPlan,
	reviews []db.ReviewRecord,
	issues []db.ArtifactIssue,
	counts *agentWorkbenchCountsResponse,
) []agentWorkbenchSceneResponse {
	shotsBySceneID := map[pgtype.UUID][]db.Shot{}
	knownShotIDs := map[pgtype.UUID]struct{}{}
	for _, shot := range shots {
		shotsBySceneID[shot.SceneID] = append(shotsBySceneID[shot.SceneID], shot)
		knownShotIDs[shot.ID] = struct{}{}
	}
	for sceneID := range shotsBySceneID {
		sort.SliceStable(shotsBySceneID[sceneID], func(i, j int) bool {
			left := shotsBySceneID[sceneID][i]
			right := shotsBySceneID[sceneID][j]
			if left.SortOrder != right.SortOrder {
				return left.SortOrder < right.SortOrder
			}
			if left.ClientKey != right.ClientKey {
				return left.ClientKey < right.ClientKey
			}
			return left.Title < right.Title
		})
	}

	nodesByShotID := map[pgtype.UUID][]db.MediaNode{}
	for _, node := range nodes {
		if node.ShotID.Valid {
			nodesByShotID[node.ShotID] = append(nodesByShotID[node.ShotID], node)
		}
	}

	shotElementsByShotID := map[pgtype.UUID][]db.ShotKeyElement{}
	for _, link := range shotElements {
		shotElementsByShotID[link.ShotID] = append(shotElementsByShotID[link.ShotID], link)
	}

	depsByShotID := map[pgtype.UUID][]db.ShotDependency{}
	for _, dep := range deps {
		depsByShotID[dep.FromShotID] = append(depsByShotID[dep.FromShotID], dep)
		depsByShotID[dep.ToShotID] = append(depsByShotID[dep.ToShotID], dep)
	}

	renderPlansByShotID := map[pgtype.UUID][]db.RenderPlan{}
	for _, plan := range renderPlans {
		if plan.ScopeType == "shot" {
			renderPlansByShotID[plan.ScopeID] = append(renderPlansByShotID[plan.ScopeID], plan)
		}
	}
	for shotID := range renderPlansByShotID {
		sort.SliceStable(renderPlansByShotID[shotID], func(i, j int) bool {
			left := renderPlansByShotID[shotID][i]
			right := renderPlansByShotID[shotID][j]
			if left.Revision != right.Revision {
				return left.Revision > right.Revision
			}
			return timestamptzTime(left.UpdatedAt).After(timestamptzTime(right.UpdatedAt))
		})
	}

	latestReviewByShotID := map[pgtype.UUID]db.ReviewRecord{}
	for _, review := range reviews {
		if !review.ShotID.Valid {
			continue
		}
		current, ok := latestReviewByShotID[review.ShotID]
		if !ok || timestamptzTime(review.CreatedAt).After(timestamptzTime(current.CreatedAt)) {
			latestReviewByShotID[review.ShotID] = review
		}
	}

	issuesByShotID := map[pgtype.UUID][]db.ArtifactIssue{}
	for _, issue := range issues {
		if issue.TargetObjectType != "shot" {
			continue
		}
		if _, ok := knownShotIDs[issue.TargetObjectID]; !ok {
			continue
		}
		issuesByShotID[issue.TargetObjectID] = append(issuesByShotID[issue.TargetObjectID], issue)
	}

	out := make([]agentWorkbenchSceneResponse, 0, len(scenes))
	for _, scene := range scenes {
		sceneResponse := agentWorkbenchSceneResponse{
			ID:       uuidToString(scene.ID),
			Title:    scene.Title,
			Status:   scene.Status,
			Summary:  scene.Description,
			Location: scene.Location,
			Shots:    []agentWorkbenchShotResponse{},
		}
		for _, shot := range shotsBySceneID[scene.ID] {
			preview, err := agentWorkbenchArtifactSlot(ctx, signer, "preview_image", nodesByShotID[shot.ID], assets, versions)
			if err != nil {
				preview = agentWorkbenchArtifactSlotResponse{Kind: "preview_image", Status: "failed", ErrorMessage: err.Error()}
			}
			video, err := agentWorkbenchArtifactSlot(ctx, signer, "shot_video", nodesByShotID[shot.ID], assets, versions)
			if err != nil {
				video = agentWorkbenchArtifactSlotResponse{Kind: "shot_video", Status: "failed", ErrorMessage: err.Error()}
			}
			artifacts, err := agentWorkbenchArtifactSlots(ctx, signer, nodesByShotID[shot.ID], assets, versions)
			if err != nil {
				artifacts = []agentWorkbenchArtifactSlotResponse{{
					Kind:         "preview_image",
					Status:       "failed",
					ErrorMessage: err.Error(),
				}}
			}
			countArtifactSlot(preview, true, counts)
			countArtifactSlot(video, false, counts)
			shotIssues := agentWorkbenchIssueSummaries(issuesByShotID[shot.ID])
			counts.OpenIssues += len(shotIssues)
			sceneResponse.Shots = append(sceneResponse.Shots, agentWorkbenchShotResponse{
				ID:            uuidToString(shot.ID),
				ClientKey:     shot.ClientKey,
				Title:         shot.Title,
				Status:        shot.Status,
				SequenceIndex: shot.SortOrder,
				CreativeText:  shot.CreativeText,
				Dependencies:  agentWorkbenchDependencySummaries(depsByShotID[shot.ID]),
				KeyElements:   agentWorkbenchShotKeyElementSummaries(shotElementsByShotID[shot.ID]),
				Preview:       preview,
				Video:         video,
				Artifacts:     artifacts,
				RenderPlans:   agentWorkbenchRenderPlanSummaries(renderPlansByShotID[shot.ID]),
				Review:        agentWorkbenchReviewSummary(latestReviewByShotID[shot.ID]),
				Issues:        shotIssues,
			})
		}
		out = append(out, sceneResponse)
	}
	return out
}

func agentWorkbenchDependencySummaries(deps []db.ShotDependency) []agentWorkbenchShotDependencyResponse {
	out := make([]agentWorkbenchShotDependencyResponse, 0, len(deps))
	for _, dep := range deps {
		out = append(out, agentWorkbenchShotDependencyResponse{
			ID:             uuidToString(dep.ID),
			FromShotID:     uuidToString(dep.FromShotID),
			ToShotID:       uuidToString(dep.ToShotID),
			DependencyType: dep.DependencyType,
		})
	}
	return out
}

func agentWorkbenchShotKeyElementSummaries(links []db.ShotKeyElement) []agentWorkbenchShotKeyElementRefResponse {
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].SortOrder != links[j].SortOrder {
			return links[i].SortOrder < links[j].SortOrder
		}
		return links[i].Role < links[j].Role
	})
	out := make([]agentWorkbenchShotKeyElementRefResponse, 0, len(links))
	for _, link := range links {
		out = append(out, agentWorkbenchShotKeyElementRefResponse{
			ID:                uuidToString(link.ID),
			KeyElementID:      uuidToString(link.KeyElementID),
			KeyElementStateID: uuidStringForWorkbench(link.KeyElementStateID),
			Role:              link.Role,
		})
	}
	return out
}

func agentWorkbenchRenderPlanSummaries(plans []db.RenderPlan) []agentWorkbenchRenderPlanSummaryResponse {
	out := make([]agentWorkbenchRenderPlanSummaryResponse, 0, len(plans))
	for _, plan := range plans {
		out = append(out, agentWorkbenchRenderPlanSummaryResponse{
			ID:          uuidToString(plan.ID),
			Revision:    plan.Revision,
			TargetPhase: plan.TargetPhase,
			Operation:   plan.Operation,
			Status:      plan.Status,
		})
	}
	return out
}

func agentWorkbenchReviewSummary(review db.ReviewRecord) *agentWorkbenchReviewSummaryResponse {
	if !review.ID.Valid {
		return nil
	}
	out := &agentWorkbenchReviewSummaryResponse{
		ID:          uuidToString(review.ID),
		ReviewTask:  review.ReviewTask,
		TargetPhase: review.TargetPhase,
		Status:      review.Status,
		Verdict:     review.Status,
	}
	if review.OverallScore.Valid {
		out.Score = review.OverallScore.Float32
	}
	return out
}

func agentWorkbenchIssueSummaries(issues []db.ArtifactIssue) []agentWorkbenchIssueSummaryResponse {
	out := make([]agentWorkbenchIssueSummaryResponse, 0, len(issues))
	for _, issue := range issues {
		out = append(out, agentWorkbenchIssueSummaryResponse{
			ID:           uuidToString(issue.ID),
			Title:        issue.Title,
			Severity:     issue.Severity,
			Dimension:    issue.Dimension,
			SuggestedFix: issue.SuggestedFix,
		})
	}
	return out
}

func agentWorkbenchArtifactSlot(ctx context.Context, signer assetURLSigner, kind string, nodes []db.MediaNode, assets map[pgtype.UUID]db.MediaAsset, versions map[pgtype.UUID]db.ArtifactVersion) (agentWorkbenchArtifactSlotResponse, error) {
	slot := agentWorkbenchArtifactSlotResponse{Kind: kind, Status: "missing"}
	node := bestAgentArtifactNode(nodes, kind)
	if node == nil {
		return slot, nil
	}
	return agentWorkbenchArtifactSlotFromNode(ctx, signer, *node, assets, versions)
}

func agentWorkbenchArtifactSlots(ctx context.Context, signer assetURLSigner, nodes []db.MediaNode, assets map[pgtype.UUID]db.MediaAsset, versions map[pgtype.UUID]db.ArtifactVersion) ([]agentWorkbenchArtifactSlotResponse, error) {
	candidates := agentArtifactNodes(nodes)
	out := make([]agentWorkbenchArtifactSlotResponse, 0, len(candidates))
	for _, node := range candidates {
		slot, err := agentWorkbenchArtifactSlotFromNode(ctx, signer, node, assets, versions)
		if err != nil {
			return nil, err
		}
		out = append(out, slot)
	}
	return out, nil
}

func agentWorkbenchArtifactSlotFromNode(ctx context.Context, signer assetURLSigner, node db.MediaNode, assets map[pgtype.UUID]db.MediaAsset, versions map[pgtype.UUID]db.ArtifactVersion) (agentWorkbenchArtifactSlotResponse, error) {
	slot := agentWorkbenchArtifactSlotResponse{
		Kind:   agentWorkbenchArtifactKind(node.Metadata),
		NodeID: uuidToString(node.ID),
		Title:  node.Title,
		Status: agentSlotStatus(node),
	}
	if node.CurrentVersionID.Valid {
		slot.VersionID = uuidToString(node.CurrentVersionID)
		if version, ok := versions[node.CurrentVersionID]; ok {
			preview, err := toCanvasProductionPreview(ctx, signer, version, assets)
			if err != nil {
				return slot, err
			}
			if preview != nil {
				slot.AccessURL = preview.AccessURL
				slot.ThumbnailURL = preview.ThumbnailURL
				slot.Width = preview.Width
				slot.Height = preview.Height
			}
		}
	}
	if slot.Status == "failed" {
		code, message := agentNodeError(node.Metadata)
		slot.ErrorCode = code
		slot.ErrorMessage = message
	}
	return slot, nil
}

func agentArtifactNodes(nodes []db.MediaNode) []db.MediaNode {
	var candidates []db.MediaNode
	for _, node := range nodes {
		if node.Source != "agent" {
			continue
		}
		kind := agentWorkbenchArtifactKind(node.Metadata)
		if kind != "preview_image" && kind != "shot_video" {
			continue
		}
		candidates = append(candidates, node)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftKind := artifactKindRank(agentWorkbenchArtifactKind(candidates[i].Metadata))
		rightKind := artifactKindRank(agentWorkbenchArtifactKind(candidates[j].Metadata))
		if leftKind != rightKind {
			return leftKind < rightKind
		}
		leftRank := agentArtifactRank(candidates[i])
		rightRank := agentArtifactRank(candidates[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return agentUpdatedAt(candidates[i]).After(agentUpdatedAt(candidates[j]))
	})
	return candidates
}

func bestAgentArtifactNode(nodes []db.MediaNode, artifactKind string) *db.MediaNode {
	candidates := []db.MediaNode{}
	for _, node := range agentArtifactNodes(nodes) {
		if agentWorkbenchArtifactKind(node.Metadata) == artifactKind {
			candidates = append(candidates, node)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	return &candidates[0]
}

func artifactKindRank(kind string) int {
	switch kind {
	case "preview_image":
		return 0
	case "shot_video":
		return 1
	default:
		return 2
	}
}

func agentArtifactRank(node db.MediaNode) int {
	if node.CurrentVersionID.Valid {
		return 0
	}
	switch node.Status {
	case db.NodeStatusRunning:
		return 1
	case db.NodeStatusQueued:
		return 2
	default:
		return 3
	}
}

func agentWorkbenchArtifactKind(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return ""
	}
	value, _ := metadata["agent_artifact_kind"].(string)
	return value
}

func agentSlotStatus(node db.MediaNode) string {
	switch node.Status {
	case db.NodeStatusQueued:
		return "queued"
	case db.NodeStatusRunning:
		return "running"
	case db.NodeStatusSucceeded:
		return "succeeded"
	case db.NodeStatusFailed:
		return "failed"
	case db.NodeStatusStale:
		return "stale"
	default:
		if node.CurrentVersionID.Valid {
			return "succeeded"
		}
		return "missing"
	}
}

func agentUpdatedAt(node db.MediaNode) time.Time {
	return timestamptzTime(node.UpdatedAt)
}

func countArtifactSlot(slot agentWorkbenchArtifactSlotResponse, preview bool, counts *agentWorkbenchCountsResponse) {
	if counts == nil {
		return
	}
	switch slot.Status {
	case "succeeded":
		if preview {
			counts.PreviewSucceeded++
		} else {
			counts.VideoSucceeded++
		}
	case "failed":
		if preview {
			counts.PreviewFailed++
		} else {
			counts.VideoFailed++
		}
	}
}

func agentNodeError(raw []byte) (string, string) {
	if len(raw) == 0 {
		return "", ""
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return "", ""
	}
	code, _ := metadata["error_code"].(string)
	message, _ := metadata["error_message"].(string)
	return code, message
}

func uuidStringForWorkbench(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuidToString(value)
}

func timestamptzTime(value pgtype.Timestamptz) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Time{}
}
