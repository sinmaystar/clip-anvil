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

func TestComputeInputHashChangesWhenTemplateKeyChanges(t *testing.T) {
	intent := templateVariantIntent()
	before := computeTemplateHashForIntent(t, intent)
	intent.Params["template_key"] = "benefit_cards_v1"
	after := computeTemplateHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after template key changed")
	}
}

func TestComputeInputHashChangesWhenTemplateVariablesChange(t *testing.T) {
	intent := templateVariantIntent()
	before := computeTemplateHashForIntent(t, intent)
	intent.Params["variables"].(map[string]any)["headline"] = "New headline"
	after := computeTemplateHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after template variables changed")
	}
}

func TestComputeInputHashStableForTemplateVariableOrdering(t *testing.T) {
	left := templateVariantIntent()
	right := templateVariantIntent()
	left.Params["variables"] = map[string]any{
		"headline":     "Travel lighter",
		"cta":          "Shop now",
		"brand_colors": []any{"#111111", "#F7D046"},
	}
	right.Params["variables"] = map[string]any{
		"brand_colors": []any{"#111111", "#F7D046"},
		"cta":          "Shop now",
		"headline":     "Travel lighter",
	}
	if got, want := computeTemplateHashForIntent(t, left), computeTemplateHashForIntent(t, right); got != want {
		t.Fatalf("hash changed with variable map ordering: %q != %q", got, want)
	}
}

func TestComputeInputHashChangesWhenTemplateInputRefRoleChanges(t *testing.T) {
	intent := templateVariantIntent()
	intent.InputRefs = []InputRef{templateInputRef(0x21, "product_image")}
	before := computeTemplateHashForIntent(t, intent)
	intent.InputRefs[0].ModelRole = "logo"
	after := computeTemplateHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after template input ref role changed")
	}
}

func TestComputeInputHashChangesWhenTemplateInputRefOrderChanges(t *testing.T) {
	intent := templateVariantIntent()
	intent.InputRefs = []InputRef{
		templateInputRef(0x21, "product_image"),
		templateInputRef(0x22, "background_image"),
	}
	before := computeTemplateHashForIntent(t, intent)
	intent.InputRefs[0], intent.InputRefs[1] = intent.InputRefs[1], intent.InputRefs[0]
	after := computeTemplateHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after template input ref order changed")
	}
}

func TestComputeInputHashChangesWhenTemplateInputRefWinnerChanges(t *testing.T) {
	intent := templateVariantIntent()
	intent.InputRefs = []InputRef{templateInputRef(0x21, "product_image")}
	before := computeTemplateHashForIntent(t, intent)
	intent.InputRefs[0].CurrentVersionID = "version-2"
	intent.InputRefs[0].InputHash = "sha256:source-v2"
	after := computeTemplateHashForIntent(t, intent)
	if before == after {
		t.Fatal("hash did not change after template input ref winner changed")
	}
}

func templateVariantIntent() GenerationIntent {
	return GenerationIntent{
		OutputType:     "video",
		OperationType:  "image_to_template_video",
		PromptTemplate: "Create a template fallback shot",
		Model:          ModelSpec{Provider: "internal_template_video", ModelID: "hyperframes-html"},
		Params: map[string]any{
			"template_key": "static_fallback_ken_burns_v1",
			"duration_sec": float64(5),
			"ratio":        "9:16",
			"fps":          float64(24),
			"variables": map[string]any{
				"headline":     "Travel lighter",
				"cta":          "Shop now",
				"brand_colors": []any{"#111111", "#F7D046"},
			},
		},
	}
}

func computeTemplateHashForIntent(t *testing.T, intent GenerationIntent) string {
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

func templateInputRef(id byte, role string) InputRef {
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
