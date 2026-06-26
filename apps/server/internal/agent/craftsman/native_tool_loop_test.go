package craftsman

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestPrepareCraftsmanToolMessagePreservesInvalidArguments(t *testing.T) {
	stateStore := newCraftsmanLoopToolStateStore()
	state := craftsmanLoopState{
		Context: Context{Input: GraphInput{}},
		LastAssistantMessage: &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   "call_bad_json",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "upsert_render_plan",
					Arguments: `{"brief":"创建计划","rationale":"未结束`,
				},
			}},
		},
	}

	message, err := prepareCraftsmanToolMessage(stateStore, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", message.ToolCalls)
	}
	args := message.ToolCalls[0].Function.Arguments
	if strings.Contains(args, "_raw") || strings.Contains(args, craftsmanLoopStateArgumentKey) {
		t.Fatalf("invalid arguments should not be wrapped or mutated: %s", args)
	}
	if args != `{"brief":"创建计划","rationale":"未结束` {
		t.Fatalf("arguments = %q", args)
	}
	if _, ok := stateStore.stateByCall("call_bad_json"); !ok {
		t.Fatal("state was not remembered for the original tool call")
	}
}
