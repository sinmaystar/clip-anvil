package production

import (
	"context"
	"fmt"
	"strings"
)

type VolcengineProvider struct {
	cfg VolcengineProviderConfig
}

func NewVolcengineProvider(cfg VolcengineProviderConfig) VolcengineProvider {
	return VolcengineProvider{cfg: cfg}
}

func (p VolcengineProvider) Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error) {
	select {
	case <-ctx.Done():
		return ProviderResult{}, ctx.Err()
	default:
	}

	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return ProviderResult{}, fmt.Errorf("%w: CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for provider volcengine", ErrProviderConfig)
	}

	return ProviderResult{}, fmt.Errorf("%w: volcengine %s adapter is not implemented in M4.2", ErrProviderUnavailable, intent.OutputType)
}
