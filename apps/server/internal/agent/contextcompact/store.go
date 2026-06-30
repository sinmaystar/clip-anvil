package contextcompact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var ErrCompactionNotFound = errors.New("agent context compaction not found")

type Store interface {
	CreateCompaction(ctx context.Context, input CreateCompactionInput) (CompactionRecord, error)
	LinkMessage(ctx context.Context, input LinkMessageInput) error
	CompactedMessageIDs(ctx context.Context, workspaceID pgtype.UUID, threadID pgtype.UUID) (map[pgtype.UUID]CompactionRecord, error)
	GetBySemanticKey(ctx context.Context, workspaceID pgtype.UUID, semanticKey string) (CompactionRecord, error)
	Search(ctx context.Context, input SearchInput) ([]CompactionRecord, error)
}

type CreateCompactionInput struct {
	WorkspaceID            pgtype.UUID
	ThreadID               pgtype.UUID
	TaskID                 pgtype.UUID
	Role                   string
	Mode                   string
	Trigger                string
	SemanticKey            string
	SourceSeqStart         int64
	SourceSeqEnd           int64
	SourceMessageIDs       []string
	SourceMediaRefs        []string
	OriginalTokenEstimate  int64
	CompactedTokenEstimate int64
	OriginalBytes          int64
	Summary                string
	DetailFiles            []string
	Payload                map[string]any
}

type LinkMessageInput struct {
	MessageID     pgtype.UUID
	CompactionID  pgtype.UUID
	CompactedRole string
}

type SearchInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	Query       string
	Limit       int32
}

type CompactionRecord struct {
	ID                     pgtype.UUID
	WorkspaceID            pgtype.UUID
	ThreadID               pgtype.UUID
	TaskID                 pgtype.UUID
	Role                   string
	Mode                   string
	Trigger                string
	SemanticKey            string
	SourceSeqStart         int64
	SourceSeqEnd           int64
	SourceMessageIDs       []string
	SourceMediaRefs        []string
	OriginalTokenEstimate  int64
	CompactedTokenEstimate int64
	OriginalBytes          int64
	Summary                string
	DetailFiles            []string
	Payload                map[string]any
	CreatedAt              pgtype.Timestamptz
}

type SQLStore struct {
	queries *db.Queries
}

func NewSQLStore(queries *db.Queries) *SQLStore {
	return &SQLStore{queries: queries}
}

func (s *SQLStore) CreateCompaction(ctx context.Context, input CreateCompactionInput) (CompactionRecord, error) {
	if s == nil || s.queries == nil {
		return CompactionRecord{}, errors.New("context compaction store is not configured")
	}
	sourceMessageIDs, err := json.Marshal(input.SourceMessageIDs)
	if err != nil {
		return CompactionRecord{}, fmt.Errorf("marshal source message ids: %w", err)
	}
	sourceMediaRefs, err := json.Marshal(input.SourceMediaRefs)
	if err != nil {
		return CompactionRecord{}, fmt.Errorf("marshal source media refs: %w", err)
	}
	detailFiles, err := json.Marshal(input.DetailFiles)
	if err != nil {
		return CompactionRecord{}, fmt.Errorf("marshal detail files: %w", err)
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return CompactionRecord{}, fmt.Errorf("marshal payload: %w", err)
	}
	row, err := s.queries.CreateAgentContextCompaction(ctx, db.CreateAgentContextCompactionParams{
		WorkspaceID:            input.WorkspaceID,
		ThreadID:               input.ThreadID,
		TaskID:                 input.TaskID,
		Role:                   input.Role,
		Mode:                   input.Mode,
		Trigger:                input.Trigger,
		SemanticKey:            input.SemanticKey,
		SourceSeqStart:         input.SourceSeqStart,
		SourceSeqEnd:           input.SourceSeqEnd,
		SourceMessageIds:       sourceMessageIDs,
		SourceMediaRefs:        sourceMediaRefs,
		OriginalTokenEstimate:  input.OriginalTokenEstimate,
		CompactedTokenEstimate: input.CompactedTokenEstimate,
		OriginalBytes:          input.OriginalBytes,
		Summary:                input.Summary,
		DetailFiles:            detailFiles,
		Payload:                payload,
	})
	if err != nil {
		return CompactionRecord{}, err
	}
	return compactionRecordFromDB(row), nil
}

func (s *SQLStore) LinkMessage(ctx context.Context, input LinkMessageInput) error {
	if s == nil || s.queries == nil {
		return errors.New("context compaction store is not configured")
	}
	_, err := s.queries.LinkAgentMessageCompaction(ctx, db.LinkAgentMessageCompactionParams{
		MessageID:     input.MessageID,
		CompactionID:  input.CompactionID,
		CompactedRole: input.CompactedRole,
	})
	return err
}

func (s *SQLStore) CompactedMessageIDs(ctx context.Context, workspaceID pgtype.UUID, threadID pgtype.UUID) (map[pgtype.UUID]CompactionRecord, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("context compaction store is not configured")
	}
	rows, err := s.queries.ListCompactedMessageIDsByThread(ctx, db.ListCompactedMessageIDsByThreadParams{WorkspaceID: workspaceID, ThreadID: threadID})
	if err != nil {
		return nil, err
	}
	out := make(map[pgtype.UUID]CompactionRecord, len(rows))
	for _, row := range rows {
		out[row.MessageID] = CompactionRecord{
			ID:           row.CompactionID,
			SemanticKey:  row.SemanticKey,
			Summary:      row.Summary,
			DetailFiles:  decodeStringSlice(row.DetailFiles),
			ThreadID:     threadID,
			WorkspaceID:  workspaceID,
			CreatedAt:    row.CompactedAt,
			SourceSeqEnd: 0,
		}
	}
	return out, nil
}

func (s *SQLStore) GetBySemanticKey(ctx context.Context, workspaceID pgtype.UUID, semanticKey string) (CompactionRecord, error) {
	if s == nil || s.queries == nil {
		return CompactionRecord{}, errors.New("context compaction store is not configured")
	}
	row, err := s.queries.GetAgentContextCompactionBySemanticKey(ctx, db.GetAgentContextCompactionBySemanticKeyParams{
		WorkspaceID: workspaceID,
		SemanticKey: semanticKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return CompactionRecord{}, ErrCompactionNotFound
	}
	if err != nil {
		return CompactionRecord{}, err
	}
	return compactionRecordFromDB(row), nil
}

func (s *SQLStore) Search(ctx context.Context, input SearchInput) ([]CompactionRecord, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("context compaction store is not configured")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.queries.SearchAgentContextCompactions(ctx, db.SearchAgentContextCompactionsParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Query:       input.Query,
		RowLimit:    limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]CompactionRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, compactionRecordFromDB(row))
	}
	return out, nil
}

func compactionRecordFromDB(row db.AgentContextCompaction) CompactionRecord {
	return CompactionRecord{
		ID:                     row.ID,
		WorkspaceID:            row.WorkspaceID,
		ThreadID:               row.ThreadID,
		TaskID:                 row.TaskID,
		Role:                   row.Role,
		Mode:                   row.Mode,
		Trigger:                row.Trigger,
		SemanticKey:            row.SemanticKey,
		SourceSeqStart:         row.SourceSeqStart,
		SourceSeqEnd:           row.SourceSeqEnd,
		SourceMessageIDs:       decodeStringSlice(row.SourceMessageIds),
		SourceMediaRefs:        decodeStringSlice(row.SourceMediaRefs),
		OriginalTokenEstimate:  row.OriginalTokenEstimate,
		CompactedTokenEstimate: row.CompactedTokenEstimate,
		OriginalBytes:          row.OriginalBytes,
		Summary:                row.Summary,
		DetailFiles:            decodeStringSlice(row.DetailFiles),
		Payload:                decodeMap(row.Payload),
		CreatedAt:              row.CreatedAt,
	}
}

func decodeStringSlice(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func decodeMap(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
