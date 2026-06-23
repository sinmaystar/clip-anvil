package modelselection

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestSelectionDefaultsToConfiguredProducerModel(t *testing.T) {
	service := NewService(fakeCapabilityStore{
		capabilities: []db.ModelCapability{
			textCapability("volcengine", "doubao-mini", "Doubao Mini", true),
		},
	}, Defaults{ProducerProviderID: "volcengine", ProducerModelID: "doubao-mini"})

	result, err := service.Resolve(context.Background(), db.Workspace{Settings: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Selection.Producer.ModelID != "doubao-mini" {
		t.Fatalf("model = %q, want doubao-mini", result.Selection.Producer.ModelID)
	}
	if len(result.Options) != 1 || result.Options[0].DisplayName != "Doubao Mini" {
		t.Fatalf("options = %#v", result.Options)
	}
}

func TestSelectionDefaultsProducerReasoningEffortFromCapability(t *testing.T) {
	service := NewService(fakeCapabilityStore{
		capabilities: []db.ModelCapability{
			thinkingTextCapability("volcengine", "doubao-seed", "Doubao Seed", true),
		},
	}, Defaults{ProducerProviderID: "volcengine", ProducerModelID: "doubao-seed"})

	result, err := service.Resolve(context.Background(), db.Workspace{Settings: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Selection.Producer.ReasoningEffort != "medium" {
		t.Fatalf("reasoning_effort = %q, want medium", result.Selection.Producer.ReasoningEffort)
	}
	if len(result.Options) != 1 {
		t.Fatalf("options len = %d, want 1", len(result.Options))
	}
	option := result.Options[0]
	if !option.SupportsThinking {
		t.Fatalf("SupportsThinking = false, want true")
	}
	if option.DefaultReasoningEffort != "medium" {
		t.Fatalf("DefaultReasoningEffort = %q, want medium", option.DefaultReasoningEffort)
	}
	wantEfforts := []string{"minimal", "low", "medium", "high"}
	if len(option.ReasoningEfforts) != len(wantEfforts) {
		t.Fatalf("ReasoningEfforts = %#v, want %#v", option.ReasoningEfforts, wantEfforts)
	}
	for i, want := range wantEfforts {
		if option.ReasoningEfforts[i] != want {
			t.Fatalf("ReasoningEfforts[%d] = %q, want %q", i, option.ReasoningEfforts[i], want)
		}
	}
}

func TestOptionsExcludeProducerRuntimeUnsupportedProviders(t *testing.T) {
	service := NewService(fakeCapabilityStore{
		capabilities: []db.ModelCapability{
			textCapability("mock", "mock-text", "Mock Text", true),
			textCapability("volcengine", "doubao-mini", "Doubao Mini", true),
		},
	}, Defaults{ProducerProviderID: "volcengine", ProducerModelID: "doubao-mini"})

	options, err := service.Options(context.Background())
	if err != nil {
		t.Fatalf("Options() error = %v", err)
	}
	if len(options) != 1 {
		t.Fatalf("options len = %d, want 1: %#v", len(options), options)
	}
	if options[0].ProviderID != "volcengine" {
		t.Fatalf("provider = %q, want volcengine", options[0].ProviderID)
	}
	_, err = service.ValidateProducerModel(context.Background(), ModelRef{
		ProviderID: "mock",
		ModelID:    "mock-text",
	})
	if !errors.Is(err, ErrUnsupportedProducerModel) {
		t.Fatalf("ValidateProducerModel() error = %v, want ErrUnsupportedProducerModel", err)
	}
}

func TestResolveFallsBackWhenStoredProducerModelIsUnsupported(t *testing.T) {
	service := NewService(fakeCapabilityStore{
		capabilities: []db.ModelCapability{
			textCapability("mock", "mock-text", "Mock Text", true),
			textCapability("volcengine", "doubao-mini", "Doubao Mini", true),
		},
	}, Defaults{ProducerProviderID: "volcengine", ProducerModelID: "doubao-mini"})
	workspace := db.Workspace{Settings: []byte(`{
		"agent": {
			"model_selection": {
				"producer": {"provider_id": "mock", "model_id": "mock-text"}
			}
		}
	}`)}

	result, err := service.Resolve(context.Background(), workspace)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Selection.Producer.ProviderID != "volcengine" ||
		result.Selection.Producer.ModelID != "doubao-mini" {
		t.Fatalf("selection = %#v, want volcengine/doubao-mini", result.Selection.Producer)
	}
}

func TestSelectionRejectsImageOnlyModel(t *testing.T) {
	service := NewService(fakeCapabilityStore{
		capabilities: []db.ModelCapability{
			imageCapability("volcengine", "seedream", "Seedream", true),
		},
	}, Defaults{ProducerProviderID: "volcengine", ProducerModelID: "seedream"})

	_, err := service.ValidateProducerModel(context.Background(), ModelRef{
		ProviderID: "volcengine",
		ModelID:    "seedream",
	})
	if !errors.Is(err, ErrUnsupportedProducerModel) {
		t.Fatalf("error = %v, want ErrUnsupportedProducerModel", err)
	}
}

func TestSelectionRejectsDisabledModel(t *testing.T) {
	service := NewService(fakeCapabilityStore{
		capabilities: []db.ModelCapability{
			textCapability("volcengine", "disabled-text", "Disabled", false),
		},
	}, Defaults{ProducerProviderID: "volcengine", ProducerModelID: "disabled-text"})

	_, err := service.ValidateProducerModel(context.Background(), ModelRef{
		ProviderID: "volcengine",
		ModelID:    "disabled-text",
	})
	if !errors.Is(err, ErrUnsupportedProducerModel) {
		t.Fatalf("error = %v, want ErrUnsupportedProducerModel", err)
	}
}

func TestSelectionRejectsUnsupportedReasoningEffort(t *testing.T) {
	service := NewService(fakeCapabilityStore{
		capabilities: []db.ModelCapability{
			limitedThinkingCapability("volcengine", "doubao-seed", "Doubao Seed", true),
		},
	}, Defaults{ProducerProviderID: "volcengine", ProducerModelID: "doubao-seed"})

	_, err := service.ValidateProducerModel(context.Background(), ModelRef{
		ProviderID:      "volcengine",
		ModelID:         "doubao-seed",
		ReasoningEffort: "high",
	})
	if !errors.Is(err, ErrUnsupportedReasoningEffort) {
		t.Fatalf("error = %v, want ErrUnsupportedReasoningEffort", err)
	}
}

func TestApplyToWorkspaceSettingsPreservesOtherSettings(t *testing.T) {
	raw, err := ApplyToWorkspaceSettings([]byte(`{"theme":"dark","agent":{"other":true}}`), Selection{
		Producer: ModelRef{ProviderID: "volcengine", ModelID: "doubao-mini", ReasoningEffort: "high"},
	})
	if err != nil {
		t.Fatalf("ApplyToWorkspaceSettings() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if got["theme"] != "dark" {
		t.Fatalf("theme = %#v", got["theme"])
	}
	agent := got["agent"].(map[string]any)
	if agent["other"] != true {
		t.Fatalf("agent.other = %#v", agent["other"])
	}
	selection := agent["model_selection"].(map[string]any)
	producer := selection["producer"].(map[string]any)
	if producer["model_id"] != "doubao-mini" {
		t.Fatalf("producer = %#v", producer)
	}
	if producer["reasoning_effort"] != "high" {
		t.Fatalf("producer.reasoning_effort = %#v, want high", producer["reasoning_effort"])
	}
}

type fakeCapabilityStore struct {
	capabilities []db.ModelCapability
}

func (f fakeCapabilityStore) ListEnabledModelCapabilities(context.Context) ([]db.ModelCapability, error) {
	out := []db.ModelCapability{}
	for _, capability := range f.capabilities {
		if capability.Enabled {
			out = append(out, capability)
		}
	}
	return out, nil
}

func textCapability(providerID, modelID, displayName string, enabled bool) db.ModelCapability {
	return db.ModelCapability{
		ProviderID:          providerID,
		ModelID:             modelID,
		DisplayName:         displayName,
		OutputTypes:         []byte(`["text"]`),
		SupportedOperations: []byte(`["text_generation"]`),
		Limits:              []byte(`{"max_prompt_chars":16000}`),
		Pricing:             []byte(`{"tier":"test"}`),
		Enabled:             enabled,
	}
}

func thinkingTextCapability(providerID, modelID, displayName string, enabled bool) db.ModelCapability {
	capability := textCapability(providerID, modelID, displayName, enabled)
	capability.Limits = []byte(`{"max_prompt_chars":32000,"reasoning_efforts":["minimal","low","medium","high"]}`)
	capability.Defaults = []byte(`{"reasoning_effort":"medium","max_completion_tokens":4096}`)
	return capability
}

func limitedThinkingCapability(providerID, modelID, displayName string, enabled bool) db.ModelCapability {
	capability := textCapability(providerID, modelID, displayName, enabled)
	capability.Limits = []byte(`{"reasoning_efforts":["minimal","low"]}`)
	capability.Defaults = []byte(`{"reasoning_effort":"low"}`)
	return capability
}

func imageCapability(providerID, modelID, displayName string, enabled bool) db.ModelCapability {
	return db.ModelCapability{
		ProviderID:          providerID,
		ModelID:             modelID,
		DisplayName:         displayName,
		OutputTypes:         []byte(`["image"]`),
		SupportedOperations: []byte(`["text_to_image"]`),
		Limits:              []byte(`{}`),
		Pricing:             []byte(`{}`),
		Enabled:             enabled,
	}
}
