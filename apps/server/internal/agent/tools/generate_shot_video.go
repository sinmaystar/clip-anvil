package tools

import (
	"context"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type GenerateShotVideoTool struct {
	dispatcher DispatchCraftsmanTool
}

func NewGenerateShotVideoTool(store CraftsmanDispatcherStore, runtime CraftsmanRuntime, enqueuer CraftsmanTaskEnqueuer) GenerateShotVideoTool {
	return GenerateShotVideoTool{dispatcher: NewDispatchCraftsmanTool(store, runtime, enqueuer)}
}

func (t GenerateShotVideoTool) Definition() Definition {
	return Definition{
		Name:        "generate_shot_video",
		Description: "Generate shot-level videos from accepted/current preview images. This schedules persistent Craftsman/Worker tasks and reuses the production image-to-video pipeline; the tool result only means work has been queued.",
		Parameters: objectSchema(map[string]any{
			"shot_refs": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Shot semantic keys or stable client keys such as shot-01. Empty means all active planned shots.",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "When true, create a new shot-video attempt even if a shot already has video output.",
			},
			"max_attempts": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     3,
				"description": "Fixed retry cap. Defaults to 3.",
			},
			"input_node_refs": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional explicit preview image node refs. If omitted, each shot uses its default preview image node title.",
			},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			UsesProductionService: true,
			MaxCallsPerTurn:       5,
		},
		Visibility: VisibilitySpec{
			ShowCallMessage:   true,
			ShowResultMessage: true,
			UserLabel:         "开始生成分镜视频",
		},
	}
}

func (t GenerateShotVideoTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	args := map[string]any{
		"mode": "shot_video",
	}
	for key, value := range input.Arguments {
		args[key] = value
	}
	input.Arguments = args
	return t.dispatcher.Execute(ctx, input)
}

var _ Executor = GenerateShotVideoTool{}
var _ CraftsmanRuntime = (*agentruntime.Service)(nil)
var _ CraftsmanDispatcherStore = (*db.Queries)(nil)
