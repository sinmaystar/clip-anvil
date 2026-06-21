package api

import (
	"encoding/json"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type mediaNodeResponse struct {
	db.MediaNode
	PromptRich  json.RawMessage `json:"prompt_rich"`
	PromptRefs  json.RawMessage `json:"prompt_refs"`
	ModelParams json.RawMessage `json:"model_params"`
	Metadata    json.RawMessage `json:"metadata"`
}

func toMediaNodeResponse(node db.MediaNode) mediaNodeResponse {
	return mediaNodeResponse{
		MediaNode:   node,
		PromptRich:  rawJSONOrNull(node.PromptRich),
		PromptRefs:  rawJSONOrNull(node.PromptRefs),
		ModelParams: rawJSONOrNull(node.ModelParams),
		Metadata:    rawJSONOrNull(node.Metadata),
	}
}

func rawJSONOrNull(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(raw)
}
