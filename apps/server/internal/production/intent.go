package production

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	InputKindExplicit            = "explicit"
	InputKindImplicit            = "implicit"
	InputKindReferencePack       = "reference_pack"
	InputKindReferencePackMember = "reference_pack_member"
)

type GenerationIntent struct {
	WorkspaceID    pgtype.UUID    `json:"workspace_id"`
	TargetNodeID   pgtype.UUID    `json:"target_node_id"`
	OutputType     string         `json:"output_type"`
	OperationType  string         `json:"operation_type"`
	PromptTemplate string         `json:"prompt_template"`
	RenderedPrompt string         `json:"rendered_prompt,omitempty"`
	InputRefs      []InputRef     `json:"input_refs"`
	Model          ModelSpec      `json:"model"`
	Params         map[string]any `json:"params"`
	RequestedBy    RequestedBy    `json:"requested_by"`
	Semantic       SemanticInfo   `json:"semantic,omitempty"`
}

type SemanticInfo struct {
	ScopeKey           string      `json:"scope_key,omitempty"`
	RenderPlanKey      string      `json:"render_plan_key,omitempty"`
	ArtifactKind       string      `json:"artifact_kind,omitempty"`
	SourceRenderPlanID pgtype.UUID `json:"source_render_plan_id,omitempty"`
}

type InputRef struct {
	NodeID           pgtype.UUID `json:"node_id"`
	Kind             string      `json:"kind"`
	Required         bool        `json:"required"`
	NodeType         string      `json:"node_type,omitempty"`
	CurrentVersionID string      `json:"current_version_id,omitempty"`
	AssetID          string      `json:"asset_id,omitempty"`
	AssetType        string      `json:"asset_type,omitempty"`
	Mime             string      `json:"mime,omitempty"`
	StorageURL       string      `json:"storage_url,omitempty"`
	ContentType      string      `json:"content_type,omitempty"`
	ModelRole        string      `json:"model_role,omitempty"`
	InputHash        string      `json:"input_hash,omitempty"`
	TextContent      string      `json:"-"`
}

type ModelSpec struct {
	Provider string `json:"provider"`
	ModelID  string `json:"model_id"`
}

type RequestedBy struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

type ProviderResult struct {
	RenderedPrompt    string
	TextContent       string
	AssetContent      []byte
	AssetSourceURL    string
	AssetMIME         string
	AssetStorageURL   string
	AssetThumbnailURL string
	AssetSizeBytes    int64
	AssetMetadata     map[string]any
	ProviderRequest   map[string]any
	ProviderResponse  map[string]any
}

type ProviderBridge interface {
	Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error)
}

func (intent GenerationIntent) EffectivePrompt() string {
	if intent.RenderedPrompt != "" {
		return intent.RenderedPrompt
	}
	return intent.PromptTemplate
}

type ProviderRunError struct {
	Err      error
	Response map[string]any
}

func (e ProviderRunError) Error() string {
	if e.Err == nil {
		return "provider execution error"
	}
	return e.Err.Error()
}

func (e ProviderRunError) Unwrap() error {
	return e.Err
}
