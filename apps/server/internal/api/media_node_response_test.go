package api

import (
	"encoding/json"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestMediaNodeResponseWritesJSONBAsJSON(t *testing.T) {
	response := toMediaNodeResponse(db.MediaNode{
		PromptRefs:  []byte(`{"version":1,"refs":[]}`),
		PromptRich:  []byte(`{"version":1,"source":"textarea-at","text":"hello"}`),
		ModelParams: []byte(`{"temperature":0.2}`),
		Metadata:    []byte(`{}`),
	})

	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["prompt_refs"].(map[string]any); !ok {
		t.Fatalf("prompt_refs = %#v, want object", body["prompt_refs"])
	}
	if _, ok := body["prompt_rich"].(map[string]any); !ok {
		t.Fatalf("prompt_rich = %#v, want object", body["prompt_rich"])
	}
	if _, ok := body["model_params"].(map[string]any); !ok {
		t.Fatalf("model_params = %#v, want object", body["model_params"])
	}
}
