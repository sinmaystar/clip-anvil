package production

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type ProductionJob struct {
	ID           pgtype.UUID
	WorkspaceID  pgtype.UUID
	TargetNodeID pgtype.UUID
}

type ProductionOutput struct {
	RenderedPrompt    string
	TextContent       string
	AssetContent      []byte
	AssetSourceURL    string
	AssetMIME         string
	AssetStorageURL   string
	AssetThumbnailURL string
	AssetSizeBytes    int64
	AssetMetadata     map[string]any
	RequestSummary    map[string]any
	ResponseSummary   map[string]any
}

type EinoProductionRuntime interface {
	Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error)
}

func outputToProviderResult(output ProductionOutput) ProviderResult {
	return ProviderResult{
		RenderedPrompt:    output.RenderedPrompt,
		TextContent:       output.TextContent,
		AssetContent:      output.AssetContent,
		AssetSourceURL:    output.AssetSourceURL,
		AssetMIME:         output.AssetMIME,
		AssetStorageURL:   output.AssetStorageURL,
		AssetThumbnailURL: output.AssetThumbnailURL,
		AssetSizeBytes:    output.AssetSizeBytes,
		AssetMetadata:     output.AssetMetadata,
		ProviderRequest:   output.RequestSummary,
		ProviderResponse:  output.ResponseSummary,
	}
}
