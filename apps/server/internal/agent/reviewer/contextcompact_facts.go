package reviewer

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

func reviewerContextCompactionFacts(reviewContext Context) ([]contextcompact.FullSummaryFact, []contextcompact.MediaCard) {
	facts := make([]contextcompact.FullSummaryFact, 0)
	if strings.TrimSpace(reviewContext.Text) != "" {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "reviewer/current_target",
			Kind:    "review_target",
			Source:  "runtime",
			Summary: strings.TrimSpace(reviewContext.Text),
		})
	}
	if reviewContext.Node.ID.Valid {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "media_node/" + reviewerSemanticOrUUID(reviewContext.Node.SemanticKey, reviewContext.Node.ID),
			Kind:    "media_node",
			Source:  "db",
			Summary: strings.TrimSpace(fmt.Sprintf("%s %s %s", reviewContext.Node.Title, reviewContext.Node.NodeType, reviewContext.Node.Status)),
		})
	}
	if reviewContext.Version.ID.Valid {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "artifact_version/" + reviewerSemanticOrUUID(reviewContext.Version.SemanticKey, reviewContext.Version.ID),
			Kind:    "artifact_version",
			Source:  "db",
			Summary: strings.TrimSpace(fmt.Sprintf("%s %s winner=%t", reviewContext.Version.ArtifactKind, reviewContext.Version.Status, reviewContext.Version.Winner)),
		})
	}
	if reviewContext.GenerationJob.ID.Valid {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "generation_job/" + uuidString(reviewContext.GenerationJob.ID),
			Kind:    "generation_job",
			Source:  "db",
			Summary: strings.TrimSpace(fmt.Sprintf("%s %s", reviewContext.GenerationJob.OperationType, reviewContext.GenerationJob.Status)),
		})
	}
	for _, record := range reviewContext.PriorReviews {
		if !record.ID.Valid {
			continue
		}
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "review_record/" + reviewerSemanticOrUUID(record.SemanticKey, record.ID),
			Kind:    "review_record",
			Source:  "db",
			Summary: strings.TrimSpace(fmt.Sprintf("%s %s", record.Status, record.Critique)),
		})
	}
	cards := make([]contextcompact.MediaCard, 0, 1)
	if reviewContext.Version.ID.Valid {
		cards = append(cards, contextcompact.MediaCard{
			Ref:       "artifact_version/" + reviewerSemanticOrUUID(reviewContext.Version.SemanticKey, reviewContext.Version.ID),
			Kind:      reviewerMediaKind(reviewContext.AssetMime, string(reviewContext.Node.NodeType)),
			Role:      "review_target",
			Status:    string(reviewContext.Version.Status),
			Summary:   "未生成视觉摘要",
			SourceRef: "db",
		})
	}
	return facts, cards
}

func reviewerSemanticOrUUID(semanticKey string, id pgtype.UUID) string {
	if strings.TrimSpace(semanticKey) != "" {
		return strings.TrimSpace(semanticKey)
	}
	return uuidString(id)
}

func reviewerMediaKind(mime string, fallback string) string {
	mime = strings.TrimSpace(mime)
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case strings.TrimSpace(fallback) != "":
		return strings.TrimSpace(fallback)
	default:
		return "media"
	}
}
