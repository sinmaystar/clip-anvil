package craftsman

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func craftsmanContextCompactionFacts(craftsmanContext Context) ([]contextcompact.FullSummaryFact, []contextcompact.MediaCard) {
	facts := make([]contextcompact.FullSummaryFact, 0)
	if strings.TrimSpace(craftsmanContext.Text) != "" {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "craftsman/current_task",
			Kind:    "current_task",
			Source:  "runtime",
			Summary: strings.TrimSpace(craftsmanContext.Text),
		})
	}
	if craftsmanContext.Shot.ID.Valid {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "shot/" + semanticOrUUID(craftsmanContext.Shot.SemanticKey, craftsmanContext.Shot.ID),
			Kind:    "shot",
			Source:  "db",
			Summary: strings.TrimSpace(strings.Join([]string{craftsmanContext.Shot.ClientKey, craftsmanContext.Shot.Title, craftsmanContext.Shot.Status}, " ")),
		})
	}
	for _, plan := range craftsmanContext.RenderPlans {
		if !plan.ID.Valid {
			continue
		}
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "render_plan/" + semanticOrUUID(plan.SemanticKey, plan.ID),
			Kind:    "render_plan",
			Source:  "db",
			Summary: strings.TrimSpace(fmt.Sprintf("%s %s %s", plan.TargetPhase, plan.Status, plan.RenderPlanKey)),
		})
	}
	cards := make([]contextcompact.MediaCard, 0, len(craftsmanContext.SourceMaterials))
	for _, material := range craftsmanContext.SourceMaterials {
		if !material.Node.ID.Valid {
			continue
		}
		cards = append(cards, mediaCardForCraftsmanNode(material.Node))
	}
	return facts, cards
}

func mediaCardForCraftsmanNode(node db.MediaNode) contextcompact.MediaCard {
	return contextcompact.MediaCard{
		Ref:       "media_node/" + semanticOrUUID(node.SemanticKey, node.ID),
		Kind:      string(node.NodeType),
		Role:      "source_material",
		Status:    string(node.Status),
		Summary:   "未生成视觉摘要",
		SourceRef: "db",
	}
}

func semanticOrUUID(semanticKey string, id pgtype.UUID) string {
	if strings.TrimSpace(semanticKey) != "" {
		return strings.TrimSpace(semanticKey)
	}
	return uuidString(id)
}
