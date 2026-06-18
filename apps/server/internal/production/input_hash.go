package production

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const (
	InputHashSchemaVersion = "m4.input_hash.v1"
	ProviderBridgeVersion  = "provider_bridge.v1"
)

type InputHashFacts struct {
	SchemaVersion   string                   `json:"schema_version"`
	ProviderVersion string                   `json:"provider_version"`
	NodeType        string                   `json:"node_type"`
	OperationType   string                   `json:"operation_type"`
	PromptTemplate  string                   `json:"prompt_template"`
	PromptRefs      json.RawMessage          `json:"prompt_refs"`
	Model           ModelSpec                `json:"model"`
	Params          map[string]any           `json:"params"`
	Dependencies    []InputHashDependency    `json:"dependencies"`
	ReferencePacks  []InputHashReferencePack `json:"reference_packs"`
}

type InputHashDependency struct {
	NodeID           string `json:"node_id"`
	CurrentVersionID string `json:"current_version_id"`
	InputHash        string `json:"input_hash"`
}

type InputHashReferencePack struct {
	PackID  string                         `json:"pack_id"`
	Members []InputHashReferencePackMember `json:"members"`
}

type InputHashReferencePackMember struct {
	NodeID           string `json:"node_id"`
	CurrentVersionID string `json:"current_version_id"`
	InputHash        string `json:"input_hash"`
}

func ComputeInputHash(facts InputHashFacts) (string, error) {
	if facts.SchemaVersion == "" {
		facts.SchemaVersion = InputHashSchemaVersion
	}
	if facts.ProviderVersion == "" {
		facts.ProviderVersion = ProviderBridgeVersion
	}
	if len(facts.PromptRefs) == 0 {
		facts.PromptRefs = []byte(`[]`)
	}
	if facts.Params == nil {
		facts.Params = map[string]any{}
	}
	if facts.Dependencies == nil {
		facts.Dependencies = []InputHashDependency{}
	}
	if facts.ReferencePacks == nil {
		facts.ReferencePacks = []InputHashReferencePack{}
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func InputHashFactsForNode(
	node db.MediaNode,
	intent GenerationIntent,
	dependencies []InputHashDependency,
	referencePacks []InputHashReferencePack,
) InputHashFacts {
	return InputHashFacts{
		NodeType:        string(node.NodeType),
		OperationType:   intent.OperationType,
		PromptTemplate:  intent.PromptTemplate,
		PromptRefs:      node.PromptRefs,
		Model:           intent.Model,
		Params:          intent.Params,
		Dependencies:    dependencies,
		ReferencePacks:  referencePacks,
		ProviderVersion: ProviderBridgeVersion,
	}
}
