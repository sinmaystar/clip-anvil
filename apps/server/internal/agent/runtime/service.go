package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidConfig  = errors.New("invalid agent runtime config")
	ErrInvalidRequest = errors.New("invalid agent runtime request")
)

type txBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Service struct {
	pool    txBeginner
	queries *db.Queries
}

func NewService(pool txBeginner, queries *db.Queries) (*Service, error) {
	if pool == nil {
		return nil, ErrInvalidConfig
	}
	if queries == nil {
		return nil, ErrInvalidConfig
	}
	return &Service{pool: pool, queries: queries}, nil
}

type CreateThreadParams struct {
	WorkspaceID      pgtype.UUID
	Role             string
	ScopeType        string
	ScopeID          pgtype.UUID
	RuntimeProvider  string
	RuntimeAgentName string
	Summary          string
}

func (s *Service) GetOrCreateProducerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error) {
	if !workspaceID.Valid {
		return db.AgentThread{}, ErrInvalidRequest
	}

	thread, err := s.queries.GetActiveProducerThreadByWorkspace(ctx, workspaceID)
	if err == nil {
		return thread, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.AgentThread{}, err
	}

	thread, err = s.CreateThread(ctx, CreateThreadParams{
		WorkspaceID:     workspaceID,
		Role:            "producer",
		ScopeType:       "workspace",
		RuntimeProvider: "eino",
	})
	if err == nil {
		return thread, nil
	}

	existing, getErr := s.queries.GetActiveProducerThreadByWorkspace(ctx, workspaceID)
	if getErr == nil {
		return existing, nil
	}
	return db.AgentThread{}, err
}

func (s *Service) CreateThread(ctx context.Context, params CreateThreadParams) (db.AgentThread, error) {
	if !params.WorkspaceID.Valid || !validThreadRole(params.Role) {
		return db.AgentThread{}, ErrInvalidRequest
	}
	scopeType := defaultString(params.ScopeType, "workspace")
	if !validThreadScope(scopeType) {
		return db.AgentThread{}, ErrInvalidRequest
	}
	runtimeProvider := defaultString(params.RuntimeProvider, "eino")

	return s.queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		WorkspaceID:      params.WorkspaceID,
		Role:             params.Role,
		ScopeType:        scopeType,
		ScopeID:          params.ScopeID,
		RuntimeProvider:  runtimeProvider,
		RuntimeAgentName: params.RuntimeAgentName,
		Summary:          params.Summary,
	})
}

func (s *Service) UpdateThreadStatus(ctx context.Context, threadID pgtype.UUID, status string) (db.AgentThread, error) {
	if !threadID.Valid || !validThreadStatus(status) {
		return db.AgentThread{}, ErrInvalidRequest
	}
	return s.queries.UpdateAgentThreadStatus(ctx, db.UpdateAgentThreadStatusParams{
		ID:     threadID,
		Status: status,
	})
}

func (s *Service) SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error) {
	if !threadID.Valid {
		return db.AgentThread{}, ErrInvalidRequest
	}
	return s.queries.SetAgentThreadCheckpoint(ctx, db.SetAgentThreadCheckpointParams{
		ID: threadID,
		CurrentCheckpointKey: pgtype.Text{
			String: checkpointKey,
			Valid:  checkpointKey != "",
		},
	})
}

type AppendMessageParams struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	Role        string
	MessageType string
	Content     []byte
	RawMessage  []byte
	TaskID      pgtype.UUID
	EventID     pgtype.UUID
}

func (s *Service) AppendMessage(ctx context.Context, params AppendMessageParams) (db.AgentMessage, error) {
	if !params.WorkspaceID.Valid || !params.ThreadID.Valid || !validMessageRole(params.Role) {
		return db.AgentMessage{}, ErrInvalidRequest
	}
	messageType := defaultString(params.MessageType, "text")
	if !validMessageType(messageType) {
		return db.AgentMessage{}, ErrInvalidRequest
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.AgentMessage{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := s.queries.WithTx(tx)
	seq, err := qtx.NextAgentMessageSeq(ctx, params.ThreadID)
	if err != nil {
		return db.AgentMessage{}, err
	}
	msg, err := qtx.CreateAgentMessage(ctx, db.CreateAgentMessageParams{
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Seq:         int64(seq),
		Role:        params.Role,
		MessageType: messageType,
		Content:     defaultJSON(params.Content),
		RawMessage:  defaultJSON(params.RawMessage),
		TaskID:      params.TaskID,
		EventID:     params.EventID,
	})
	if err != nil {
		return db.AgentMessage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.AgentMessage{}, err
	}
	committed = true
	return msg, nil
}

func (s *Service) ListMessages(ctx context.Context, threadID pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error) {
	if !threadID.Valid {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.queries.ListAgentMessagesByThread(ctx, db.ListAgentMessagesByThreadParams{
		ThreadID: threadID,
		Seq:      afterSeq,
		Limit:    limit,
	})
}

type CreateTaskParams struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	Role        string
	ScopeType   string
	ScopeID     pgtype.UUID
	TaskType    string
	MaxAttempts int32
	Input       []byte
}

func (s *Service) CreateTask(ctx context.Context, params CreateTaskParams) (db.AgentTask, error) {
	if !params.WorkspaceID.Valid || !validTaskRole(params.Role) || !validTaskType(params.TaskType) || params.MaxAttempts < 1 {
		return db.AgentTask{}, ErrInvalidRequest
	}
	scopeType := defaultString(params.ScopeType, "workspace")
	if !validTaskScope(scopeType) {
		return db.AgentTask{}, ErrInvalidRequest
	}
	return s.queries.CreateAgentTask(ctx, db.CreateAgentTaskParams{
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		ScopeType:   scopeType,
		ScopeID:     params.ScopeID,
		TaskType:    params.TaskType,
		MaxAttempts: params.MaxAttempts,
		Input:       defaultJSON(params.Input),
	})
}

func (s *Service) ListQueuedProducerTasks(ctx context.Context, workspaceID pgtype.UUID, limit int32) ([]db.AgentTask, error) {
	if !workspaceID.Valid {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.queries.ListQueuedProducerTasks(ctx, db.ListQueuedProducerTasksParams{
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
}

func (s *Service) ListQueuedProducerTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.queries.ListQueuedProducerTasksAcrossWorkspaces(ctx, limit)
}

func (s *Service) ListActiveAgentTasksByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error) {
	if !workspaceID.Valid {
		return nil, ErrInvalidRequest
	}
	return s.queries.ListActiveAgentTasksByWorkspace(ctx, workspaceID)
}

func (s *Service) MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error) {
	if !taskID.Valid {
		return db.AgentTask{}, ErrInvalidRequest
	}
	return s.queries.MarkAgentTaskRunning(ctx, taskID)
}

func (s *Service) MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error) {
	if !taskID.Valid {
		return db.AgentTask{}, ErrInvalidRequest
	}
	return s.queries.MarkAgentTaskSucceeded(ctx, db.MarkAgentTaskSucceededParams{
		ID:     taskID,
		Output: defaultJSON(output),
	})
}

func (s *Service) MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error) {
	if !taskID.Valid {
		return db.AgentTask{}, ErrInvalidRequest
	}
	return s.queries.MarkAgentTaskFailed(ctx, db.MarkAgentTaskFailedParams{
		ID:           taskID,
		ErrorCode:    nullableText(code),
		ErrorMessage: nullableText(message),
	})
}

func (s *Service) MarkTaskCancelled(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error) {
	if !taskID.Valid {
		return db.AgentTask{}, ErrInvalidRequest
	}
	return s.queries.MarkAgentTaskCancelled(ctx, taskID)
}

func (s *Service) MarkTaskWaitingForUser(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error) {
	if !taskID.Valid {
		return db.AgentTask{}, ErrInvalidRequest
	}
	return s.queries.MarkAgentTaskWaitingForUser(ctx, taskID)
}

type CreateEventParams struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	EventType   string
	SourceRole  string
	TargetRole  string
	Scope       []byte
	Payload     []byte
}

func (s *Service) CreateEvent(ctx context.Context, params CreateEventParams) (db.AgentEvent, error) {
	if !params.WorkspaceID.Valid || params.EventType == "" {
		return db.AgentEvent{}, ErrInvalidRequest
	}
	sourceRole := defaultString(params.SourceRole, "system")
	if !validEventSourceRole(sourceRole) {
		return db.AgentEvent{}, ErrInvalidRequest
	}
	return s.queries.CreateAgentEvent(ctx, db.CreateAgentEventParams{
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		TaskID:      params.TaskID,
		EventType:   params.EventType,
		SourceRole:  sourceRole,
		TargetRole:  nullableText(params.TargetRole),
		Scope:       defaultJSON(params.Scope),
		Payload:     defaultJSON(params.Payload),
	})
}

func (s *Service) MarkEventHandled(ctx context.Context, eventID pgtype.UUID) (db.AgentEvent, error) {
	if !eventID.Valid {
		return db.AgentEvent{}, ErrInvalidRequest
	}
	return s.queries.MarkAgentEventHandled(ctx, eventID)
}

func (s *Service) GetAgentEventForWorkspace(ctx context.Context, eventID, workspaceID pgtype.UUID) (db.AgentEvent, error) {
	if !eventID.Valid || !workspaceID.Valid {
		return db.AgentEvent{}, ErrInvalidRequest
	}
	return s.queries.GetAgentEventForWorkspace(ctx, db.GetAgentEventForWorkspaceParams{
		ID:          eventID,
		WorkspaceID: workspaceID,
	})
}

type UpsertCheckpointParams struct {
	Key         string
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	Value       []byte
	Metadata    []byte
}

func (s *Service) UpsertCheckpoint(ctx context.Context, params UpsertCheckpointParams) (db.EinoCheckpoint, error) {
	if !params.WorkspaceID.Valid || len(params.Value) == 0 {
		return db.EinoCheckpoint{}, ErrInvalidRequest
	}
	key := params.Key
	if key == "" {
		key = CheckpointKey(params.WorkspaceID, params.ThreadID, params.TaskID)
	}
	return s.queries.UpsertEinoCheckpoint(ctx, db.UpsertEinoCheckpointParams{
		Key:         key,
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		TaskID:      params.TaskID,
		Value:       params.Value,
		Metadata:    defaultJSON(params.Metadata),
	})
}

func (s *Service) GetCheckpoint(ctx context.Context, key string) (db.EinoCheckpoint, error) {
	if key == "" {
		return db.EinoCheckpoint{}, ErrInvalidRequest
	}
	return s.queries.GetEinoCheckpoint(ctx, key)
}

func (s *Service) DeleteCheckpoint(ctx context.Context, key string) error {
	if key == "" {
		return ErrInvalidRequest
	}
	return s.queries.DeleteEinoCheckpoint(ctx, key)
}

func CheckpointKey(workspaceID, threadID, taskID pgtype.UUID) string {
	return fmt.Sprintf("agent:%s:%s:%s", uuidString(workspaceID), uuidString(threadID), uuidString(taskID))
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return "00000000-0000-0000-0000-000000000000"
	}
	return uuid.UUID(id.Bytes).String()
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultJSON(value []byte) []byte {
	if len(value) == 0 {
		return []byte("{}")
	}
	return value
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func validThreadRole(value string) bool {
	switch value {
	case "producer", "craftsman", "reviewer", "composer":
		return true
	default:
		return false
	}
}

func validThreadScope(value string) bool {
	switch value {
	case "workspace", "shot", "final_output":
		return true
	default:
		return false
	}
}

func validThreadStatus(value string) bool {
	switch value {
	case "active", "paused", "archived", "failed":
		return true
	default:
		return false
	}
}

func validMessageRole(value string) bool {
	switch value {
	case "user", "assistant", "tool", "system":
		return true
	default:
		return false
	}
}

func validMessageType(value string) bool {
	switch value {
	case "text", "tool_call", "tool_result", "ui_card", "error", "status":
		return true
	default:
		return false
	}
}

func validTaskRole(value string) bool {
	switch value {
	case "producer", "craftsman", "reviewer", "composer", "worker", "system":
		return true
	default:
		return false
	}
}

func validTaskScope(value string) bool {
	switch value {
	case "workspace", "shot", "node", "job", "final_output":
		return true
	default:
		return false
	}
}

func validTaskType(value string) bool {
	switch value {
	case "producer_turn", "tool_call", "decision_resume":
		return true
	default:
		return false
	}
}

func validEventSourceRole(value string) bool {
	switch value {
	case "user", "producer", "craftsman", "reviewer", "composer", "worker", "system":
		return true
	default:
		return false
	}
}
