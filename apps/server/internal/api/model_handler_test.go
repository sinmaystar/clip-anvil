package api

import (
	"encoding/json"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestToModelCapabilityResponseParsesJSONFields(t *testing.T) {
	row := db.ModelCapability{
		ProviderID:              "mock",
		ModelID:                 "mock-image",
		DisplayName:             "Mock Image",
		OutputTypes:             []byte(`["image"]`),
		SupportedOperations:     []byte(`["text_to_image","image_to_image"]`),
		SupportedInputNodeTypes: []byte(`["text","image"]`),
		Limits:                  []byte(`{"max_prompt_chars":8000,"max_attempts":3}`),
		Pricing:                 []byte(`{"tier":"mock"}`),
		Defaults:                []byte(`{"steps":20}`),
		Enabled:                 true,
	}
	got := toModelCapabilityResponse(row)
	if got.ProviderID != "mock" || got.ModelID != "mock-image" {
		t.Fatalf("ids = %s/%s", got.ProviderID, got.ModelID)
	}
	if len(got.SupportedInputNodeTypes) != 2 || got.SupportedInputNodeTypes[1] != "image" {
		raw, _ := json.Marshal(got)
		t.Fatalf("bad response: %s", raw)
	}
	if len(got.SupportedOperations) != 2 || got.SupportedOperations[0] != "text_to_image" {
		t.Fatalf("operations = %#v", got.SupportedOperations)
	}
	if got.Limits["max_prompt_chars"].(float64) != 8000 {
		t.Fatalf("limits = %#v", got.Limits)
	}
}
