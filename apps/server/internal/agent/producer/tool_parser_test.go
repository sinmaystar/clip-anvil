package producer

import "testing"

func TestParseToolCallFromJSONFence(t *testing.T) {
	out, err := ParseToolCall(`上一段文字
` + "```json" + `
{"tool_call":{"name":"create_agent_text_node","arguments":{"title":"商品 brief","text":"低糖燕麦拿铁"}}}
` + "```")
	if err != nil {
		t.Fatal(err)
	}
	if !out.HasToolCall || out.ToolCall.Name != "create_agent_text_node" {
		t.Fatalf("parsed = %#v", out)
	}
	if out.ToolCall.Arguments["title"] != "商品 brief" {
		t.Fatalf("arguments = %#v", out.ToolCall.Arguments)
	}
}

func TestParseToolCallFromFunctionCallBlock(t *testing.T) {
	out, err := ParseToolCall(`我先保存分镜。
<|FunctionCallBegin|>[{"name":"update_storyboard","parameters":{"storyboard_shots":[{"shot_number":1,"duration":4,"content":"开场"}]}}]<|FunctionCallEnd|>`)
	if err != nil {
		t.Fatal(err)
	}
	if !out.HasToolCall || out.ToolCall.Name != "update_storyboard" {
		t.Fatalf("parsed = %#v", out)
	}
	if _, ok := out.ToolCall.Arguments["storyboard_shots"]; !ok {
		t.Fatalf("arguments = %#v", out.ToolCall.Arguments)
	}
}

func TestParseToolCallReturnsTextWhenNoToolCall(t *testing.T) {
	out, err := ParseToolCall("普通回复")
	if err != nil {
		t.Fatal(err)
	}
	if out.HasToolCall {
		t.Fatalf("parsed tool unexpectedly: %#v", out)
	}
	if out.Text != "普通回复" {
		t.Fatalf("text = %q", out.Text)
	}
}

func TestParseToolCallRejectsUnknownShape(t *testing.T) {
	_, err := ParseToolCall(`{"tool_call":{"arguments":{}}}`)
	if err == nil {
		t.Fatal("expected invalid tool call error")
	}
}
