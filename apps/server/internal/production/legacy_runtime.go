package production

import (
	"context"
)

type LegacyProviderRuntime struct {
	registry *ProviderRegistry
}

func NewLegacyProviderRuntime(registry *ProviderRegistry) LegacyProviderRuntime {
	return LegacyProviderRuntime{registry: registry}
}

func (r LegacyProviderRuntime) Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error) {
	events := make(chan ProductionEvent, 4)
	provider, err := r.registry.Resolve(intent)
	if err != nil {
		return nil, err
	}
	go func() {
		defer close(events)
		events <- ProductionEvent{
			JobID:        job.ID,
			WorkspaceID:  job.WorkspaceID,
			TargetNodeID: job.TargetNodeID,
			Type:         ProductionEventProviderProgress,
			Progress:     10,
			Payload:      map[string]any{"stage": "provider_started"},
		}
		result, err := provider.Run(ctx, intent)
		if err != nil {
			events <- ProductionEvent{
				JobID:        job.ID,
				WorkspaceID:  job.WorkspaceID,
				TargetNodeID: job.TargetNodeID,
				Type:         ProductionEventJobFailed,
				Progress:     100,
				Err:          err,
			}
			return
		}
		events <- ProductionEvent{
			JobID:        job.ID,
			WorkspaceID:  job.WorkspaceID,
			TargetNodeID: job.TargetNodeID,
			Type:         ProductionEventJobSucceeded,
			Progress:     100,
			Output: ProductionOutput{
				RenderedPrompt:  result.RenderedPrompt,
				TextContent:     result.TextContent,
				AssetContent:    result.AssetContent,
				AssetMIME:       result.AssetMIME,
				AssetStorageURL: result.AssetStorageURL,
				AssetSizeBytes:  result.AssetSizeBytes,
				AssetMetadata:   result.AssetMetadata,
				RequestSummary:  result.ProviderRequest,
				ResponseSummary: result.ProviderResponse,
			},
		}
	}()
	return events, nil
}
