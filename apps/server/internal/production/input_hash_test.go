package production

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestComputeInputHashIsStableForMapOrdering(t *testing.T) {
	left, err := ComputeInputHash(InputHashFacts{
		NodeType:        "text",
		OperationType:   "text_generation",
		PromptTemplate:  "write a line",
		PromptRefs:      []byte(`[]`),
		Model:           ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params:          map[string]any{"temperature": 0.2, "seed": "abc"},
		Dependencies:    []InputHashDependency{},
		ReferencePacks:  []InputHashReferencePack{},
		ProviderVersion: ProviderBridgeVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := ComputeInputHash(InputHashFacts{
		NodeType:        "text",
		OperationType:   "text_generation",
		PromptTemplate:  "write a line",
		PromptRefs:      []byte(`[]`),
		Model:           ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params:          map[string]any{"seed": "abc", "temperature": 0.2},
		Dependencies:    []InputHashDependency{},
		ReferencePacks:  []InputHashReferencePack{},
		ProviderVersion: ProviderBridgeVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("hash changed with map ordering: %q != %q", left, right)
	}
}

func TestComputeInputHashChangesWhenDependencyWinnerChanges(t *testing.T) {
	base := InputHashFacts{
		NodeType:       "text",
		OperationType:  "text_generation",
		PromptTemplate: "summarize dependency",
		PromptRefs:     []byte(`[]`),
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params:         map[string]any{},
		Dependencies: []InputHashDependency{
			{NodeID: "node-a", CurrentVersionID: "version-1", InputHash: "hash-1"},
		},
		ReferencePacks:  []InputHashReferencePack{},
		ProviderVersion: ProviderBridgeVersion,
	}
	before, err := ComputeInputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Dependencies[0].CurrentVersionID = "version-2"
	base.Dependencies[0].InputHash = "hash-2"
	after, err := ComputeInputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("hash did not change after dependency winner changed")
	}
}

func TestComputeInputHashChangesWhenReferencePackMembershipChanges(t *testing.T) {
	base := InputHashFacts{
		NodeType:       "text",
		OperationType:  "text_generation",
		PromptTemplate: "use pack",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params:         map[string]any{},
		ReferencePacks: []InputHashReferencePack{{
			PackID: "pack-1",
			Members: []InputHashReferencePackMember{
				{NodeID: "node-a", CurrentVersionID: "version-a", InputHash: "hash-a"},
			},
		}},
	}
	before, err := ComputeInputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	base.ReferencePacks[0].Members = append(base.ReferencePacks[0].Members, InputHashReferencePackMember{
		NodeID: "node-b", CurrentVersionID: "version-b", InputHash: "hash-b",
	})
	after, err := ComputeInputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("hash did not change after reference pack membership changed")
	}
}

func TestComputeInputHashChangesWhenReferencePackMemberWinnerChanges(t *testing.T) {
	base := InputHashFacts{
		NodeType:       "text",
		OperationType:  "text_generation",
		PromptTemplate: "use pack",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params:         map[string]any{},
		ReferencePacks: []InputHashReferencePack{{
			PackID: "pack-1",
			Members: []InputHashReferencePackMember{
				{NodeID: "node-a", CurrentVersionID: "version-a1", InputHash: "hash-a1"},
			},
		}},
	}
	before, err := ComputeInputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	base.ReferencePacks[0].Members[0].CurrentVersionID = "version-a2"
	base.ReferencePacks[0].Members[0].InputHash = "hash-a2"
	after, err := ComputeInputHash(base)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("hash did not change after reference pack member winner changed")
	}
}

func TestComputeInputHashChangesWhenMotionStyleChanges(t *testing.T) {
	intent := motionVariantIntent()
	before := computeMotionHashForIntent(t, intent)
	intent.Params["motion_style"] = "social_fast"
	after := computeMotionHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after motion style changed")
	}
}

func TestComputeInputHashChangesWhenMotionTextLayersChange(t *testing.T) {
	intent := motionVariantIntent()
	before := computeMotionHashForIntent(t, intent)
	intent.Params["text_layers"].([]any)[0].(map[string]any)["text"] = "New headline"
	after := computeMotionHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after motion text layers changed")
	}
}

func TestComputeInputHashStableForMotionTransitionOrdering(t *testing.T) {
	left := motionVariantIntent()
	right := motionVariantIntent()
	left.Params["transitions"] = map[string]any{
		"in":  "soft_zoom",
		"out": "swipe_up",
	}
	right.Params["transitions"] = map[string]any{
		"out": "swipe_up",
		"in":  "soft_zoom",
	}
	if got, want := computeMotionHashForIntent(t, left), computeMotionHashForIntent(t, right); got != want {
		t.Fatalf("hash changed with transition map ordering: %q != %q", got, want)
	}
}

func TestComputeInputHashChangesWhenMotionInputRefRoleChanges(t *testing.T) {
	intent := motionVariantIntent()
	intent.InputRefs = []InputRef{motionInputRef(0x21, "product_image")}
	before := computeMotionHashForIntent(t, intent)
	intent.InputRefs[0].ModelRole = "logo"
	after := computeMotionHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after motion input ref role changed")
	}
}

func TestComputeInputHashChangesWhenMotionInputRefOrderChanges(t *testing.T) {
	intent := motionVariantIntent()
	intent.InputRefs = []InputRef{
		motionInputRef(0x21, "product_image"),
		motionInputRef(0x22, "background_image"),
	}
	before := computeMotionHashForIntent(t, intent)
	intent.InputRefs[0], intent.InputRefs[1] = intent.InputRefs[1], intent.InputRefs[0]
	after := computeMotionHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after motion input ref order changed")
	}
}

func TestComputeInputHashChangesWhenMotionInputRefWinnerChanges(t *testing.T) {
	intent := motionVariantIntent()
	intent.InputRefs = []InputRef{motionInputRef(0x21, "product_image")}
	before := computeMotionHashForIntent(t, intent)
	intent.InputRefs[0].CurrentVersionID = "version-2"
	intent.InputRefs[0].InputHash = "sha256:source-v2"
	after := computeMotionHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after motion input ref winner changed")
	}
}

func motionVariantIntent() GenerationIntent {
	return GenerationIntent{
		OutputType:     "video",
		OperationType:  "image_to_motion_video",
		PromptTemplate: "Create a motion shot",
		Model:          ModelSpec{Provider: "internal_motion_video", ModelID: "remotion-motion-shot-v1"},
		Params: map[string]any{
			"motion_style": "premium_product_ad",
			"duration_sec": float64(5),
			"ratio":        "9:16",
			"fps":          float64(30),
			"text_layers": []any{
				map[string]any{"role": "hook", "text": "Travel lighter", "start_sec": float64(0.2), "end_sec": float64(2.4)},
			},
			"transitions": map[string]any{"in": "soft_zoom", "out": "swipe_up"},
		},
	}
}

func computeMotionHashForIntent(t *testing.T, intent GenerationIntent) string {
	t.Helper()
	hash, err := ComputeInputHash(InputHashFactsForNode(
		db.MediaNode{NodeType: db.NodeTypeVideo, PromptRefs: []byte(`[]`)},
		intent,
		[]InputHashDependency{{NodeID: "source-node", CurrentVersionID: "version-1", InputHash: "sha256:source-v1"}},
		[]InputHashReferencePack{},
	))
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func motionInputRef(id byte, role string) InputRef {
	return InputRef{
		NodeID:           pgtype.UUID{Bytes: [16]byte{id}, Valid: true},
		Kind:             InputKindExplicit,
		Required:         true,
		NodeType:         "image",
		CurrentVersionID: "version-1",
		AssetID:          "asset-1",
		AssetType:        "image",
		Mime:             "image/png",
		ContentType:      "image_url",
		ModelRole:        role,
		InputHash:        "sha256:source-v1",
	}
}
