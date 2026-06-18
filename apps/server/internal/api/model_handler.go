package api

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ModelHandler struct {
	queries *db.Queries
}

func NewModelHandler(queries *db.Queries) *ModelHandler {
	return &ModelHandler{queries: queries}
}

type modelCapabilityResponse struct {
	ProviderID              string         `json:"provider_id"`
	ModelID                 string         `json:"model_id"`
	DisplayName             string         `json:"display_name"`
	OutputTypes             []string       `json:"output_types"`
	SupportedOperations     []string       `json:"supported_operations"`
	SupportedInputNodeTypes []string       `json:"supported_input_node_types"`
	Limits                  map[string]any `json:"limits"`
	Pricing                 map[string]any `json:"pricing"`
	Defaults                map[string]any `json:"defaults"`
	Enabled                 bool           `json:"enabled"`
}

func (h *ModelHandler) ListCapabilities(ctx context.Context, c *app.RequestContext) {
	if _, ok := accountIDFromContext(c); !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	rows, err := h.queries.ListEnabledModelCapabilities(ctx)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list model capabilities")
		return
	}
	resp := make([]modelCapabilityResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, toModelCapabilityResponse(row))
	}
	c.JSON(consts.StatusOK, resp)
}

func toModelCapabilityResponse(row db.ModelCapability) modelCapabilityResponse {
	return modelCapabilityResponse{
		ProviderID:              row.ProviderID,
		ModelID:                 row.ModelID,
		DisplayName:             row.DisplayName,
		OutputTypes:             stringList(row.OutputTypes),
		SupportedOperations:     stringList(row.SupportedOperations),
		SupportedInputNodeTypes: stringList(row.SupportedInputNodeTypes),
		Limits:                  jsonMap(row.Limits),
		Pricing:                 jsonMap(row.Pricing),
		Defaults:                jsonMap(row.Defaults),
		Enabled:                 row.Enabled,
	}
}

func stringList(raw []byte) []string {
	out := []string{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func jsonMap(raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}
