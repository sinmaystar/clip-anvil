package production

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestGenerationIntentJSONShape(t *testing.T) {
	workspaceID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	nodeID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	intent := GenerationIntent{
		WorkspaceID:    workspaceID,
		TargetNodeID:   nodeID,
		OutputType:     "text",
		OperationType:  "text_generation",
		PromptTemplate: "write a short ad",
		InputRefs: []InputRef{
			{NodeID: nodeID, Kind: "dependency", Required: false},
		},
		Model: ModelSpec{
			Provider: "mock",
			ModelID:  "mock-text",
		},
		Params: map[string]any{"temperature": 0.2},
		RequestedBy: RequestedBy{
			Type: "user",
			ID:   "account-123",
		},
	}

	raw, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	if got["operation_type"] != "text_generation" {
		t.Fatalf("operation_type = %v", got["operation_type"])
	}
	model := got["model"].(map[string]any)
	if model["provider"] != "mock" || model["model_id"] != "mock-text" {
		t.Fatalf("model = %#v", model)
	}
	requestedBy := got["requested_by"].(map[string]any)
	if requestedBy["type"] != "user" || requestedBy["id"] != "account-123" {
		t.Fatalf("requested_by = %#v", requestedBy)
	}
	if _, ok := got["model_provider"]; ok {
		t.Fatalf("intent must use nested model, got legacy model_provider")
	}
}

func TestProviderRegistrySelectsMockProvider(t *testing.T) {
	registry := NewProviderRegistry(ProviderConfig{
		ProviderMode:     "mock",
		DefaultProvider:  "mock",
		DefaultTextModel: "mock-text",
	})

	intent := GenerationIntent{
		PromptTemplate: "write a short ad",
		OutputType:     "text",
		OperationType:  "text_generation",
		Model:          ModelSpec{},
	}

	resolved := registry.ApplyDefaults(intent)
	if resolved.Model.Provider != "mock" {
		t.Fatalf("provider = %q, want mock", resolved.Model.Provider)
	}
	if resolved.Model.ModelID != "mock-text" {
		t.Fatalf("model = %q, want mock-text", resolved.Model.ModelID)
	}

	provider, err := registry.Resolve(resolved)
	if err != nil {
		t.Fatalf("resolve provider: %v", err)
	}
	result, err := provider.Run(context.Background(), resolved)
	if err != nil {
		t.Fatalf("run mock provider: %v", err)
	}
	if result.RenderedPrompt != "write a short ad" {
		t.Fatalf("rendered prompt = %q", result.RenderedPrompt)
	}
}

func TestProviderRegistryRejectsUnknownProvider(t *testing.T) {
	registry := NewProviderRegistry(ProviderConfig{
		ProviderMode:     "mock",
		DefaultProvider:  "mock",
		DefaultTextModel: "mock-text",
	})

	_, err := registry.Resolve(GenerationIntent{
		Model: ModelSpec{Provider: "unknown", ModelID: "model"},
	})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("error = %v, want ErrProviderUnavailable", err)
	}
}

func TestVolcengineProviderFailsBeforeNetworkWithoutAPIKey(t *testing.T) {
	provider := NewVolcengineProvider(VolcengineProviderConfig{
		BaseURL:   "https://example.invalid",
		TextModel: "doubao-cheap",
	})

	_, err := provider.Run(context.Background(), GenerationIntent{
		OutputType:     "text",
		OperationType:  "text_generation",
		PromptTemplate: "write a short ad",
		Model:          ModelSpec{Provider: "volcengine", ModelID: "doubao-cheap"},
		Params:         map[string]any{},
	})
	if !errors.Is(err, ErrProviderConfig) {
		t.Fatalf("error = %v, want ErrProviderConfig", err)
	}
}

func TestMockProviderReturnsDeterministicText(t *testing.T) {
	provider := MockProvider{}
	intent := GenerationIntent{
		PromptTemplate: "write a short ad",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params:         map[string]any{},
	}

	result, err := provider.Run(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}

	if result.RenderedPrompt != "write a short ad" {
		t.Fatalf("rendered prompt = %q, want write a short ad", result.RenderedPrompt)
	}
	if result.TextContent != "[mock:mock-text] write a short ad" {
		t.Fatalf("text content = %q, want mock text", result.TextContent)
	}
}

func TestErrorCodeForProviderConfig(t *testing.T) {
	err := fmt.Errorf("%w: missing key", ErrProviderConfig)
	if code := errorCodeForRun(err); code != "provider_config_error" {
		t.Fatalf("code = %q, want provider_config_error", code)
	}
}

func TestIntentForNodeUsesProductionFields(t *testing.T) {
	node := db.MediaNode{
		NodeType:       db.NodeTypeText,
		OperationType:  "text_generation",
		PromptTemplate: "write a crisp line",
		ModelProvider:  pgtype.Text{String: "mock", Valid: true},
		ModelID:        pgtype.Text{String: "mock-text", Valid: true},
		ModelParams:    []byte(`{"temperature":0.2}`),
	}

	intent := intentForNode(node, RequestedBy{Type: "user", ID: "account-123"})
	if intent.Model.Provider != "mock" {
		t.Fatalf("provider = %q", intent.Model.Provider)
	}
	if intent.Model.ModelID != "mock-text" {
		t.Fatalf("model = %q", intent.Model.ModelID)
	}
	if intent.Params["temperature"] != 0.2 {
		t.Fatalf("params = %#v", intent.Params)
	}
	if intent.RequestedBy.ID != "account-123" {
		t.Fatalf("requested by = %#v", intent.RequestedBy)
	}
}

func TestCapabilityValidatorAcceptsSupportedIntent(t *testing.T) {
	capability := Capability{
		ProviderID:              "mock",
		ModelID:                 "mock-text",
		OutputTypes:             []string{"text"},
		SupportedOperations:     []string{"text_generation"},
		SupportedInputNodeTypes: []string{"text"},
		Limits:                  CapabilityLimits{MaxPromptChars: 100, MaxAttempts: 3},
	}
	intent := GenerationIntent{
		OutputType:     "text",
		OperationType:  "text_generation",
		PromptTemplate: "write a short ad",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params:         map[string]any{},
	}

	if err := ValidateCapability(intent, capability); err != nil {
		t.Fatalf("ValidateCapability() error = %v", err)
	}
}

func TestCapabilityValidatorRejectsOutputMismatch(t *testing.T) {
	capability := Capability{
		ProviderID:          "mock",
		ModelID:             "mock-image-only",
		OutputTypes:         []string{"image"},
		SupportedOperations: []string{"text_to_image"},
		Limits:              CapabilityLimits{MaxAttempts: 3},
	}
	intent := GenerationIntent{
		OutputType:     "video",
		OperationType:  "text_to_video",
		PromptTemplate: "make a video",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-image-only"},
		Params:         map[string]any{},
	}

	err := ValidateCapability(intent, capability)
	if !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("error = %v, want ErrCapabilityMismatch", err)
	}
	if code := errorCodeForRun(err); code != "capability_mismatch" {
		t.Fatalf("code = %q, want capability_mismatch", code)
	}
}

func TestCapabilityValidatorRejectsLimitMismatch(t *testing.T) {
	capability := Capability{
		ProviderID:          "mock",
		ModelID:             "mock-video",
		OutputTypes:         []string{"video"},
		SupportedOperations: []string{"text_to_video"},
		Limits: CapabilityLimits{
			MaxPromptChars:   100,
			MaxAttempts:      3,
			AllowedDurations: []int{4, 5, 8},
		},
	}
	intent := GenerationIntent{
		OutputType:     "video",
		OperationType:  "text_to_video",
		PromptTemplate: "make a video",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-video"},
		Params:         map[string]any{"duration_sec": float64(15)},
	}

	err := ValidateCapability(intent, capability)
	if !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("error = %v, want ErrCapabilityMismatch", err)
	}
}

func TestMaxAttemptsRespectsCapabilityLimit(t *testing.T) {
	options := RunOptions{MaxAttempts: 10}
	capability := Capability{Limits: CapabilityLimits{MaxAttempts: 3}}
	if got := maxAttemptsForRun(options, capability); got != 3 {
		t.Fatalf("max attempts = %d, want 3", got)
	}
}

func TestChangedInputStaleReasonDetails(t *testing.T) {
	upstreamNodeID := pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true}
	upstreamVersionID := pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true}
	targetVersionID := pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true}

	details, err := changedInputStaleReasonDetails(upstreamNodeID, upstreamVersionID, targetVersionID, "sha256:old", "sha256:new")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(details, &got); err != nil {
		t.Fatal(err)
	}
	if got["upstream_node_id"] != uuidToString(upstreamNodeID) {
		t.Fatalf("upstream_node_id = %v", got["upstream_node_id"])
	}
	if got["target_version_id"] != uuidToString(targetVersionID) {
		t.Fatalf("target_version_id = %v", got["target_version_id"])
	}
	if got["previous_input_hash"] != "sha256:old" || got["current_input_hash"] != "sha256:new" {
		t.Fatalf("hash details = %#v", got)
	}
	if got["reason"] != "upstream_current_version_changed" {
		t.Fatalf("reason = %v", got["reason"])
	}
}

func TestChangedInputStaleReasonDetailsSupportsReferencePackReason(t *testing.T) {
	sourceNodeID := pgtype.UUID{Bytes: [16]byte{0x07}, Valid: true}
	sourceVersionID := pgtype.UUID{Bytes: [16]byte{0x08}, Valid: true}
	targetVersionID := pgtype.UUID{Bytes: [16]byte{0x09}, Valid: true}

	details, err := changedInputStaleReasonDetailsWithReason(
		sourceNodeID,
		sourceVersionID,
		targetVersionID,
		"sha256:old",
		"sha256:new",
		"reference_pack_member_version_changed",
	)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(details, &got); err != nil {
		t.Fatal(err)
	}
	if got["reason"] != "reference_pack_member_version_changed" {
		t.Fatalf("reason = %v", got["reason"])
	}
}

func TestMaxAttemptsDefaultsToOne(t *testing.T) {
	options := RunOptions{}
	capability := Capability{Limits: CapabilityLimits{MaxAttempts: 3}}
	if got := maxAttemptsForRun(options, capability); got != 1 {
		t.Fatalf("max attempts = %d, want 1", got)
	}
}

func TestMockProviderCanFailDeterministically(t *testing.T) {
	provider := MockProvider{}
	_, err := provider.Run(context.Background(), GenerationIntent{
		OperationType:  "text_generation",
		PromptTemplate: "fail this",
		Model:          ModelSpec{Provider: "mock", ModelID: "mock-text"},
		Params: map[string]any{
			"mock_fail": true,
		},
	})
	if !errors.Is(err, ErrProviderExecution) {
		t.Fatalf("error = %v, want ErrProviderExecution", err)
	}
	if code := errorCodeForRun(err); code != "provider_error" {
		t.Fatalf("code = %q, want provider_error", code)
	}
}
