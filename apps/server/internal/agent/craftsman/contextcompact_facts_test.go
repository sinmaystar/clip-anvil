package craftsman

import (
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestCraftsmanFullCompactFactsIncludeCurrentScopeAndRenderPlans(t *testing.T) {
	facts, cards := craftsmanContextCompactionFacts(Context{
		Text: "Current Task\n- target_phase: preview_image\n- shot: shot_01",
		Shot: db.Shot{ID: uuidWithByte(2), ClientKey: "shot_01", Title: "开场", Status: "planned"},
		RenderPlans: []db.RenderPlan{
			{ID: uuidWithByte(3), SemanticKey: "shot_01.preview.r1", TargetPhase: "preview_image", Status: "submitted"},
		},
		SourceMaterials: []NodeState{
			{Node: db.MediaNode{ID: uuidWithByte(4), SemanticKey: "product.box", NodeType: db.NodeTypeImage, Status: db.NodeStatusSucceeded, Title: "box.png"}},
		},
	})

	if len(facts) < 3 {
		t.Fatalf("facts = %#v", facts)
	}
	if len(cards) != 1 || cards[0].Ref != "media_node/product.box" || cards[0].Summary != "未生成视觉摘要" {
		t.Fatalf("cards = %#v", cards)
	}
}
