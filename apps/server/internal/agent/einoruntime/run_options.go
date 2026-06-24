package einoruntime

import (
	"context"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
	"github.com/jackc/pgx/v5/pgtype"
)

type RunOptions struct {
	CheckPointID string
	ForceNewRun  bool
	ResumeData   map[string]any
	Callbacks    []callbacks.Handler
}

func ApplyRunOptions(ctx context.Context, options RunOptions) (context.Context, []compose.Option) {
	out := make([]compose.Option, 0, 3)
	if options.CheckPointID != "" {
		out = append(out, compose.WithCheckPointID(options.CheckPointID))
	}
	if options.ForceNewRun {
		out = append(out, compose.WithForceNewRun())
	}
	if len(options.Callbacks) > 0 {
		out = append(out, compose.WithCallbacks(options.Callbacks...))
	}
	if len(options.ResumeData) > 0 {
		ctx = compose.BatchResumeWithData(ctx, options.ResumeData)
	}
	return ctx, out
}

func ResumeDecisionData(eventID pgtype.UUID, selectedOptionID string, freeText string) map[string]any {
	return map[string]any{
		"decision_event_id":  uuidString(eventID),
		"selected_option_id": selectedOptionID,
		"free_text":          freeText,
	}
}
