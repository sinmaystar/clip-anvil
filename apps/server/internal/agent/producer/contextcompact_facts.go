package producer

import (
	"sort"
	"strings"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

func producerContextCompactionFacts(producerContext ProducerContext) ([]contextcompact.FullSummaryFact, []contextcompact.MediaCard) {
	facts := append([]contextcompact.FullSummaryFact(nil), producerContext.ProjectFacts...)
	if text := strings.TrimSpace(producerContext.LatestUserText); text != "" {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "producer/latest_user_text",
			Kind:    "user_instruction",
			Source:  "agent_message",
			Summary: text,
		})
	}
	cards := append([]contextcompact.MediaCard(nil), producerContext.ProjectMediaCards...)
	keys := make([]string, 0, len(producerContext.ImageAttachments))
	for key := range producerContext.ImageAttachments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		attachment := producerContext.ImageAttachments[key]
		ref := "media_asset/" + strings.TrimSpace(attachment.AssetID)
		if strings.TrimSpace(attachment.AssetID) == "" {
			ref = "media_asset/" + strings.TrimSpace(key)
		}
		cards = append(cards, contextcompact.MediaCard{
			Ref:       ref,
			Kind:      "image",
			Role:      "reference",
			Status:    "available",
			Summary:   "未生成视觉摘要",
			SourceRef: "user",
		})
	}
	return facts, cards
}
