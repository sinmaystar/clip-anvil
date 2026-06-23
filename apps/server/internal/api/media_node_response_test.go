package api

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

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

func TestMediaNodeResponseIncludesAgentPreviewMetadata(t *testing.T) {
	response := toMediaNodeResponse(db.MediaNode{
		NodeType:         db.NodeTypeImage,
		Source:           "agent",
		OperationType:    "text_to_image",
		Prompt:           "preview prompt",
		ModelProvider:    pgtype.Text{String: "volcengine", Valid: true},
		ModelID:          pgtype.Text{String: "test-image", Valid: true},
		ModelParams:      []byte(`{"size":"1024x1024"}`),
		CurrentVersionID: pgtype.UUID{Bytes: [16]byte{7}, Valid: true},
		ShotID:           pgtype.UUID{Bytes: [16]byte{2}, Valid: true},
		Metadata:         []byte(`{"agent_artifact_kind":"preview_image"}`),
	})

	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["operation_type"] != "text_to_image" || body["source"] != "agent" {
		t.Fatalf("body = %#v", body)
	}
	if metadata, ok := body["metadata"].(map[string]any); !ok || metadata["agent_artifact_kind"] != "preview_image" {
		t.Fatalf("metadata = %#v", body["metadata"])
	}
	if _, ok := body["model_params"].(map[string]any); !ok {
		t.Fatalf("model_params = %#v", body["model_params"])
	}
	if body["shot_id"] == nil || body["current_version_id"] == nil {
		t.Fatalf("body = %#v", body)
	}
}
