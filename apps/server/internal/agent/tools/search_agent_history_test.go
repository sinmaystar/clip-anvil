package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

func TestSearchAgentHistoryRequiresRuntimeWorkspace(t *testing.T) {
	tool := NewSearchAgentHistoryNativeTool(newFakeHistoryStore(), contextcompact.DefaultConfig())
	got, err := tool.InvokableRun(context.Background(), `{"compact_ref":"ctxcmp:producer:test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "工具调用失败") {
		t.Fatalf("result = %q, want natural tool error", got)
	}
}

func TestSearchAgentHistoryReturnsExactCompactRef(t *testing.T) {
	store := newFakeHistoryStore()
	record := contextcompact.CompactionRecord{
		ID:               uuidWithByte(9),
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		Role:             "producer",
		Mode:             "micro",
		SemanticKey:      "ctxcmp:producer:abc",
		SourceSeqStart:   3,
		SourceSeqEnd:     3,
		SourceMessageIDs: []string{"message-1"},
		SourceMediaRefs:  []string{"artifact:video:1"},
		Summary:          "ffmpeg stderr 已压缩",
		DetailFiles:      []string{"/workspace/.clipanvil/context/producer-3-3-abcd.md"},
	}
	store.records[record.SemanticKey] = record
	tool := NewSearchAgentHistoryNativeTool(store, contextcompact.DefaultConfig())

	got, err := tool.InvokableRun(historyToolContext(), `{"compact_ref":"ctxcmp:producer:abc"}`)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Matches []struct {
			CompactRef      string   `json:"compact_ref"`
			Summary         string   `json:"summary"`
			DetailFiles     []string `json:"detail_files"`
			SourceMessageID []string `json:"source_message_ids"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal %q: %v", got, err)
	}
	if len(out.Matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(out.Matches))
	}
	if out.Matches[0].CompactRef != record.SemanticKey || out.Matches[0].DetailFiles[0] != record.DetailFiles[0] {
		t.Fatalf("unexpected match: %#v", out.Matches[0])
	}
}

func TestSearchAgentHistoryReturnsFullCompactMode(t *testing.T) {
	store := newFakeHistoryStore()
	record := contextcompact.CompactionRecord{
		ID:           uuidWithByte(9),
		WorkspaceID:  uuidWithByte(1),
		ThreadID:     uuidWithByte(2),
		Role:         "producer",
		Mode:         "full",
		SemanticKey:  "ctxcmp:producer:full",
		Summary:      "handoff summary",
		DetailFiles:  []string{"/workspace/.clipanvil/context/producer-full.md"},
		SourceSeqEnd: 42,
	}
	store.records[record.SemanticKey] = record
	tool := NewSearchAgentHistoryNativeTool(store, contextcompact.DefaultConfig())

	got, err := tool.InvokableRun(historyToolContext(), `{"compact_ref":"ctxcmp:producer:full"}`)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Matches []struct {
			CompactRef  string   `json:"compact_ref"`
			Mode        string   `json:"mode"`
			DetailFiles []string `json:"detail_files"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal %q: %v", got, err)
	}
	if len(out.Matches) != 1 || out.Matches[0].Mode != "full" || out.Matches[0].DetailFiles[0] != record.DetailFiles[0] {
		t.Fatalf("matches = %#v", out.Matches)
	}
}

func TestSearchAgentHistoryUsesRuntimeThreadByDefault(t *testing.T) {
	store := newFakeHistoryStore()
	tool := NewSearchAgentHistoryNativeTool(store, contextcompact.DefaultConfig())
	_, err := tool.InvokableRun(historyToolContext(), `{"query":"ffmpeg","limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if store.lastSearch.ThreadID != uuidWithByte(2) {
		t.Fatalf("ThreadID = %#v, want runtime thread", store.lastSearch.ThreadID)
	}
	if store.lastSearch.Limit != 1 {
		t.Fatalf("Limit = %d, want 1", store.lastSearch.Limit)
	}
}

func historyToolContext() context.Context {
	return WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
	})
}

type fakeHistoryStore struct {
	records    map[string]contextcompact.CompactionRecord
	lastSearch contextcompact.SearchInput
}

func newFakeHistoryStore() *fakeHistoryStore {
	return &fakeHistoryStore{records: map[string]contextcompact.CompactionRecord{}}
}

func (s *fakeHistoryStore) CreateCompaction(context.Context, contextcompact.CreateCompactionInput) (contextcompact.CompactionRecord, error) {
	return contextcompact.CompactionRecord{}, nil
}

func (s *fakeHistoryStore) LinkMessage(context.Context, contextcompact.LinkMessageInput) error {
	return nil
}

func (s *fakeHistoryStore) CompactedMessageIDs(context.Context, pgtype.UUID, pgtype.UUID) (map[pgtype.UUID]contextcompact.CompactionRecord, error) {
	return map[pgtype.UUID]contextcompact.CompactionRecord{}, nil
}

func (s *fakeHistoryStore) GetBySemanticKey(_ context.Context, _ pgtype.UUID, key string) (contextcompact.CompactionRecord, error) {
	record, ok := s.records[key]
	if !ok {
		return contextcompact.CompactionRecord{}, contextcompact.ErrCompactionNotFound
	}
	return record, nil
}

func (s *fakeHistoryStore) Search(_ context.Context, input contextcompact.SearchInput) ([]contextcompact.CompactionRecord, error) {
	s.lastSearch = input
	out := make([]contextcompact.CompactionRecord, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record)
	}
	return out, nil
}
