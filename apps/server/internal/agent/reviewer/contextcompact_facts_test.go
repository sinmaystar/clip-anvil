package reviewer

import (
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestReviewerFullCompactFactsIncludeTargetAndMediaCard(t *testing.T) {
	facts, cards := reviewerContextCompactionFacts(Context{
		Text:          "Review Target\n- shot: shot_01",
		AssetURL:      "data:image/png;base64,iVBORw0KGgo=",
		AssetMime:     "image/png",
		Node:          db.MediaNode{ID: uuidWithByte(5), SemanticKey: "shot_01.preview.node", NodeType: db.NodeTypeImage, Status: db.NodeStatusSucceeded, Title: "shot preview"},
		Version:       db.ArtifactVersion{ID: uuidWithByte(6), SemanticKey: "shot_01.preview.r1", Status: db.JobStatusSucceeded, ArtifactKind: "preview_image"},
		GenerationJob: db.GenerationJob{ID: uuidWithByte(7), OperationType: "text_to_image", Status: db.JobStatusSucceeded},
		PriorReviews: []db.ReviewRecord{
			{ID: uuidWithByte(8), SemanticKey: "review.shot_01.r1", Status: "accepted", Critique: "产品清晰"},
		},
	})

	if len(facts) < 3 {
		t.Fatalf("facts = %#v", facts)
	}
	if len(cards) != 1 || cards[0].Ref != "artifact_version/shot_01.preview.r1" || cards[0].Kind != "image" {
		t.Fatalf("cards = %#v", cards)
	}
}
