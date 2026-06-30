package producer

import (
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

func TestProducerFullCompactFactsIncludeProjectStateAndImages(t *testing.T) {
	ctx := ProducerContext{
		LatestUserText: "做一条悦行行李箱广告",
		ImageAttachments: map[string]ProducerImageAttachment{
			"asset-1": {AssetID: "asset-1", NodeID: "node-1", Name: "box.png", Mime: "image/png"},
		},
		ProjectFacts: []contextcompact.FullSummaryFact{
			{Ref: "project_memory/active", Kind: "project_memory", Source: "db", Summary: "核心意图：轻松出行"},
		},
	}

	facts, cards := producerContextCompactionFacts(ctx)
	if len(facts) == 0 || facts[0].Ref != "project_memory/active" {
		t.Fatalf("facts = %#v", facts)
	}
	if len(cards) != 1 || cards[0].Ref != "media_asset/asset-1" || cards[0].Summary != "未生成视觉摘要" {
		t.Fatalf("cards = %#v", cards)
	}
}
