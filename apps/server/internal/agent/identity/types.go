package identity

import "github.com/jackc/pgx/v5/pgtype"

const (
	ObjectCreativeBrief         = "creative_brief"
	ObjectProjectMemory         = "project_memory"
	ObjectKeyElement            = "key_element"
	ObjectKeyElementState       = "key_element_state"
	ObjectScene                 = "scene"
	ObjectShot                  = "shot"
	ObjectShotDependency        = "shot_dependency"
	ObjectRenderPlan            = "render_plan"
	ObjectMediaNode             = "media_node"
	ObjectGenerationJob         = "generation_job"
	ObjectArtifactVersion       = "artifact_version"
	ObjectReviewRecord          = "review_record"
	ObjectArtifactIssue         = "artifact_issue"
	ObjectAgentThread           = "agent_thread"
	ObjectAgentTask             = "agent_task"
	ObjectProducerPendingSignal = "producer_pending_signal"
)

type ObjectRef struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

type ArtifactSelectorRef struct {
	Key          string    `json:"key"`
	Scope        ObjectRef `json:"scope"`
	ArtifactKind string    `json:"artifact_kind"`
	Selector     string    `json:"selector"`
}

type ResolvedObject struct {
	WorkspaceID       pgtype.UUID
	ObjectType        string
	ObjectID          pgtype.UUID
	SemanticKey       string
	DisplayName       string
	ParentObjectType  string
	ParentObjectID    pgtype.UUID
	ParentSemanticKey string
}
