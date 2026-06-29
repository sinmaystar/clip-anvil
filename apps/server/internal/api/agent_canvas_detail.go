package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const (
	agentCanvasObjectOverview        = "overview"
	agentCanvasObjectKeyElement      = "key_element"
	agentCanvasObjectKeyElementState = "key_element_state"
	agentCanvasObjectScene           = "scene"
	agentCanvasObjectShot            = "shot"
	agentCanvasObjectArtifact        = "artifact"
	agentCanvasObjectRenderPlan      = "render_plan"
	agentCanvasObjectReview          = "review"
	agentCanvasObjectIssue           = "issue"
	agentCanvasObjectFinalOutput     = "final_output"
)

type agentCanvasDetailError struct {
	Status  int
	Message string
}

func (e agentCanvasDetailError) Error() string {
	return e.Message
}

func newAgentCanvasDetailError(status int, message string) error {
	return agentCanvasDetailError{Status: status, Message: message}
}

type agentCanvasDetailResponse struct {
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	Title      string `json:"title"`
	Status     string `json:"status,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`

	Overview        *agentCanvasOverviewDetailResponse        `json:"overview,omitempty"`
	KeyElement      *agentCanvasKeyElementDetailResponse      `json:"key_element,omitempty"`
	KeyElementState *agentCanvasKeyElementStateDetailResponse `json:"key_element_state,omitempty"`
	Scene           *agentCanvasSceneDetailResponse           `json:"scene,omitempty"`
	Shot            *agentCanvasShotDetailResponse            `json:"shot,omitempty"`
	Artifact        *agentCanvasArtifactDetailResponse        `json:"artifact,omitempty"`
	RenderPlan      *agentCanvasRenderPlanDetailResponse      `json:"render_plan,omitempty"`
	Review          *agentCanvasReviewDetailResponse          `json:"review,omitempty"`
	Issue           *agentCanvasIssueDetailResponse           `json:"issue,omitempty"`
	FinalOutput     *agentCanvasFinalOutputDetailResponse     `json:"final_output,omitempty"`
}

type agentCanvasOverviewDetailResponse struct {
	WorkspaceID      string                                  `json:"workspace_id"`
	Brief            *agentCanvasCreativeBriefDetailResponse `json:"brief,omitempty"`
	Memory           *agentCanvasProjectMemoryDetailResponse `json:"memory,omitempty"`
	AudioPlan        *agentWorkbenchAudioPlanResponse        `json:"audio_plan,omitempty"`
	KeyElements      []agentCanvasKeyElementSummaryResponse  `json:"key_elements"`
	KeyElementStates []agentCanvasKeyElementStateSummary     `json:"key_element_states"`
	SourceMaterials  []agentWorkbenchSourceMaterialResponse  `json:"source_materials"`
}

type agentCanvasCreativeBriefDetailResponse struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	VideoType      string `json:"video_type"`
	TargetAudience string `json:"target_audience"`
	Tone           string `json:"tone"`
	VisualStyle    string `json:"visual_style"`
	DurationSec    any    `json:"duration_sec,omitempty"`
	AspectRatio    string `json:"aspect_ratio"`
	Language       string `json:"language"`
	Objective      string `json:"objective"`
	Concept        string `json:"concept"`
	Constraints    any    `json:"constraints,omitempty"`
	Metadata       any    `json:"metadata,omitempty"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

type agentCanvasProjectMemoryDetailResponse struct {
	ID                   string `json:"id"`
	Version              int32  `json:"version"`
	Status               string `json:"status"`
	CoreIntent           string `json:"core_intent"`
	Soul                 string `json:"soul"`
	BrandFacts           any    `json:"brand_facts,omitempty"`
	NonNegotiables       any    `json:"non_negotiables,omitempty"`
	VisualAnchors        any    `json:"visual_anchors,omitempty"`
	Allowed              any    `json:"allowed,omitempty"`
	Forbidden            any    `json:"forbidden,omitempty"`
	PromptInjectionHints any    `json:"prompt_injection_hints,omitempty"`
	SourceRefs           any    `json:"source_refs,omitempty"`
}

type agentCanvasKeyElementSummaryResponse struct {
	ID          string `json:"id"`
	ClientKey   string `json:"client_key"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	SourceType  string `json:"source_type,omitempty"`
	Status      string `json:"status"`
}

type agentCanvasKeyElementStateSummary struct {
	ID                 string `json:"id"`
	KeyElementID       string `json:"key_element_id"`
	ClientKey          string `json:"client_key"`
	Label              string `json:"label"`
	VisualDescription  string `json:"visual_description,omitempty"`
	ReferenceStatus    string `json:"reference_status"`
	ReferenceNodeID    string `json:"reference_node_id,omitempty"`
	ReferenceVersionID string `json:"reference_version_id,omitempty"`
	Status             string `json:"status"`
	IsDefault          bool   `json:"is_default"`
}

type agentCanvasKeyElementDetailResponse struct {
	agentCanvasKeyElementSummaryResponse
	SourceRefs any                                  `json:"source_refs,omitempty"`
	States     []agentCanvasKeyElementStateSummary  `json:"states"`
	ShotRefs   []agentCanvasShotKeyElementRefDetail `json:"shot_refs"`
}

type agentCanvasKeyElementStateDetailResponse struct {
	agentCanvasKeyElementStateSummary
	StateFacts       any                                   `json:"state_facts,omitempty"`
	SourceRefs       any                                   `json:"source_refs,omitempty"`
	KeyElement       *agentCanvasKeyElementSummaryResponse `json:"key_element,omitempty"`
	ReferenceNode    *agentCanvasMediaNodeDetailResponse   `json:"reference_node,omitempty"`
	ReferenceVersion *artifactVersionResponse              `json:"reference_version,omitempty"`
	DependentShots   []agentCanvasShotSummaryResponse      `json:"dependent_shots"`
	MissingReason    string                                `json:"missing_reason,omitempty"`
}

type agentCanvasSceneDetailResponse struct {
	ID          string                           `json:"id"`
	ClientKey   string                           `json:"client_key"`
	SortOrder   int32                            `json:"sort_order"`
	Title       string                           `json:"title"`
	Description string                           `json:"description"`
	Location    string                           `json:"location"`
	Mood        string                           `json:"mood"`
	Status      string                           `json:"status"`
	Shots       []agentCanvasShotSummaryResponse `json:"shots"`
	UpdatedAt   string                           `json:"updated_at,omitempty"`
}

type agentCanvasShotSummaryResponse struct {
	ID            string `json:"id"`
	ClientKey     string `json:"client_key"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	SequenceIndex int32  `json:"sequence_index"`
}

type agentCanvasShotDetailResponse struct {
	ID               string                                    `json:"id"`
	ClientKey        string                                    `json:"client_key"`
	Title            string                                    `json:"title"`
	Status           string                                    `json:"status"`
	SequenceIndex    int32                                     `json:"sequence_index"`
	SceneID          string                                    `json:"scene_id,omitempty"`
	ShotKind         string                                    `json:"shot_kind,omitempty"`
	DurationSec      any                                       `json:"duration_sec,omitempty"`
	NarrativePurpose string                                    `json:"narrative_purpose,omitempty"`
	Brief            any                                       `json:"brief,omitempty"`
	CreativeText     string                                    `json:"creative_text,omitempty"`
	VisualIntent     string                                    `json:"visual_intent,omitempty"`
	ActionText       string                                    `json:"action_text,omitempty"`
	CameraIntent     string                                    `json:"camera_intent,omitempty"`
	Dialogue         string                                    `json:"dialogue,omitempty"`
	Narration        string                                    `json:"narration,omitempty"`
	AudioPlan        any                                       `json:"audio_plan,omitempty"`
	Dependencies     []agentCanvasShotDependencyDetail         `json:"dependencies"`
	KeyElements      []agentCanvasShotKeyElementRefDetail      `json:"key_elements"`
	Artifacts        []agentWorkbenchArtifactSlotResponse      `json:"artifacts"`
	RenderPlans      []agentWorkbenchRenderPlanSummaryResponse `json:"render_plans"`
	Reviews          []agentWorkbenchReviewSummaryResponse     `json:"reviews"`
	Issues           []agentWorkbenchIssueSummaryResponse      `json:"issues"`
	UpdatedAt        string                                    `json:"updated_at,omitempty"`
}

type agentCanvasShotDependencyDetail struct {
	ID               string `json:"id"`
	FromShotID       string `json:"from_shot_id"`
	ToShotID         string `json:"to_shot_id"`
	DependencyType   string `json:"dependency_type"`
	RequiredArtifact string `json:"required_artifact,omitempty"`
	InjectionRole    string `json:"injection_role,omitempty"`
	BlockingPhase    string `json:"blocking_phase,omitempty"`
	StalePolicy      string `json:"stale_policy,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type agentCanvasShotKeyElementRefDetail struct {
	ID                string `json:"id"`
	ShotID            string `json:"shot_id"`
	ShotTitle         string `json:"shot_title,omitempty"`
	KeyElementID      string `json:"key_element_id"`
	KeyElementName    string `json:"key_element_name,omitempty"`
	KeyElementStateID string `json:"key_element_state_id,omitempty"`
	StateLabel        string `json:"state_label,omitempty"`
	Role              string `json:"role"`
	Required          bool   `json:"required"`
	SortOrder         int32  `json:"sort_order"`
}

type agentCanvasArtifactDetailResponse struct {
	Node           agentCanvasMediaNodeDetailResponse        `json:"node"`
	Asset          *assetReadResponse                        `json:"asset,omitempty"`
	CurrentVersion *artifactVersionResponse                  `json:"current_version,omitempty"`
	Versions       []artifactVersionResponse                 `json:"versions"`
	GenerationJobs []generationJobResponse                   `json:"generation_jobs"`
	RenderPlans    []agentWorkbenchRenderPlanSummaryResponse `json:"render_plans"`
	Reviews        []reviewRecordResponse                    `json:"reviews"`
	Issues         []agentWorkbenchIssueSummaryResponse      `json:"issues"`
}

type agentCanvasMediaNodeDetailResponse struct {
	ID               string `json:"id"`
	WorkspaceID      string `json:"workspace_id"`
	NodeType         string `json:"node_type"`
	Title            string `json:"title"`
	Status           string `json:"status"`
	Prompt           string `json:"prompt,omitempty"`
	Source           string `json:"source,omitempty"`
	OperationType    string `json:"operation_type,omitempty"`
	ShotID           string `json:"shot_id,omitempty"`
	AssetID          string `json:"asset_id,omitempty"`
	ModelProvider    string `json:"model_provider,omitempty"`
	ModelID          string `json:"model_id,omitempty"`
	ModelParams      any    `json:"model_params,omitempty"`
	CurrentVersionID string `json:"current_version_id,omitempty"`
	Metadata         any    `json:"metadata,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

type agentCanvasRenderPlanDetailResponse struct {
	ID                    string                               `json:"id"`
	ScopeType             string                               `json:"scope_type"`
	ScopeID               string                               `json:"scope_id"`
	TargetPhase           string                               `json:"target_phase"`
	TaskType              string                               `json:"task_type"`
	ModelPromptProfile    string                               `json:"model_prompt_profile"`
	Operation             string                               `json:"operation"`
	Status                string                               `json:"status"`
	Revision              int32                                `json:"revision"`
	RenderPlanKey         string                               `json:"render_plan_key,omitempty"`
	ReferenceBindings     any                                  `json:"reference_bindings,omitempty"`
	SubjectBindings       any                                  `json:"subject_bindings,omitempty"`
	PromptParts           any                                  `json:"prompt_parts,omitempty"`
	Params                any                                  `json:"params,omitempty"`
	AuditHints            any                                  `json:"audit_hints,omitempty"`
	Blocker               any                                  `json:"blocker,omitempty"`
	CompiledPrompt        string                               `json:"compiled_prompt,omitempty"`
	CompiledRequest       any                                  `json:"compiled_request,omitempty"`
	PromptAudit           any                                  `json:"prompt_audit,omitempty"`
	CostEstimate          any                                  `json:"cost_estimate,omitempty"`
	Rationale             string                               `json:"rationale,omitempty"`
	SubmittedWorkerTaskID string                               `json:"submitted_worker_task_id,omitempty"`
	OutputNode            *agentCanvasMediaNodeDetailResponse  `json:"output_node,omitempty"`
	OutputVersion         *artifactVersionResponse             `json:"output_version,omitempty"`
	Reviews               []reviewRecordResponse               `json:"reviews"`
	Issues                []agentWorkbenchIssueSummaryResponse `json:"issues"`
	CreatedAt             string                               `json:"created_at,omitempty"`
	UpdatedAt             string                               `json:"updated_at,omitempty"`
	CompiledAt            string                               `json:"compiled_at,omitempty"`
	SubmittedAt           string                               `json:"submitted_at,omitempty"`
	CompletedAt           string                               `json:"completed_at,omitempty"`
}

type agentCanvasReviewDetailResponse struct {
	Review reviewRecordResponse                 `json:"review"`
	Issues []agentWorkbenchIssueSummaryResponse `json:"issues"`
}

type agentCanvasIssueDetailResponse struct {
	ID                       string                `json:"id"`
	ReviewRecordID           string                `json:"review_record_id,omitempty"`
	Dimension                string                `json:"dimension"`
	Severity                 string                `json:"severity"`
	Status                   string                `json:"status"`
	TargetObjectType         string                `json:"target_object_type"`
	TargetObjectID           string                `json:"target_object_id"`
	Title                    string                `json:"title"`
	Description              string                `json:"description"`
	Evidence                 string                `json:"evidence,omitempty"`
	SuggestedFix             string                `json:"suggested_fix,omitempty"`
	FixHint                  string                `json:"fix_hint,omitempty"`
	RequiresUserConfirmation bool                  `json:"requires_user_confirmation"`
	Review                   *reviewRecordResponse `json:"review,omitempty"`
	CreatedAt                string                `json:"created_at,omitempty"`
	UpdatedAt                string                `json:"updated_at,omitempty"`
}

type agentCanvasFinalOutputDetailResponse struct {
	TimelinePlanID    string                                `json:"timeline_plan_id"`
	OutputNode        *agentCanvasMediaNodeDetailResponse   `json:"output_node,omitempty"`
	OutputVersion     *artifactVersionResponse              `json:"output_version,omitempty"`
	ProductionJobID   string                                `json:"production_job_id,omitempty"`
	ArtifactVersionID string                                `json:"artifact_version_id,omitempty"`
	SandboxJobID      string                                `json:"sandbox_job_id,omitempty"`
	Status            string                                `json:"status"`
	TemplateKey       string                                `json:"template_key"`
	AudioSummary      *agentWorkbenchAudioSummaryResponse   `json:"audio_summary,omitempty"`
	AudioTracks       []agentWorkbenchAudioTrackResponse    `json:"audio_tracks,omitempty"`
	FinalReviews      []agentWorkbenchReviewSummaryResponse `json:"final_reviews"`
	Issues            []agentWorkbenchIssueSummaryResponse  `json:"issues"`
	Plan              map[string]any                        `json:"plan,omitempty"`
	Result            map[string]any                        `json:"result,omitempty"`
	ErrorMessage      string                                `json:"error_message,omitempty"`
	CreatedAt         string                                `json:"created_at,omitempty"`
	UpdatedAt         string                                `json:"updated_at,omitempty"`
}

func buildAgentCanvasDetail(ctx context.Context, queries *db.Queries, signer assetURLSigner, workspaceID pgtype.UUID, objectType string, objectID string) (agentCanvasDetailResponse, error) {
	if queries == nil || !workspaceID.Valid {
		return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusNotFound, "workspace not found")
	}
	if objectType == "" {
		return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusBadRequest, "object_type is required")
	}
	if objectType == agentCanvasObjectOverview {
		if objectID != "" && objectID != uuidToString(workspaceID) {
			return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusBadRequest, "overview object_id must be empty or workspace id")
		}
		return buildAgentCanvasOverviewDetail(ctx, queries, workspaceID)
	}

	id, ok := uuidFromString(objectID)
	if !ok {
		return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusBadRequest, "object_id must be a uuid")
	}

	switch objectType {
	case agentCanvasObjectKeyElement:
		return buildAgentCanvasKeyElementDetail(ctx, queries, workspaceID, id)
	case agentCanvasObjectKeyElementState:
		return buildAgentCanvasKeyElementStateDetail(ctx, queries, signer, workspaceID, id)
	case agentCanvasObjectScene:
		return buildAgentCanvasSceneDetail(ctx, queries, workspaceID, id)
	case agentCanvasObjectShot:
		return buildAgentCanvasShotDetail(ctx, queries, signer, workspaceID, id)
	case agentCanvasObjectArtifact:
		return buildAgentCanvasArtifactDetail(ctx, queries, signer, workspaceID, id)
	case agentCanvasObjectRenderPlan:
		return buildAgentCanvasRenderPlanDetail(ctx, queries, signer, workspaceID, id)
	case agentCanvasObjectReview:
		return buildAgentCanvasReviewDetail(ctx, queries, workspaceID, id)
	case agentCanvasObjectIssue:
		return buildAgentCanvasIssueDetail(ctx, queries, workspaceID, id)
	case agentCanvasObjectFinalOutput:
		return buildAgentCanvasFinalOutputDetail(ctx, queries, signer, workspaceID, id)
	default:
		return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusBadRequest, "unsupported object_type")
	}
}

func buildAgentCanvasOverviewDetail(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID) (agentCanvasDetailResponse, error) {
	detail := agentCanvasOverviewDetailResponse{
		WorkspaceID:      uuidToString(workspaceID),
		KeyElements:      []agentCanvasKeyElementSummaryResponse{},
		KeyElementStates: []agentCanvasKeyElementStateSummary{},
		SourceMaterials:  []agentWorkbenchSourceMaterialResponse{},
	}
	title := "Project Overview"
	status := "active"
	if brief, ok, err := activeCreativeBrief(ctx, queries, workspaceID); err != nil {
		return agentCanvasDetailResponse{}, err
	} else if ok {
		title = brief.Title
		status = brief.Status
		detail.Brief = &agentCanvasCreativeBriefDetailResponse{
			ID:             uuidToString(brief.ID),
			Title:          brief.Title,
			VideoType:      brief.VideoType,
			TargetAudience: brief.TargetAudience,
			Tone:           brief.Tone,
			VisualStyle:    brief.VisualStyle,
			DurationSec:    floatValue(brief.DurationSec),
			AspectRatio:    brief.AspectRatio,
			Language:       brief.Language,
			Objective:      brief.Objective,
			Concept:        brief.Concept,
			Constraints:    jsonValue(brief.Constraints),
			Metadata:       jsonValue(brief.Metadata),
			Status:         brief.Status,
			UpdatedAt:      timeString(brief.UpdatedAt),
		}
	}
	if memory, ok, err := activeProjectMemory(ctx, queries, workspaceID); err != nil {
		return agentCanvasDetailResponse{}, err
	} else if ok {
		detail.Memory = &agentCanvasProjectMemoryDetailResponse{
			ID:                   uuidToString(memory.ID),
			Version:              memory.Version,
			Status:               memory.Status,
			CoreIntent:           memory.CoreIntent,
			Soul:                 memory.Soul,
			BrandFacts:           jsonValue(memory.BrandFacts),
			NonNegotiables:       jsonValue(memory.NonNegotiables),
			VisualAnchors:        jsonValue(memory.VisualAnchors),
			Allowed:              jsonValue(memory.Allowed),
			Forbidden:            jsonValue(memory.Forbidden),
			PromptInjectionHints: jsonValue(memory.PromptInjectionHints),
			SourceRefs:           jsonValue(memory.SourceRefs),
		}
	}
	elements, err := queries.ListActiveKeyElementsByWorkspace(ctx, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	for _, element := range elements {
		detail.KeyElements = append(detail.KeyElements, keyElementSummary(element))
	}
	states, err := queries.ListActiveKeyElementStatesByWorkspace(ctx, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	for _, state := range states {
		detail.KeyElementStates = append(detail.KeyElementStates, keyElementStateSummary(state))
	}
	nodes, err := queries.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	detail.SourceMaterials = agentWorkbenchSourceMaterials(nodes)
	nodesByID := make(map[pgtype.UUID]db.MediaNode, len(nodes))
	for _, node := range nodes {
		nodesByID[node.ID] = node
	}
	if audioPlan, ok, err := activeWorkbenchAudioPlan(ctx, queries, workspaceID); err != nil {
		return agentCanvasDetailResponse{}, err
	} else if ok {
		detail.AudioPlan = agentWorkbenchAudioPlanSummary(audioPlan, nodesByID)
	}
	return agentCanvasDetailResponse{
		ObjectType: agentCanvasObjectOverview,
		ObjectID:   uuidToString(workspaceID),
		Title:      title,
		Status:     status,
		Overview:   &detail,
	}, nil
}

func buildAgentCanvasFinalOutputDetail(ctx context.Context, queries *db.Queries, signer assetURLSigner, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	plan, err := queries.GetTimelinePlan(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return buildAgentCanvasFinalOutputNodeDetail(ctx, queries, signer, workspaceID, id)
		}
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	if plan.WorkspaceID != workspaceID {
		return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusNotFound, "object not found")
	}
	out := agentCanvasFinalOutputDetailResponse{
		TimelinePlanID:    uuidToString(plan.ID),
		ProductionJobID:   uuidString(plan.ProductionJobID),
		ArtifactVersionID: uuidString(plan.ArtifactVersionID),
		SandboxJobID:      uuidString(plan.SandboxJobID),
		Status:            plan.Status,
		TemplateKey:       plan.TemplateKey,
		Plan:              jsonObjectValue(plan.PlanJson),
		Result:            jsonObjectValue(plan.Result),
		ErrorMessage:      textString(plan.ErrorMessage),
		CreatedAt:         timeString(plan.CreatedAt),
		UpdatedAt:         timeString(plan.UpdatedAt),
	}
	out.AudioTracks = agentWorkbenchAudioTracks(plan.PlanJson)
	if audioSummary, ok := agentWorkbenchAudioSummary(plan.PlanJson, plan.Result); ok {
		out.AudioSummary = audioSummary
	}
	title := "Final Output"
	if plan.OutputNodeID.Valid {
		if node, err := queries.GetMediaNodeByID(ctx, plan.OutputNodeID); err == nil && node.WorkspaceID == workspaceID {
			nodeDetail := mediaNodeDetail(node)
			out.OutputNode = &nodeDetail
			title = node.Title
			if !plan.ArtifactVersionID.Valid {
				plan.ArtifactVersionID = node.CurrentVersionID
			}
		}
	}
	if plan.ArtifactVersionID.Valid {
		if version, err := artifactVersionWithAsset(ctx, queries, signer, plan.ArtifactVersionID); err == nil && version.ID != "" {
			out.OutputVersion = &version
			out.ArtifactVersionID = version.ID
		}
	}
	reviews, err := queries.ListReviewRecordsByWorkspace(ctx, db.ListReviewRecordsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out.FinalReviews = agentCanvasFinalReviewSummaries(reviews, plan.ID, plan.OutputNodeID, plan.ArtifactVersionID)
	issues, err := queries.ListOpenArtifactIssuesByWorkspace(ctx, db.ListOpenArtifactIssuesByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out.Issues = agentCanvasFinalOutputIssueSummaries(issues, plan.ID, plan.OutputNodeID, plan.ArtifactVersionID)
	return agentCanvasDetailResponse{
		ObjectType:  agentCanvasObjectFinalOutput,
		ObjectID:    uuidToString(plan.ID),
		Title:       title,
		Status:      plan.Status,
		UpdatedAt:   timeString(plan.UpdatedAt),
		FinalOutput: &out,
	}, nil
}

func buildAgentCanvasFinalOutputNodeDetail(ctx context.Context, queries *db.Queries, signer assetURLSigner, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	node, err := queries.GetMediaNodeByID(ctx, id)
	if err != nil {
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	if node.WorkspaceID != workspaceID || !isWorkbenchFinalVideoNode(node) {
		return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusNotFound, "object not found")
	}
	nodeDetail := mediaNodeDetail(node)
	out := agentCanvasFinalOutputDetailResponse{
		OutputNode:  &nodeDetail,
		Status:      agentSlotStatus(node),
		TemplateKey: finalVideoTemplateKey(node),
		Result:      jsonObjectValue(node.Metadata),
		CreatedAt:   timeString(node.CreatedAt),
		UpdatedAt:   timeString(node.UpdatedAt),
	}
	if node.CurrentVersionID.Valid {
		if version, err := artifactVersionWithAsset(ctx, queries, signer, node.CurrentVersionID); err == nil && version.ID != "" {
			out.OutputVersion = &version
			out.ArtifactVersionID = version.ID
		}
	}
	reviews, err := queries.ListReviewRecordsByWorkspace(ctx, db.ListReviewRecordsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out.FinalReviews = agentCanvasFinalReviewSummaries(reviews, pgtype.UUID{}, node.ID, node.CurrentVersionID)
	issues, err := queries.ListOpenArtifactIssuesByWorkspace(ctx, db.ListOpenArtifactIssuesByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out.Issues = agentCanvasFinalOutputIssueSummaries(issues, pgtype.UUID{}, node.ID, node.CurrentVersionID)
	return agentCanvasDetailResponse{
		ObjectType:  agentCanvasObjectFinalOutput,
		ObjectID:    uuidToString(node.ID),
		Title:       node.Title,
		Status:      out.Status,
		UpdatedAt:   timeString(node.UpdatedAt),
		FinalOutput: &out,
	}, nil
}

func buildAgentCanvasKeyElementDetail(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	element, err := queries.GetAgentCanvasKeyElementByID(ctx, db.GetAgentCanvasKeyElementByIDParams{ID: id, WorkspaceID: workspaceID})
	if err != nil {
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	states, err := queries.ListActiveKeyElementStatesByWorkspace(ctx, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	links, shotsByID, statesByID, elementsByID, err := shotElementContext(ctx, queries, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out := agentCanvasKeyElementDetailResponse{
		agentCanvasKeyElementSummaryResponse: keyElementSummary(element),
		SourceRefs:                           jsonValue(element.SourceRefs),
		States:                               []agentCanvasKeyElementStateSummary{},
		ShotRefs:                             []agentCanvasShotKeyElementRefDetail{},
	}
	for _, state := range states {
		if state.KeyElementID == element.ID {
			out.States = append(out.States, keyElementStateSummary(state))
		}
	}
	for _, link := range links {
		if link.KeyElementID == element.ID {
			out.ShotRefs = append(out.ShotRefs, shotKeyElementRefDetail(link, shotsByID, elementsByID, statesByID))
		}
	}
	return agentCanvasDetailResponse{
		ObjectType: agentCanvasObjectKeyElement,
		ObjectID:   uuidToString(element.ID),
		Title:      element.Name,
		Status:     element.Status,
		UpdatedAt:  timeString(element.UpdatedAt),
		KeyElement: &out,
	}, nil
}

func buildAgentCanvasKeyElementStateDetail(ctx context.Context, queries *db.Queries, signer assetURLSigner, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	state, err := queries.GetKeyElementStateByID(ctx, db.GetKeyElementStateByIDParams{ID: id, WorkspaceID: workspaceID})
	if err != nil {
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	links, shotsByID, statesByID, elementsByID, err := shotElementContext(ctx, queries, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out := agentCanvasKeyElementStateDetailResponse{
		agentCanvasKeyElementStateSummary: keyElementStateSummary(state),
		StateFacts:                        jsonValue(state.StateFacts),
		SourceRefs:                        jsonValue(state.SourceRefs),
		DependentShots:                    []agentCanvasShotSummaryResponse{},
	}
	if element, ok := elementsByID[state.KeyElementID]; ok {
		summary := keyElementSummary(element)
		out.KeyElement = &summary
	}
	if state.ReferenceStatus == "needs_reference" {
		out.MissingReason = "该元素状态还没有可复用的参考资源，依赖它的分镜在生成时可能无法保持一致性。"
	}
	if state.ReferenceNodeID.Valid {
		if node, err := queries.GetMediaNodeByID(ctx, state.ReferenceNodeID); err == nil && node.WorkspaceID == workspaceID {
			nodeDetail := mediaNodeDetail(node)
			out.ReferenceNode = &nodeDetail
		}
	}
	if state.ReferenceVersionID.Valid {
		if version, err := artifactVersionWithAsset(ctx, queries, signer, state.ReferenceVersionID); err == nil && version.ID != "" {
			out.ReferenceVersion = &version
		}
	}
	for _, link := range links {
		if link.KeyElementStateID == state.ID {
			if shot, ok := shotsByID[link.ShotID]; ok {
				out.DependentShots = append(out.DependentShots, shotSummary(shot))
			}
		}
	}
	_ = statesByID
	return agentCanvasDetailResponse{
		ObjectType:      agentCanvasObjectKeyElementState,
		ObjectID:        uuidToString(state.ID),
		Title:           state.Label,
		Status:          state.ReferenceStatus,
		UpdatedAt:       timeString(state.UpdatedAt),
		KeyElementState: &out,
	}, nil
}

func buildAgentCanvasSceneDetail(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	scene, err := queries.GetAgentCanvasSceneByID(ctx, db.GetAgentCanvasSceneByIDParams{ID: id, WorkspaceID: workspaceID})
	if err != nil {
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	shots, err := queries.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out := agentCanvasSceneDetailResponse{
		ID:          uuidToString(scene.ID),
		ClientKey:   scene.ClientKey,
		SortOrder:   scene.SortOrder,
		Title:       scene.Title,
		Description: scene.Description,
		Location:    scene.Location,
		Mood:        scene.Mood,
		Status:      scene.Status,
		Shots:       []agentCanvasShotSummaryResponse{},
		UpdatedAt:   timeString(scene.UpdatedAt),
	}
	for _, shot := range shots {
		if shot.SceneID == scene.ID {
			out.Shots = append(out.Shots, shotSummary(shot))
		}
	}
	return agentCanvasDetailResponse{
		ObjectType: agentCanvasObjectScene,
		ObjectID:   uuidToString(scene.ID),
		Title:      scene.Title,
		Status:     scene.Status,
		UpdatedAt:  timeString(scene.UpdatedAt),
		Scene:      &out,
	}, nil
}

func buildAgentCanvasShotDetail(ctx context.Context, queries *db.Queries, signer assetURLSigner, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	shot, err := queries.GetShotByID(ctx, id)
	if err != nil {
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	if shot.WorkspaceID != workspaceID {
		return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusNotFound, "object not found")
	}
	nodes, err := queries.ListMediaNodesByShot(ctx, db.ListMediaNodesByShotParams{WorkspaceID: workspaceID, ShotID: shot.ID})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	assets, err := queries.ListMediaAssetsByWorkspace(ctx, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	assetsByID := make(map[pgtype.UUID]db.MediaAsset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.ID] = asset
	}
	versionsByID := map[pgtype.UUID]db.ArtifactVersion{}
	for _, node := range nodes {
		if !node.CurrentVersionID.Valid {
			continue
		}
		if version, err := queries.GetArtifactVersionByID(ctx, node.CurrentVersionID); err == nil {
			versionsByID[node.CurrentVersionID] = version
		}
	}
	artifacts, err := agentWorkbenchArtifactSlots(ctx, signer, nodes, assetsByID, versionsByID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	deps, err := queries.ListShotDependenciesByWorkspace(ctx, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	links, shotsByID, statesByID, elementsByID, err := shotElementContext(ctx, queries, workspaceID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	renderPlans, err := queries.ListRenderPlansByScope(ctx, db.ListRenderPlansByScopeParams{WorkspaceID: workspaceID, ScopeType: "shot", ScopeID: shot.ID})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	reviews, err := queries.ListReviewRecordsByTarget(ctx, db.ListReviewRecordsByTargetParams{WorkspaceID: workspaceID, TargetObjectType: "shot", TargetObjectID: shot.ID, Limit: 50})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	issues, err := queries.ListArtifactIssuesByTarget(ctx, db.ListArtifactIssuesByTargetParams{TargetObjectType: "shot", TargetObjectID: shot.ID})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out := agentCanvasShotDetailResponse{
		ID:               uuidToString(shot.ID),
		ClientKey:        shot.ClientKey,
		Title:            shot.Title,
		Status:           shot.Status,
		SequenceIndex:    shot.SortOrder,
		SceneID:          uuidString(shot.SceneID),
		ShotKind:         shot.ShotKind,
		DurationSec:      floatValue(shot.DurationSec),
		NarrativePurpose: shot.NarrativePurpose,
		Brief:            jsonValue(shot.Brief),
		CreativeText:     shot.CreativeText,
		VisualIntent:     shot.VisualIntent,
		ActionText:       shot.ActionText,
		CameraIntent:     shot.CameraIntent,
		Dialogue:         shot.Dialogue,
		Narration:        shot.Narration,
		AudioPlan:        jsonValue(shot.AudioPlan),
		Dependencies:     []agentCanvasShotDependencyDetail{},
		KeyElements:      []agentCanvasShotKeyElementRefDetail{},
		Artifacts:        artifacts,
		RenderPlans:      agentWorkbenchRenderPlanSummaries(renderPlans),
		Reviews:          []agentWorkbenchReviewSummaryResponse{},
		Issues:           agentWorkbenchIssueSummaries(issues),
		UpdatedAt:        timeString(shot.UpdatedAt),
	}
	for _, dep := range deps {
		if dep.FromShotID == shot.ID || dep.ToShotID == shot.ID {
			out.Dependencies = append(out.Dependencies, shotDependencyDetail(dep))
		}
	}
	for _, link := range links {
		if link.ShotID == shot.ID {
			out.KeyElements = append(out.KeyElements, shotKeyElementRefDetail(link, shotsByID, elementsByID, statesByID))
		}
	}
	for _, review := range reviews {
		if summary := agentWorkbenchReviewSummary(review); summary != nil {
			out.Reviews = append(out.Reviews, *summary)
		}
	}
	return agentCanvasDetailResponse{
		ObjectType: agentCanvasObjectShot,
		ObjectID:   uuidToString(shot.ID),
		Title:      shot.Title,
		Status:     shot.Status,
		UpdatedAt:  timeString(shot.UpdatedAt),
		Shot:       &out,
	}, nil
}

func buildAgentCanvasArtifactDetail(ctx context.Context, queries *db.Queries, signer assetURLSigner, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	node, err := queries.GetMediaNodeByID(ctx, id)
	if err != nil {
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	if node.WorkspaceID != workspaceID {
		return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusNotFound, "object not found")
	}
	out := agentCanvasArtifactDetailResponse{
		Node:           mediaNodeDetail(node),
		Versions:       []artifactVersionResponse{},
		GenerationJobs: []generationJobResponse{},
		RenderPlans:    []agentWorkbenchRenderPlanSummaryResponse{},
		Reviews:        []reviewRecordResponse{},
		Issues:         []agentWorkbenchIssueSummaryResponse{},
	}
	if node.AssetID.Valid {
		if asset, err := assetReadWithAccess(ctx, queries, signer, node.AssetID); err == nil {
			out.Asset = &asset
		}
	}
	versions, err := queries.ListArtifactVersionsByNode(ctx, node.ID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	for _, version := range versions {
		if version.WorkspaceID != workspaceID {
			continue
		}
		versionResp, err := artifactVersionResponseWithAsset(ctx, queries, signer, version)
		if err != nil {
			return agentCanvasDetailResponse{}, err
		}
		out.Versions = append(out.Versions, versionResp)
		if node.CurrentVersionID.Valid && version.ID == node.CurrentVersionID {
			current := versionResp
			out.CurrentVersion = &current
		}
	}
	jobs, err := queries.ListGenerationJobsByNode(ctx, node.ID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	for _, job := range jobs {
		if job.WorkspaceID == workspaceID {
			out.GenerationJobs = append(out.GenerationJobs, toGenerationJobResponse(job))
		}
	}
	if node.ShotID.Valid {
		plans, err := queries.ListRenderPlansByScope(ctx, db.ListRenderPlansByScopeParams{WorkspaceID: workspaceID, ScopeType: "shot", ScopeID: node.ShotID})
		if err != nil {
			return agentCanvasDetailResponse{}, err
		}
		for _, plan := range plans {
			if plan.OutputNodeID == node.ID {
				out.RenderPlans = append(out.RenderPlans, agentWorkbenchRenderPlanSummaries([]db.RenderPlan{plan})...)
			}
		}
	}
	reviews, err := queries.ListReviewRecordsByNode(ctx, node.ID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	for _, review := range reviews {
		if review.WorkspaceID == workspaceID {
			out.Reviews = append(out.Reviews, toReviewRecordResponse(review))
		}
	}
	issues, err := queries.ListArtifactIssuesByTarget(ctx, db.ListArtifactIssuesByTargetParams{TargetObjectType: "media_node", TargetObjectID: node.ID})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out.Issues = agentWorkbenchIssueSummaries(filterIssuesByWorkspace(issues, workspaceID))
	return agentCanvasDetailResponse{
		ObjectType: agentCanvasObjectArtifact,
		ObjectID:   uuidToString(node.ID),
		Title:      node.Title,
		Status:     string(node.Status),
		UpdatedAt:  timeString(node.UpdatedAt),
		Artifact:   &out,
	}, nil
}

func buildAgentCanvasRenderPlanDetail(ctx context.Context, queries *db.Queries, signer assetURLSigner, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	plan, err := queries.GetRenderPlanByID(ctx, db.GetRenderPlanByIDParams{ID: id, WorkspaceID: workspaceID})
	if err != nil {
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	out := agentCanvasRenderPlanDetailResponse{
		ID:                    uuidToString(plan.ID),
		ScopeType:             plan.ScopeType,
		ScopeID:               uuidToString(plan.ScopeID),
		TargetPhase:           plan.TargetPhase,
		TaskType:              plan.TaskType,
		ModelPromptProfile:    plan.ModelPromptProfile,
		Operation:             plan.Operation,
		Status:                plan.Status,
		Revision:              plan.Revision,
		RenderPlanKey:         plan.RenderPlanKey,
		ReferenceBindings:     jsonValue(plan.ReferenceBindings),
		SubjectBindings:       jsonValue(plan.SubjectBindings),
		PromptParts:           jsonValue(plan.PromptParts),
		Params:                jsonValue(plan.Params),
		AuditHints:            jsonValue(plan.AuditHints),
		Blocker:               jsonValue(plan.Blocker),
		CompiledPrompt:        plan.CompiledPrompt,
		CompiledRequest:       jsonValue(plan.CompiledRequest),
		PromptAudit:           jsonValue(plan.PromptAudit),
		CostEstimate:          jsonValue(plan.CostEstimate),
		Rationale:             plan.Rationale,
		SubmittedWorkerTaskID: uuidString(plan.SubmittedWorkerTaskID),
		Reviews:               []reviewRecordResponse{},
		Issues:                []agentWorkbenchIssueSummaryResponse{},
		CreatedAt:             timeString(plan.CreatedAt),
		UpdatedAt:             timeString(plan.UpdatedAt),
		CompiledAt:            timeString(plan.CompiledAt),
		SubmittedAt:           timeString(plan.SubmittedAt),
		CompletedAt:           timeString(plan.CompletedAt),
	}
	if plan.OutputNodeID.Valid {
		if node, err := queries.GetMediaNodeByID(ctx, plan.OutputNodeID); err == nil && node.WorkspaceID == workspaceID {
			nodeDetail := mediaNodeDetail(node)
			out.OutputNode = &nodeDetail
		}
	}
	if plan.OutputVersionID.Valid {
		if version, err := artifactVersionWithAsset(ctx, queries, signer, plan.OutputVersionID); err == nil && version.ID != "" {
			out.OutputVersion = &version
		}
	}
	reviews, err := queries.ListReviewRecordsByRenderPlan(ctx, plan.ID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	for _, review := range reviews {
		if review.WorkspaceID == workspaceID {
			out.Reviews = append(out.Reviews, toReviewRecordResponse(review))
		}
	}
	issues, err := queries.ListArtifactIssuesByTarget(ctx, db.ListArtifactIssuesByTargetParams{TargetObjectType: "render_plan", TargetObjectID: plan.ID})
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out.Issues = agentWorkbenchIssueSummaries(filterIssuesByWorkspace(issues, workspaceID))
	return agentCanvasDetailResponse{
		ObjectType: agentCanvasObjectRenderPlan,
		ObjectID:   uuidToString(plan.ID),
		Title:      renderPlanTitle(plan),
		Status:     plan.Status,
		UpdatedAt:  timeString(plan.UpdatedAt),
		RenderPlan: &out,
	}, nil
}

func buildAgentCanvasReviewDetail(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	review, err := queries.GetReviewRecordByID(ctx, id)
	if err != nil {
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	if review.WorkspaceID != workspaceID {
		return agentCanvasDetailResponse{}, newAgentCanvasDetailError(http.StatusNotFound, "object not found")
	}
	issues, err := queries.ListArtifactIssuesByReviewRecord(ctx, review.ID)
	if err != nil {
		return agentCanvasDetailResponse{}, err
	}
	out := agentCanvasReviewDetailResponse{
		Review: toReviewRecordResponse(review),
		Issues: agentWorkbenchIssueSummaries(filterIssuesByWorkspace(issues, workspaceID)),
	}
	return agentCanvasDetailResponse{
		ObjectType: agentCanvasObjectReview,
		ObjectID:   uuidToString(review.ID),
		Title:      review.ReviewTask,
		Status:     review.Status,
		UpdatedAt:  timeString(review.CompletedAt),
		Review:     &out,
	}, nil
}

func buildAgentCanvasIssueDetail(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID, id pgtype.UUID) (agentCanvasDetailResponse, error) {
	issue, err := queries.GetAgentCanvasArtifactIssueByID(ctx, db.GetAgentCanvasArtifactIssueByIDParams{ID: id, WorkspaceID: workspaceID})
	if err != nil {
		return agentCanvasDetailResponse{}, detailNotFound(err)
	}
	out := agentCanvasIssueDetailResponse{
		ID:                       uuidToString(issue.ID),
		ReviewRecordID:           uuidString(issue.ReviewRecordID),
		Dimension:                issue.Dimension,
		Severity:                 issue.Severity,
		Status:                   issue.Status,
		TargetObjectType:         issue.TargetObjectType,
		TargetObjectID:           uuidToString(issue.TargetObjectID),
		Title:                    issue.Title,
		Description:              issue.Description,
		Evidence:                 issue.Evidence,
		SuggestedFix:             issue.SuggestedFix,
		FixHint:                  issue.FixHint,
		RequiresUserConfirmation: issue.RequiresUserConfirmation,
		CreatedAt:                timeString(issue.CreatedAt),
		UpdatedAt:                timeString(issue.UpdatedAt),
	}
	if issue.ReviewRecordID.Valid {
		if review, err := queries.GetReviewRecordByID(ctx, issue.ReviewRecordID); err == nil && review.WorkspaceID == workspaceID {
			reviewResp := toReviewRecordResponse(review)
			out.Review = &reviewResp
		}
	}
	return agentCanvasDetailResponse{
		ObjectType: agentCanvasObjectIssue,
		ObjectID:   uuidToString(issue.ID),
		Title:      issue.Title,
		Status:     issue.Status,
		UpdatedAt:  timeString(issue.UpdatedAt),
		Issue:      &out,
	}, nil
}

func detailNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return newAgentCanvasDetailError(http.StatusNotFound, "object not found")
	}
	return err
}

func keyElementSummary(element db.KeyElement) agentCanvasKeyElementSummaryResponse {
	return agentCanvasKeyElementSummaryResponse{
		ID:          uuidToString(element.ID),
		ClientKey:   element.ClientKey,
		Name:        element.Name,
		Type:        element.ElementType,
		Description: element.Description,
		SourceType:  element.SourceType,
		Status:      element.Status,
	}
}

func keyElementStateSummary(state db.KeyElementState) agentCanvasKeyElementStateSummary {
	return agentCanvasKeyElementStateSummary{
		ID:                 uuidToString(state.ID),
		KeyElementID:       uuidToString(state.KeyElementID),
		ClientKey:          state.ClientKey,
		Label:              state.Label,
		VisualDescription:  state.VisualDescription,
		ReferenceStatus:    state.ReferenceStatus,
		ReferenceNodeID:    uuidString(state.ReferenceNodeID),
		ReferenceVersionID: uuidString(state.ReferenceVersionID),
		Status:             state.Status,
		IsDefault:          state.IsDefault,
	}
}

func shotSummary(shot db.Shot) agentCanvasShotSummaryResponse {
	return agentCanvasShotSummaryResponse{
		ID:            uuidToString(shot.ID),
		ClientKey:     shot.ClientKey,
		Title:         shot.Title,
		Status:        shot.Status,
		SequenceIndex: shot.SortOrder,
	}
}

func shotDependencyDetail(dep db.ShotDependency) agentCanvasShotDependencyDetail {
	return agentCanvasShotDependencyDetail{
		ID:               uuidToString(dep.ID),
		FromShotID:       uuidToString(dep.FromShotID),
		ToShotID:         uuidToString(dep.ToShotID),
		DependencyType:   dep.DependencyType,
		RequiredArtifact: dep.RequiredArtifact,
		InjectionRole:    dep.InjectionRole,
		BlockingPhase:    dep.BlockingPhase,
		StalePolicy:      dep.StalePolicy,
		Reason:           dep.Reason,
	}
}

func shotKeyElementRefDetail(link db.ShotKeyElement, shots map[pgtype.UUID]db.Shot, elements map[pgtype.UUID]db.KeyElement, states map[pgtype.UUID]db.KeyElementState) agentCanvasShotKeyElementRefDetail {
	out := agentCanvasShotKeyElementRefDetail{
		ID:                uuidToString(link.ID),
		ShotID:            uuidToString(link.ShotID),
		KeyElementID:      uuidToString(link.KeyElementID),
		KeyElementStateID: uuidString(link.KeyElementStateID),
		Role:              link.Role,
		Required:          link.Required,
		SortOrder:         link.SortOrder,
	}
	if shot, ok := shots[link.ShotID]; ok {
		out.ShotTitle = shot.Title
	}
	if element, ok := elements[link.KeyElementID]; ok {
		out.KeyElementName = element.Name
	}
	if state, ok := states[link.KeyElementStateID]; ok {
		out.StateLabel = state.Label
	}
	return out
}

func shotElementContext(ctx context.Context, queries *db.Queries, workspaceID pgtype.UUID) ([]db.ShotKeyElement, map[pgtype.UUID]db.Shot, map[pgtype.UUID]db.KeyElementState, map[pgtype.UUID]db.KeyElement, error) {
	links, err := queries.ListShotKeyElementsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	shots, err := queries.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	states, err := queries.ListActiveKeyElementStatesByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	elements, err := queries.ListActiveKeyElementsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	shotsByID := make(map[pgtype.UUID]db.Shot, len(shots))
	for _, shot := range shots {
		shotsByID[shot.ID] = shot
	}
	statesByID := make(map[pgtype.UUID]db.KeyElementState, len(states))
	for _, state := range states {
		statesByID[state.ID] = state
	}
	elementsByID := make(map[pgtype.UUID]db.KeyElement, len(elements))
	for _, element := range elements {
		elementsByID[element.ID] = element
	}
	return links, shotsByID, statesByID, elementsByID, nil
}

func mediaNodeDetail(node db.MediaNode) agentCanvasMediaNodeDetailResponse {
	return agentCanvasMediaNodeDetailResponse{
		ID:               uuidToString(node.ID),
		WorkspaceID:      uuidToString(node.WorkspaceID),
		NodeType:         string(node.NodeType),
		Title:            node.Title,
		Status:           string(node.Status),
		Prompt:           node.Prompt,
		Source:           node.Source,
		OperationType:    node.OperationType,
		ShotID:           uuidString(node.ShotID),
		AssetID:          uuidString(node.AssetID),
		ModelProvider:    textString(node.ModelProvider),
		ModelID:          textString(node.ModelID),
		ModelParams:      jsonValue(node.ModelParams),
		CurrentVersionID: uuidString(node.CurrentVersionID),
		Metadata:         jsonValue(node.Metadata),
		UpdatedAt:        timeString(node.UpdatedAt),
	}
}

func artifactVersionWithAsset(ctx context.Context, queries *db.Queries, signer assetURLSigner, id pgtype.UUID) (artifactVersionResponse, error) {
	version, err := queries.GetArtifactVersionByID(ctx, id)
	if err != nil {
		return artifactVersionResponse{}, err
	}
	return artifactVersionResponseWithAsset(ctx, queries, signer, version)
}

func artifactVersionResponseWithAsset(ctx context.Context, queries *db.Queries, signer assetURLSigner, version db.ArtifactVersion) (artifactVersionResponse, error) {
	if !version.AssetID.Valid {
		return toArtifactVersionResponse(version, nil, ""), nil
	}
	asset, err := queries.GetMediaAssetByID(ctx, version.AssetID)
	if err != nil {
		return toArtifactVersionResponse(version, nil, ""), nil
	}
	accessURL := ""
	if signer != nil {
		accessURL, err = previewAssetAccessURL(ctx, signer, asset)
		if err != nil {
			return artifactVersionResponse{}, err
		}
	}
	return toArtifactVersionResponse(version, &asset, accessURL), nil
}

func assetReadWithAccess(ctx context.Context, queries *db.Queries, signer assetURLSigner, id pgtype.UUID) (assetReadResponse, error) {
	asset, err := queries.GetMediaAssetByID(ctx, id)
	if err != nil {
		return assetReadResponse{}, err
	}
	accessURL := ""
	if signer != nil {
		accessURL, err = previewAssetAccessURL(ctx, signer, asset)
		if err != nil {
			return assetReadResponse{}, err
		}
	}
	return assetReadResponse{
		ID:          uuidToString(asset.ID),
		Type:        string(asset.Type),
		Mime:        asset.Mime,
		StorageURL:  textString(asset.StorageUrl),
		AccessURL:   accessURL,
		TextContent: textString(asset.TextContent),
		SizeBytes:   asset.SizeBytes.Int64,
		Metadata:    jsonObject(asset.Metadata),
	}, nil
}

func agentCanvasFinalReviewSummaries(reviews []db.ReviewRecord, timelinePlanID pgtype.UUID, outputNodeID pgtype.UUID, versionID pgtype.UUID) []agentWorkbenchReviewSummaryResponse {
	out := make([]agentWorkbenchReviewSummaryResponse, 0)
	for _, review := range reviews {
		if review.ReviewTask != "final_video_review" && review.TargetPhase != "final_video" {
			continue
		}
		if !agentCanvasReviewMatchesFinalOutput(review, timelinePlanID, outputNodeID, versionID) {
			continue
		}
		if summary := agentWorkbenchReviewSummary(review); summary != nil {
			out = append(out, *summary)
		}
	}
	return out
}

func agentCanvasReviewMatchesFinalOutput(review db.ReviewRecord, timelinePlanID pgtype.UUID, outputNodeID pgtype.UUID, versionID pgtype.UUID) bool {
	if review.NodeID.Valid {
		return uuidEquals(review.NodeID, outputNodeID)
	}
	if review.ArtifactVersionID.Valid {
		return uuidEquals(review.ArtifactVersionID, versionID)
	}
	if review.TargetObjectID.Valid {
		return uuidEquals(review.TargetObjectID, timelinePlanID) || uuidEquals(review.TargetObjectID, outputNodeID) || uuidEquals(review.TargetObjectID, versionID)
	}
	return true
}

func agentCanvasFinalOutputIssueSummaries(issues []db.ArtifactIssue, timelinePlanID pgtype.UUID, outputNodeID pgtype.UUID, versionID pgtype.UUID) []agentWorkbenchIssueSummaryResponse {
	matched := make([]db.ArtifactIssue, 0)
	for _, issue := range issues {
		if !agentCanvasIssueMatchesFinalOutput(issue, timelinePlanID, outputNodeID, versionID) {
			continue
		}
		matched = append(matched, issue)
	}
	return agentWorkbenchIssueSummaries(matched)
}

func agentCanvasIssueMatchesFinalOutput(issue db.ArtifactIssue, timelinePlanID pgtype.UUID, outputNodeID pgtype.UUID, versionID pgtype.UUID) bool {
	switch issue.TargetObjectType {
	case "final_video", "final_output":
		return uuidEquals(issue.TargetObjectID, timelinePlanID) || uuidEquals(issue.TargetObjectID, outputNodeID) || uuidEquals(issue.TargetObjectID, versionID)
	case "artifact_version":
		return uuidEquals(issue.TargetObjectID, versionID)
	case "media_node":
		return uuidEquals(issue.TargetObjectID, outputNodeID)
	default:
		return false
	}
}

func uuidEquals(left pgtype.UUID, right pgtype.UUID) bool {
	return left.Valid && right.Valid && left.Bytes == right.Bytes
}

func filterIssuesByWorkspace(issues []db.ArtifactIssue, workspaceID pgtype.UUID) []db.ArtifactIssue {
	out := make([]db.ArtifactIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.WorkspaceID == workspaceID {
			out = append(out, issue)
		}
	}
	return out
}

func renderPlanTitle(plan db.RenderPlan) string {
	if plan.RenderPlanKey != "" {
		return plan.RenderPlanKey
	}
	if plan.TargetPhase != "" && plan.Operation != "" {
		return plan.TargetPhase + " / " + plan.Operation
	}
	if plan.TargetPhase != "" {
		return plan.TargetPhase
	}
	return "RenderPlan"
}

func jsonValue(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func floatValue(value pgtype.Float8) any {
	if !value.Valid {
		return nil
	}
	return value.Float64
}
