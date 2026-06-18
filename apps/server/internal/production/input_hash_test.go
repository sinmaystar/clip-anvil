package production

import "testing"

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
