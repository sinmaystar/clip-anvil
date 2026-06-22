package producer

import (
	"context"
	"fmt"
	"strings"
)

type Responder interface {
	Respond(ctx context.Context, producerContext ProducerContext) (ProducerTurnOutput, error)
}

type DeterministicResponder struct{}

func (DeterministicResponder) Respond(_ context.Context, producerContext ProducerContext) (ProducerTurnOutput, error) {
	text := strings.TrimSpace(producerContext.LatestUserText)
	if text == "" {
		text = "你的需求"
	}
	return ProducerTurnOutput{
		AssistantText: fmt.Sprintf("我已收到你的需求：「%s」。\n下一步我会先整理创作目标，再在后续阶段拆成分镜和生产任务。", text),
		Metadata: map[string]any{
			"responder": "deterministic",
		},
	}, nil
}
