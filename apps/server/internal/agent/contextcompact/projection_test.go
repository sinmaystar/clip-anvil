package contextcompact

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestProjectMicroCompactionReplacesHistoricalToolResultOnlyInProjection(t *testing.T) {
	originalLong := strings.Repeat("ffmpeg stderr line with codec details\n", 160)
	messages := []*schema.Message{
		schema.SystemMessage("system prompt"),
		schema.UserMessage("create an ad"),
		{
			Role:       schema.Tool,
			ToolCallID: "call-old",
			ToolName:   "run_ffmpeg_command",
			Content:    originalLong,
		},
		schema.UserMessage("latest user intent must stay visible"),
	}
	store := newMemoryStore()
	middleware := NewMiddleware(MiddlewareConfig{
		Config: compactTestConfig(CompactionThresholds{
			MicroTriggerTokens:          100,
			MicroTargetTokens:           40,
			MicroMinReductionTokens:     1,
			PreserveRecentUserMessages:  1,
			PreserveRecentTotalMessages: 1,
		}),
		Store:      store,
		FileWriter: newMemoryDetailFileWriter(),
	})

	out, err := middleware.Project(context.Background(), ProjectionInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Role:        "producer",
		ModelID:     "doubao-seed-2-0-mini-260428",
		Messages:    messages,
		MessageRefs: []SourceMessageRef{{MessageIndex: 2, MessageID: uuidWithByte(4)}},
		Trigger:     "micro_threshold",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Applied) != 1 {
		t.Fatalf("len(Applied) = %d, want 1", len(out.Applied))
	}
	projected := out.Messages[2]
	if projected == messages[2] {
		t.Fatal("projection reused original message pointer")
	}
	if projected.Role != schema.Tool || projected.ToolCallID != "call-old" || projected.ToolName != "run_ffmpeg_command" {
		t.Fatalf("projected tool identity changed: %#v", projected)
	}
	if !strings.Contains(projected.Content, "compact_ref:") || !strings.Contains(projected.Content, "detail_file: /workspace/.clipanvil/context/") {
		t.Fatalf("projected content missing recovery refs: %s", projected.Content)
	}
	if strings.Contains(projected.Content, "ffmpeg stderr line with codec details") {
		t.Fatal("projected content still contains original long output")
	}
	if messages[2].Content != originalLong {
		t.Fatal("original message content was mutated")
	}
	if out.OriginalUnchanged != true {
		t.Fatal("OriginalUnchanged = false, want true")
	}
	if out.TokenAfter >= out.TokenBefore {
		t.Fatalf("token after = %d, before = %d, want reduction", out.TokenAfter, out.TokenBefore)
	}
	if len(store.links) != 1 || store.links[0].MessageID != uuidWithByte(4) {
		t.Fatalf("message compaction links = %#v, want source message linked", store.links)
	}
}

func TestProjectMicroCompactionLinksSourceMessageWhenRecordAlreadyExists(t *testing.T) {
	originalLong := strings.Repeat("stored tool result with repeatable semantic hash\n", 1200)
	messages := []*schema.Message{
		schema.SystemMessage("system prompt"),
		schema.UserMessage("older user intent"),
		{Role: schema.Tool, ToolCallID: "call-old", ToolName: "run_ffmpeg_command", Content: originalLong},
		schema.UserMessage("latest user intent must stay visible"),
	}
	store := newMemoryStore()
	middleware := NewMiddleware(MiddlewareConfig{
		Config: compactTestConfig(CompactionThresholds{
			MicroTriggerTokens:          100,
			MicroTargetTokens:           40,
			MicroMinReductionTokens:     1,
			PreserveRecentUserMessages:  1,
			PreserveRecentTotalMessages: 1,
		}),
		Store:      store,
		FileWriter: newMemoryDetailFileWriter(),
	})
	input := ProjectionInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Role:        "producer",
		ModelID:     "doubao-seed-2-0-mini-260428",
		Messages:    messages,
		MessageRefs: []SourceMessageRef{{MessageIndex: 2, MessageID: uuidWithByte(4)}},
		Trigger:     "micro_threshold",
	}

	if _, err := middleware.Project(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	store.links = nil

	if _, err := middleware.Project(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if len(store.links) != 1 || store.links[0].MessageID != uuidWithByte(4) {
		t.Fatalf("existing compaction link = %#v, want new source message linked", store.links)
	}
}

func TestProjectMicroCompactionSkipsSameTurnAndPendingReminderMessages(t *testing.T) {
	longText := strings.Repeat("tool output ", 1200)
	messages := []*schema.Message{
		schema.SystemMessage("system prompt"),
		{Role: schema.Tool, ToolCallID: "call-same-turn", ToolName: "read_project_context", Content: longText},
		schema.UserMessage("pending reminder: continue current tool loop"),
	}
	middleware := NewMiddleware(MiddlewareConfig{
		Config: compactTestConfig(CompactionThresholds{
			MicroTriggerTokens:          100,
			MicroTargetTokens:           40,
			MicroMinReductionTokens:     1,
			PreserveRecentUserMessages:  0,
			PreserveRecentTotalMessages: 0,
		}),
		Store:      newMemoryStore(),
		FileWriter: newMemoryDetailFileWriter(),
	})

	out, err := middleware.Project(context.Background(), ProjectionInput{
		WorkspaceID:       uuidWithByte(1),
		ThreadID:          uuidWithByte(2),
		Role:              "composer",
		ModelID:           "doubao-seed-2-0-mini-260428",
		Messages:          messages,
		SameTurnFromIndex: 1,
		PendingFromIndex:  2,
		Trigger:           "micro_threshold",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Applied) != 0 {
		t.Fatalf("len(Applied) = %d, want 0", len(out.Applied))
	}
	if out.Messages[1].Content != longText {
		t.Fatal("same-turn tool result was compacted")
	}
	if out.Messages[2].Content != "pending reminder: continue current tool loop" {
		t.Fatal("pending reminder message was compacted")
	}
}

func TestDetailFileWriterUsesDeterministicWorkspacePath(t *testing.T) {
	writer := newMemoryDetailFileWriter()
	input := DetailFileInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		Role:        "producer",
		SeqStart:    10,
		SeqEnd:      11,
		ToolName:    "run_ffmpeg_command",
		ToolCallID:  "call-123",
		Original:    "original stderr",
	}
	first, err := writer.WriteDetailFile(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := writer.WriteDetailFile(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != second.Path {
		t.Fatalf("paths differ: %s vs %s", first.Path, second.Path)
	}
	if !strings.HasPrefix(first.Path, "/workspace/.clipanvil/context/producer-10-11-") {
		t.Fatalf("path = %s, want context detail path", first.Path)
	}
	if !strings.Contains(writer.files[first.Path], "tool_name: run_ffmpeg_command") ||
		!strings.Contains(writer.files[first.Path], "tool_call_id: call-123") ||
		!strings.Contains(writer.files[first.Path], "original stderr") {
		t.Fatalf("detail file missing metadata/original: %s", writer.files[first.Path])
	}
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}

func compactTestConfig(thresholds CompactionThresholds) Config {
	cfg := DefaultConfig()
	cfg.MicroTriggerTokens = thresholds.MicroTriggerTokens
	cfg.MicroTargetTokens = thresholds.MicroTargetTokens
	cfg.MicroMinReductionTokens = thresholds.MicroMinReductionTokens
	cfg.PreserveRecentUserMessages = thresholds.PreserveRecentUserMessages
	cfg.PreserveRecentTotalMessages = thresholds.PreserveRecentTotalMessages
	return cfg
}
