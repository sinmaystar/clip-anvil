package composer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

func TestVolcengineModelResponderAppliesContextCompactionProjection(t *testing.T) {
	longResult := strings.Repeat("ffmpeg stderr line with frame details\n", 180)
	model := &fakeComposerArkModel{final: &schema.Message{Role: schema.Assistant, Content: "ok"}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:     compactComposerResponderTestConfig(),
			Store:      contextcompactTestStore(),
			FileWriter: contextcompactTestFileWriter(),
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{
		WorkspaceID: uuidWithByte(1),
		Input: GraphInput{
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(2),
			TaskID:      uuidWithByte(3),
			Input:       CompositionInput{Instructions: "compose"},
		},
		SameTurnMessages: []ComposerSameTurnMessage{
			{Role: "tool", MessageType: "tool_result", ToolCallID: "call-old", ToolName: "run_ffmpeg_command", Content: longResult},
			{Role: "assistant", MessageType: "tool_call", ToolCallID: "call-current", ToolName: "probe_media", ToolArguments: map[string]any{"path": "/workspace/current.mp4"}},
			{Role: "tool", MessageType: "tool_result", ToolCallID: "call-current", ToolName: "probe_media", Content: "current probe result must stay visible"},
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
	if len(model.messages) < 5 {
		t.Fatalf("messages = %#v", model.messages)
	}
	oldTool := model.messages[2]
	if oldTool.Role != schema.Tool || oldTool.ToolCallID != "call-old" || oldTool.ToolName != "run_ffmpeg_command" {
		t.Fatalf("old tool identity changed: %#v", oldTool)
	}
	if !strings.Contains(oldTool.Content, "compact_ref:") || strings.Contains(oldTool.Content, "ffmpeg stderr line with frame details") {
		t.Fatalf("old tool result not compacted correctly: %s", oldTool.Content)
	}
	currentTool := model.messages[len(model.messages)-1]
	if !strings.Contains(currentTool.Content, "current probe result must stay visible") {
		t.Fatalf("current tool loop result was compacted: %#v", currentTool)
	}
}

func TestVolcengineModelResponderRetriesOnceWithFullCompactOnContextOverflow(t *testing.T) {
	model := &fakeComposerArkModel{
		final:        &schema.Message{Role: schema.Assistant, Content: "ok"},
		generateErrs: []error{errors.New("context length exceeded")},
	}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:         compactComposerResponderTestConfig(),
			Store:          contextcompactTestStore(),
			FileWriter:     contextcompactTestFileWriter(),
			FullSummarizer: contextcompact.StaticFullSummarizer{},
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{
		WorkspaceID: uuidWithByte(1),
		Input: GraphInput{
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(2),
			TaskID:      uuidWithByte(3),
			Input:       CompositionInput{Instructions: "compose"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Metadata["context_compaction_retry"] != true {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
	if model.generateCalls != 2 {
		t.Fatalf("generateCalls = %d, want 2", model.generateCalls)
	}
	if !strings.Contains(model.messages[1].Content, "# Compacted Agent Handoff Summary") {
		t.Fatalf("retry prompt missing full summary: %#v", model.messages)
	}
}

func TestComposerCompactsOldProbeOutputButPreservesCurrentTimelinePath(t *testing.T) {
	longProbe := strings.Repeat("ffprobe stream codec duration bitrate\n", 1200)
	model := &fakeComposerArkModel{final: &schema.Message{Role: schema.Assistant, Content: "ok"}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:         compactComposerResponderTestConfig(),
			Store:          contextcompactTestStore(),
			FileWriter:     contextcompactTestFileWriter(),
			FullSummarizer: contextcompact.StaticFullSummarizer{},
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})

	_, err := responder.Respond(context.Background(), Context{
		WorkspaceID: uuidWithByte(1),
		Input: GraphInput{
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(2),
			TaskID:      uuidWithByte(3),
			Input:       CompositionInput{Instructions: "compose"},
		},
		SameTurnMessages: []ComposerSameTurnMessage{
			{Role: "tool", MessageType: "tool_result", ToolCallID: "old-probe", ToolName: "probe_media", Content: longProbe},
			{Role: "assistant", MessageType: "tool_call", ToolCallID: "current-render", ToolName: "render_timeline", ToolArguments: map[string]any{"timeline_path": "/workspace/timeline/current.json"}},
			{Role: "tool", MessageType: "tool_result", ToolCallID: "current-render", ToolName: "render_timeline", Content: "current timeline path /workspace/timeline/current.json must stay visible"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.messages[2].Content, "compact_ref:") {
		t.Fatalf("old probe was not compacted: %#v", model.messages)
	}
	if !strings.Contains(model.messages[len(model.messages)-1].Content, "/workspace/timeline/current.json") {
		t.Fatalf("current timeline path not preserved: %#v", model.messages)
	}
}

type fakeComposerArkModel struct {
	messages      []*schema.Message
	final         *schema.Message
	generateErrs  []error
	generateCalls int
}

func (f *fakeComposerArkModel) Generate(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.Message, error) {
	f.messages = messages
	if f.generateCalls < len(f.generateErrs) && f.generateErrs[f.generateCalls] != nil {
		f.generateCalls++
		return nil, f.generateErrs[f.generateCalls-1]
	}
	f.generateCalls++
	return f.final, nil
}

func (f *fakeComposerArkModel) Stream(context.Context, []*schema.Message, ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}
