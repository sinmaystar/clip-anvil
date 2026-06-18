package production

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type GenerationIntent struct {
	WorkspaceID    pgtype.UUID    `json:"workspace_id"`
	TargetNodeID   pgtype.UUID    `json:"target_node_id"`
	OutputType     string         `json:"output_type"`
	OperationType  string         `json:"operation_type"`
	PromptTemplate string         `json:"prompt_template"`
	InputRefs      []InputRef     `json:"input_refs"`
	Model          ModelSpec      `json:"model"`
	Params         map[string]any `json:"params"`
	RequestedBy    RequestedBy    `json:"requested_by"`
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
	InputHash        string      `json:"input_hash,omitempty"`
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
	RenderedPrompt   string
	TextContent      string
	AssetContent     []byte
	AssetMIME        string
	AssetStorageURL  string
	AssetSizeBytes   int64
	AssetMetadata    map[string]any
	ProviderRequest  map[string]any
	ProviderResponse map[string]any
}

type ProviderBridge interface {
	Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error)
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
