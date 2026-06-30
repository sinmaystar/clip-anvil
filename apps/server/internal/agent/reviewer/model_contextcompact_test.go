package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestVolcengineReviewerResponderAppliesContextCompactionAndProtectsCurrentReviewMedia(t *testing.T) {
	longResult := strings.Repeat("review history diagnostic with artifact pixels and rubric notes\n", 180)
	store := reviewerContextcompactTestStore()
	streamer := &fakeReviewArkStreamer{chunks: []*schema.Message{{Content: "ok"}}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-reviewer",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:     compactReviewerResponderTestConfig(),
			Store:      store,
			FileWriter: reviewerContextcompactTestFileWriter(),
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})
	originalContent := []byte(`{"text":` + mustReviewerJSON(t, longResult) + `}`)

	out, err := responder.Respond(context.Background(), Context{
		Input:     GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3)},
		Text:      "Review Target\n- shot: shot_01\n- phase: preview_image\n- must keep this current review target",
		AssetURL:  "data:image/png;base64,iVBORw0KGgo=",
		AssetMime: "image/png",
		Messages: []db.AgentMessage{
			{
				ID:          uuidWithByte(10),
				Role:        "tool",
				MessageType: "tool_result",
				Content:     originalContent,
				RawMessage:  []byte(`{"tool_call_id":"call-review-old","tool_name":"submit_review_result","result_text":` + mustReviewerJSON(t, longResult) + `}`),
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
	if len(streamer.messages) < 3 {
		t.Fatalf("messages = %#v", streamer.messages)
	}
	tool := streamer.messages[1]
	if tool.Role != schema.Tool || tool.ToolCallID != "call-review-old" || tool.ToolName != "submit_review_result" {
		t.Fatalf("tool identity changed: %#v", tool)
	}
	if !strings.Contains(tool.Content, "compact_ref:") || !strings.Contains(tool.Content, "detail_file: /workspace/.clipanvil/context/") {
		t.Fatalf("tool result was not compacted: %s", tool.Content)
	}
	if strings.Contains(tool.Content, "review history diagnostic with artifact pixels") {
		t.Fatal("provider input still contains original long tool result")
	}
	current := streamer.messages[len(streamer.messages)-1]
	if current.Role != schema.User || len(current.UserInputMultiContent) != 2 {
		t.Fatalf("current review media message was not preserved: %#v", current)
	}
	if current.UserInputMultiContent[1].Image == nil || current.UserInputMultiContent[1].Image.URL == nil || *current.UserInputMultiContent[1].Image.URL != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatalf("current review image input was not preserved: %#v", current.UserInputMultiContent)
	}
	if len(store.links) != 1 || store.links[0].MessageID != uuidWithByte(10) {
		t.Fatalf("source message links = %#v", store.links)
	}
	if string(originalContent) != `{"text":`+mustReviewerJSON(t, longResult)+`}` {
		t.Fatal("original agent_message content was mutated")
	}
}
