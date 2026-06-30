package producer

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

func compactResponderTestConfig() contextcompact.Config {
	cfg := contextcompact.DefaultConfig()
	cfg.MicroTriggerTokens = 100
	cfg.MicroTargetTokens = 40
	cfg.MicroMinReductionTokens = 1
	cfg.PreserveRecentUserMessages = 1
	cfg.PreserveRecentTotalMessages = 1
	return cfg
}

func contextcompactTestStore() *producerCompactionStore {
	return &producerCompactionStore{records: map[string]contextcompact.CompactionRecord{}}
}

func contextcompactTestFileWriter() *producerCompactionFileWriter {
	return &producerCompactionFileWriter{}
}

func mustJSONString(t interface{ Fatalf(string, ...any) }, value string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON string: %v", err)
	}
	return string(raw)
}

type producerCompactionStore struct {
	mu      sync.Mutex
	nextID  byte
	records map[string]contextcompact.CompactionRecord
}

func (s *producerCompactionStore) CreateCompaction(_ context.Context, input contextcompact.CreateCompactionInput) (contextcompact.CompactionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.records[input.SemanticKey]; ok {
		return record, nil
	}
	s.nextID++
	record := contextcompact.CompactionRecord{
		ID:                     uuidWithByte(s.nextID),
		WorkspaceID:            input.WorkspaceID,
		ThreadID:               input.ThreadID,
		TaskID:                 input.TaskID,
		Role:                   input.Role,
		Mode:                   input.Mode,
		Trigger:                input.Trigger,
		SemanticKey:            input.SemanticKey,
		SourceSeqStart:         input.SourceSeqStart,
		SourceSeqEnd:           input.SourceSeqEnd,
		OriginalTokenEstimate:  input.OriginalTokenEstimate,
		CompactedTokenEstimate: input.CompactedTokenEstimate,
		OriginalBytes:          input.OriginalBytes,
		Summary:                input.Summary,
		DetailFiles:            append([]string(nil), input.DetailFiles...),
	}
	s.records[input.SemanticKey] = record
	return record, nil
}

func (s *producerCompactionStore) LinkMessage(context.Context, contextcompact.LinkMessageInput) error {
	return nil
}

func (s *producerCompactionStore) CompactedMessageIDs(context.Context, pgtype.UUID, pgtype.UUID) (map[pgtype.UUID]contextcompact.CompactionRecord, error) {
	return map[pgtype.UUID]contextcompact.CompactionRecord{}, nil
}

func (s *producerCompactionStore) GetBySemanticKey(_ context.Context, _ pgtype.UUID, key string) (contextcompact.CompactionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key]
	if !ok {
		return contextcompact.CompactionRecord{}, contextcompact.ErrCompactionNotFound
	}
	return record, nil
}

func (s *producerCompactionStore) Search(context.Context, contextcompact.SearchInput) ([]contextcompact.CompactionRecord, error) {
	return nil, nil
}

type producerCompactionFileWriter struct{}

func (producerCompactionFileWriter) WriteDetailFile(_ context.Context, input contextcompact.DetailFileInput) (contextcompact.DetailFileResult, error) {
	return contextcompact.DetailFileResult{
		Path:  "/workspace/.clipanvil/context/" + input.Role + "-0-0-0123456789abcdef.md",
		Hash:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Bytes: int64(len([]byte(input.Original))),
	}, nil
}
