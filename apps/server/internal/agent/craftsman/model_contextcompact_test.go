package craftsman

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestVolcengineCraftsmanResponderAppliesContextCompactionProjection(t *testing.T) {
	longResult := strings.Repeat("render plan probe result with source media details\n", 180)
	store := craftsmanContextcompactTestStore()
	model := &fakeCraftsmanArkModel{message: &schema.Message{Role: schema.Assistant, Content: "ok"}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:     compactCraftsmanResponderTestConfig(),
			Store:      store,
			FileWriter: craftsmanContextcompactTestFileWriter(),
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})
	originalContent := []byte(`{"text":` + mustCraftsmanJSON(t, longResult) + `}`)

	out, err := responder.Respond(context.Background(), Context{
		Input: GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3)},
		Text:  "Current Task\n- target_phase: preview_image\n- shot: shot_01\n- must keep this current craftsman task text",
		Messages: []db.AgentMessage{
			{
				ID:          uuidWithByte(9),
				Role:        "tool",
				MessageType: "tool_result",
				Content:     originalContent,
				RawMessage:  []byte(`{"tool_call_id":"call-render-plan","tool_name":"upsert_render_plan","result_text":` + mustCraftsmanJSON(t, longResult) + `}`),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Metadata["context_compaction_applied"] != true {
		t.Fatalf("metadata missing compaction applied: %#v", out.Metadata)
	}
	if out.Metadata["context_compaction_mode"] != "micro" || out.Metadata["context_compaction_count"] != 1 {
		t.Fatalf("metadata missing compaction mode/count: %#v", out.Metadata)
	}
	if refs, ok := out.Metadata["context_compaction_refs"].([]string); !ok || len(refs) == 0 {
		t.Fatalf("metadata missing compaction refs: %#v", out.Metadata)
	}
	if files, ok := out.Metadata["context_compaction_detail_files"].([]string); !ok || len(files) == 0 {
		t.Fatalf("metadata missing compaction detail files: %#v", out.Metadata)
	}
	if len(model.messages) < 3 {
		t.Fatalf("messages = %#v", model.messages)
	}
	tool := model.messages[1]
	if tool.Role != schema.Tool || tool.ToolCallID != "call-render-plan" || tool.ToolName != "upsert_render_plan" {
		t.Fatalf("tool identity changed: %#v", tool)
	}
	if !strings.Contains(tool.Content, "compact_ref:") || !strings.Contains(tool.Content, "detail_file: /workspace/.clipanvil/context/") {
		t.Fatalf("tool result was not compacted: %s", tool.Content)
	}
	if strings.Contains(tool.Content, "render plan probe result with source media details") {
		t.Fatal("provider input still contains original long tool result")
	}
	current := model.messages[len(model.messages)-1]
	if current.Role != schema.User || !strings.Contains(current.Content, "must keep this current craftsman task text") {
		t.Fatalf("current craftsman task text was not preserved: %#v", current)
	}
	if len(store.links) != 1 || store.links[0].MessageID != uuidWithByte(9) {
		t.Fatalf("source message links = %#v", store.links)
	}
	if string(originalContent) != `{"text":`+mustCraftsmanJSON(t, longResult)+`}` {
		t.Fatal("original agent_message content was mutated")
	}
}
