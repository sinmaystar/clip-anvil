package producer

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/agent/pss"
)

type PSSFactsProvider struct {
	builder *pss.Builder
}

func NewPSSFactsProvider(store pss.Store) PSSFactsProvider {
	return PSSFactsProvider{builder: pss.NewBuilder(store)}
}

func (p PSSFactsProvider) LoadProducerFacts(ctx context.Context, workspaceID pgtype.UUID) ([]contextcompact.FullSummaryFact, []contextcompact.MediaCard, error) {
	if p.builder == nil {
		return nil, nil, nil
	}
	packet, err := p.builder.BuildProducerPSS(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	facts := []contextcompact.FullSummaryFact{{
		Ref:     "project_state/producer_pss",
		Kind:    "project_state",
		Source:  "db",
		Summary: strings.TrimSpace(packet.Text),
	}}
	return facts, nil, nil
}
