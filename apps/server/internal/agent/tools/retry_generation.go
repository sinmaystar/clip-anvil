package tools

import (
	"context"
	"errors"
)

type RetryGenerationTool struct {
	store    CraftsmanDispatcherStore
	runtime  CraftsmanRuntime
	enqueuer CraftsmanTaskEnqueuer
}

func NewRetryGenerationTool(store CraftsmanDispatcherStore, runtime CraftsmanRuntime, enqueuer CraftsmanTaskEnqueuer) RetryGenerationTool {
	return RetryGenerationTool{store: store, runtime: runtime, enqueuer: enqueuer}
}

func (t RetryGenerationTool) Definition() Definition {
	return Definition{
		Name:        "retry_generation",
		Description: "Retry a shot preview generation using a review critique or user revision instruction. This dispatches the existing Craftsman/Worker preview pipeline with force=true.",
		Parameters: objectSchema(map[string]any{
			"shot_ref":         map[string]any{"type": "string"},
			"target_phase":     map[string]any{"type": "string", "enum": []string{"preview_image"}},
			"review_record_id": map[string]any{"type": "string"},
			"critique":         map[string]any{"type": "string"},
			"fix_hints":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"max_attempts":     map[string]any{"type": "integer", "minimum": 1, "maximum": 3},
		}),
		Result:     map[string]any{"type": "object"},
		Safety:     SafetySpec{UsesProductionService: true, MaxCallsPerTurn: 5},
		Visibility: VisibilitySpec{ShowCallMessage: true, ShowResultMessage: true, UserLabel: "重新生成"},
	}
}

func (t RetryGenerationTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.store == nil || t.runtime == nil {
		return ExecuteOutput{}, errors.New("retry_generation service is not configured")
	}
	shotRef := stringValue(input.Arguments, "shot_ref")
	if shotRef == "" {
		return ExecuteOutput{}, errors.New("retry_generation requires shot_ref")
	}
	dispatch := NewDispatchCraftsmanTool(t.store, t.runtime, t.enqueuer)
	args := map[string]any{
		"mode":             "preview_image",
		"shot_refs":        []any{shotRef},
		"force":            true,
		"max_attempts":     int32Value(input.Arguments, "max_attempts", 3),
		"review_record_id": stringValue(input.Arguments, "review_record_id"),
		"critique":         stringValue(input.Arguments, "critique"),
		"fix_hints":        stringSliceValue(input.Arguments, "fix_hints"),
	}
	out, err := dispatch.Execute(ctx, ExecuteInput{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		Arguments:   args,
	})
	if err != nil {
		return ExecuteOutput{}, err
	}
	return ExecuteOutput{Summary: "已根据评审意见重新生成该分镜预览图。", Result: out.Result}, nil
}
