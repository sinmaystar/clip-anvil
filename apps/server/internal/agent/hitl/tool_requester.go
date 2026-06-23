package hitl

import (
	"context"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
)

type ToolDecisionRequester struct {
	service *Service
}

func NewToolDecisionRequester(service *Service) ToolDecisionRequester {
	return ToolDecisionRequester{service: service}
}

func (r ToolDecisionRequester) RequestUserDecision(ctx context.Context, input agenttools.ExecuteInput) (agenttools.ExecuteOutput, error) {
	checkpointKey := agenteino.CheckpointKey("producer_turn", input.WorkspaceID, input.ThreadID, input.TaskID)
	output, err := r.service.RequestUserDecision(ctx, RequestDecisionInput{
		WorkspaceID:     input.WorkspaceID,
		ThreadID:        input.ThreadID,
		TaskID:          input.TaskID,
		CheckpointKey:   checkpointKey,
		Arguments:       input.Arguments,
		CheckpointValue: mustJSON(map[string]any{"tool": "request_user_decision", "arguments": input.Arguments}),
	})
	if err != nil {
		return agenttools.ExecuteOutput{}, err
	}
	return agenttools.ExecuteOutput{Interrupted: true, Summary: "已请求用户决策，ProducerGraph 已暂停等待用户输入。", Result: map[string]any{
		"decision_id":    output.EventID,
		"status":         "waiting_for_user",
		"checkpoint_key": checkpointKey,
	}}, nil
}
