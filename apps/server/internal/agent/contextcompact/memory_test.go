package contextcompact

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
)

type memoryStore struct {
	mu      sync.Mutex
	nextID  byte
	records map[string]CompactionRecord
	links   []LinkMessageInput
}

func newMemoryStore() *memoryStore {
	return &memoryStore{nextID: 10, records: map[string]CompactionRecord{}}
}

func (s *memoryStore) CreateCompaction(_ context.Context, input CreateCompactionInput) (CompactionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.records[input.SemanticKey]; ok {
		return record, nil
	}
	s.nextID++
	record := CompactionRecord{
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
		SourceMessageIDs:       append([]string(nil), input.SourceMessageIDs...),
		SourceMediaRefs:        append([]string(nil), input.SourceMediaRefs...),
		OriginalTokenEstimate:  input.OriginalTokenEstimate,
		CompactedTokenEstimate: input.CompactedTokenEstimate,
		OriginalBytes:          input.OriginalBytes,
		Summary:                input.Summary,
		DetailFiles:            append([]string(nil), input.DetailFiles...),
		Payload:                input.Payload,
	}
	s.records[input.SemanticKey] = record
	return record, nil
}

func (s *memoryStore) LinkMessage(_ context.Context, input LinkMessageInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links = append(s.links, input)
	return nil
}

func (s *memoryStore) CompactedMessageIDs(context.Context, pgtype.UUID, pgtype.UUID) (map[pgtype.UUID]CompactionRecord, error) {
	return map[pgtype.UUID]CompactionRecord{}, nil
}

func (s *memoryStore) GetBySemanticKey(_ context.Context, _ pgtype.UUID, semanticKey string) (CompactionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[semanticKey]
	if !ok {
		return CompactionRecord{}, ErrCompactionNotFound
	}
	return record, nil
}

func (s *memoryStore) Search(_ context.Context, input SearchInput) ([]CompactionRecord, error) {
	if input.Query == "error" {
		return nil, errors.New("search error")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []CompactionRecord
	for _, record := range s.records {
		if record.SemanticKey == input.Query {
			out = append(out, record)
		}
	}
	return out, nil
}

type memoryDetailFileWriter struct {
	files map[string]string
}

func newMemoryDetailFileWriter() *memoryDetailFileWriter {
	return &memoryDetailFileWriter{files: map[string]string{}}
}

func (w *memoryDetailFileWriter) WriteDetailFile(_ context.Context, input DetailFileInput) (DetailFileResult, error) {
	result := detailFileResult(input)
	w.files[result.Path] = detailFileContent(input, result.Hash)
	return result, nil
}
