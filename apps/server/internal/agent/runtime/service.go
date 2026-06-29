package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

func (s *Service) GetOrCreateComposerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error) {
	if !workspaceID.Valid {
		return db.AgentThread{}, ErrInvalidRequest
	}

	thread, err := s.queries.GetActiveComposerThreadByWorkspace(ctx, workspaceID)
	if err == nil {
		return thread, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.AgentThread{}, err
	}

	thread, err = s.CreateThread(ctx, CreateThreadParams{
		WorkspaceID:      workspaceID,
		Role:             "composer",
		ScopeType:        "final_output",
		RuntimeProvider:  "eino",
		RuntimeAgentName: "ComposerGraph",
	})
	if err == nil {
		return thread, nil
	}

	existing, getErr := s.queries.GetActiveComposerThreadByWorkspace(ctx, workspaceID)
	if getErr == nil {
		return existing, nil
	}
	return db.AgentThread{}, err
}

func (s *Service) GetOrCreateCraftsmanThread(ctx context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error) {
	return s.GetOrCreateCraftsmanThreadForScope(ctx, workspaceID, "shot", shotID)
}

func (s *Service) GetOrCreateCraftsmanThreadForScope(ctx context.Context, workspaceID pgtype.UUID, scopeType string, scopeID pgtype.UUID) (db.AgentThread, error) {
	if !workspaceID.Valid || !scopeID.Valid || scopeType == "" {
		return db.AgentThread{}, ErrInvalidRequest
	}

	params := db.GetActiveAgentThreadByScopeParams{
		WorkspaceID: workspaceID,
		Role:        "craftsman",
		ScopeType:   scopeType,
		ScopeID:     scopeID,
	}
	thread, err := s.queries.GetActiveAgentThreadByScope(ctx, params)
	if err == nil {
		return thread, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.AgentThread{}, err
	}

	thread, err = s.CreateThread(ctx, CreateThreadParams{
		WorkspaceID:      workspaceID,
		Role:             "craftsman",
		ScopeType:        scopeType,
		ScopeID:          scopeID,
		RuntimeProvider:  "eino",
		RuntimeAgentName: "CraftsmanGraph",
	})
	if err == nil {
		return thread, nil
	}

	existing, getErr := s.queries.GetActiveAgentThreadByScope(ctx, params)
	if getErr == nil {
		return existing, nil
	}
	return db.AgentThread{}, err
}

func (s *Service) GetOrCreateReviewerThread(ctx context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error) {
	return s.GetOrCreateReviewerThreadForScope(ctx, workspaceID, "shot", shotID)
}

func (s *Service) UpdateShotStatus(ctx context.Context, params db.UpdateShotStatusParams) (db.Shot, error) {
	if s == nil || s.queries == nil {
		return db.Shot{}, ErrInvalidConfig
	}
	return s.queries.UpdateShotStatus(ctx, params)
}

func (s *Service) GetLatestRenderPlanByTaskScopePhase(ctx context.Context, params db.GetLatestRenderPlanByTaskScopePhaseParams) (db.RenderPlan, error) {
	if s == nil || s.queries == nil {
		return db.RenderPlan{}, ErrInvalidConfig
	}
	if !params.WorkspaceID.Valid || !params.ScopeID.Valid || !params.CreatedByTaskID.Valid || strings.TrimSpace(params.ScopeType) == "" || strings.TrimSpace(params.TargetPhase) == "" {
		return db.RenderPlan{}, ErrInvalidRequest
	}
	return s.queries.GetLatestRenderPlanByTaskScopePhase(ctx, params)
}

func (s *Service) GetOrCreateReviewerThreadForScope(ctx context.Context, workspaceID pgtype.UUID, scopeType string, scopeID pgtype.UUID) (db.AgentThread, error) {
	if !workspaceID.Valid {
		return db.AgentThread{}, ErrInvalidRequest
	}
	scopeType = defaultString(scopeType, "shot")
	if !validThreadScope(scopeType) || !scopeID.Valid {
		return db.AgentThread{}, ErrInvalidRequest
	}

	params := db.GetActiveAgentThreadByScopeParams{
		WorkspaceID: workspaceID,
		Role:        "reviewer",
		ScopeType:   scopeType,
		ScopeID:     scopeID,
	}
	thread, err := s.queries.GetActiveAgentThreadByScope(ctx, params)
	if err == nil {
		return thread, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.AgentThread{}, err
	}

	thread, err = s.CreateThread(ctx, CreateThreadParams{
		WorkspaceID:      workspaceID,
		Role:             "reviewer",
		ScopeType:        scopeType,
		ScopeID:          scopeID,
		RuntimeProvider:  "eino",
		RuntimeAgentName: "ReviewerGate",
	})
	if err == nil {
		return thread, nil
	}

	existing, getErr := s.queries.GetActiveAgentThreadByScope(ctx, params)
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

type UpdateMessageParams struct {
	ID         pgtype.UUID
	Content    []byte
	RawMessage []byte
	EventID    pgtype.UUID
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

func (s *Service) UpdateMessage(ctx context.Context, params UpdateMessageParams) (db.AgentMessage, error) {
	if !params.ID.Valid {
		return db.AgentMessage{}, ErrInvalidRequest
	}
	return s.queries.UpdateAgentMessage(ctx, db.UpdateAgentMessageParams{
		ID:         params.ID,
		Content:    defaultJSON(params.Content),
		RawMessage: defaultJSON(params.RawMessage),
		EventID:    params.EventID,
	})
}

func (s *Service) ListMessages(ctx context.Context, threadID pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error) {
	if !threadID.Valid {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListAgentMessagesByThread(ctx, db.ListAgentMessagesByThreadParams{
		ThreadID: threadID,
		Seq:      afterSeq,
		Limit:    limit,
	})
}

func (s *Service) ListWorkspaceMessages(ctx context.Context, workspaceID pgtype.UUID, afterCreatedAt pgtype.Timestamptz, limit int32) ([]db.AgentMessage, error) {
	if !workspaceID.Valid {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListAgentMessagesByWorkspace(ctx, db.ListAgentMessagesByWorkspaceParams{
		WorkspaceID:    workspaceID,
		AfterCreatedAt: afterCreatedAt,
		RowLimit:       limit,
	})
}

func (s *Service) ListAgentThreadsByWorkspace(ctx context.Context, workspaceID pgtype.UUID, includeProducer bool) ([]db.AgentThread, error) {
	if !workspaceID.Valid {
		return nil, ErrInvalidRequest
	}
	if s == nil || s.queries == nil {
		return nil, ErrInvalidConfig
	}
	return s.queries.ListObservableAgentThreadsByWorkspace(ctx, db.ListObservableAgentThreadsByWorkspaceParams{
		WorkspaceID:     workspaceID,
		IncludeProducer: includeProducer,
	})
}

func (s *Service) GetThreadForWorkspace(ctx context.Context, threadID pgtype.UUID, workspaceID pgtype.UUID) (db.AgentThread, error) {
	if !threadID.Valid || !workspaceID.Valid {
		return db.AgentThread{}, ErrInvalidRequest
	}
	if s == nil || s.queries == nil {
		return db.AgentThread{}, ErrInvalidConfig
	}
	return s.queries.GetAgentThreadForWorkspace(ctx, db.GetAgentThreadForWorkspaceParams{
		ID:          threadID,
		WorkspaceID: workspaceID,
	})
}

func (s *Service) ListThreadMessages(ctx context.Context, threadID pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error) {
	return s.ListMessages(ctx, threadID, afterSeq, limit)
}

func (s *Service) LatestTaskByThread(ctx context.Context, threadID pgtype.UUID) (db.AgentTask, error) {
	if !threadID.Valid {
		return db.AgentTask{}, ErrInvalidRequest
	}
	if s == nil || s.queries == nil {
		return db.AgentTask{}, ErrInvalidConfig
	}
	return s.queries.GetLatestAgentTaskByThread(ctx, threadID)
}

func (s *Service) LatestMessageByThread(ctx context.Context, threadID pgtype.UUID) (db.AgentMessage, error) {
	if !threadID.Valid {
		return db.AgentMessage{}, ErrInvalidRequest
	}
	if s == nil || s.queries == nil {
		return db.AgentMessage{}, ErrInvalidConfig
	}
	return s.queries.GetLatestAgentMessageByThread(ctx, threadID)
}

type CreateTaskParams struct {
	WorkspaceID  pgtype.UUID
	ThreadID     pgtype.UUID
	Role         string
	ScopeType    string
	ScopeID      pgtype.UUID
	TaskType     string
	MaxAttempts  int32
	Input        []byte
	RenderPlanID pgtype.UUID
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
		WorkspaceID:  params.WorkspaceID,
		ThreadID:     params.ThreadID,
		Role:         params.Role,
		ScopeType:    scopeType,
		ScopeID:      params.ScopeID,
		TaskType:     params.TaskType,
		MaxAttempts:  params.MaxAttempts,
		Input:        defaultJSON(params.Input),
		RenderPlanID: params.RenderPlanID,
	})
}

func (s *Service) ListQueuedProducerTasks(ctx context.Context, workspaceID pgtype.UUID, limit int32) ([]db.AgentTask, error) {
	if !workspaceID.Valid {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListQueuedProducerTasks(ctx, db.ListQueuedProducerTasksParams{
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
}

func (s *Service) ListQueuedProducerTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListQueuedProducerTasksAcrossWorkspaces(ctx, limit)
}

func (s *Service) ListQueuedCraftsmanTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListQueuedCraftsmanTasksAcrossWorkspaces(ctx, limit)
}

func (s *Service) ListQueuedWorkerTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListQueuedWorkerTasksAcrossWorkspaces(ctx, limit)
}

func (s *Service) ListQueuedReviewerTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListQueuedReviewerTasksAcrossWorkspaces(ctx, limit)
}

func (s *Service) ListQueuedComposerTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListQueuedComposerTasksAcrossWorkspaces(ctx, limit)
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

func (s *Service) ListAgentEventsByWorkspace(ctx context.Context, workspaceID pgtype.UUID, limit int32) ([]db.AgentEvent, error) {
	if !workspaceID.Valid {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 {
		limit = 50
	}
	return s.queries.ListAgentEventsByWorkspace(ctx, db.ListAgentEventsByWorkspaceParams{
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
}

type CreateProducerPendingSignalParams struct {
	WorkspaceID      pgtype.UUID
	ProducerThreadID pgtype.UUID
	SourceRole       string
	SourceTaskID     pgtype.UUID
	SourceThreadID   pgtype.UUID
	SignalType       string
	ScopeType        string
	ScopeID          pgtype.UUID
	RenderPlanID     pgtype.UUID
	MessageID        pgtype.UUID
	Priority         int32
	DedupeKey        string
	Payload          []byte
}

func (s *Service) CreateProducerPendingSignal(ctx context.Context, params CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error) {
	if s == nil || s.queries == nil {
		return db.ProducerPendingSignal{}, ErrInvalidConfig
	}
	if !params.WorkspaceID.Valid || !params.ProducerThreadID.Valid || !validProducerSignalSourceRole(params.SourceRole) || strings.TrimSpace(params.SignalType) == "" || !validProducerSignalScope(params.ScopeType) || strings.TrimSpace(params.DedupeKey) == "" {
		return db.ProducerPendingSignal{}, ErrInvalidRequest
	}
	if params.SignalType == "craftsman_render_plan_ready" && !params.RenderPlanID.Valid {
		return db.ProducerPendingSignal{}, ErrInvalidRequest
	}
	if params.Priority == 0 {
		params.Priority = 100
	}
	return s.queries.CreateProducerPendingSignal(ctx, db.CreateProducerPendingSignalParams{
		WorkspaceID:      params.WorkspaceID,
		ProducerThreadID: params.ProducerThreadID,
		SourceRole:       params.SourceRole,
		SourceTaskID:     params.SourceTaskID,
		SourceThreadID:   params.SourceThreadID,
		SignalType:       params.SignalType,
		ScopeType:        params.ScopeType,
		ScopeID:          params.ScopeID,
		RenderPlanID:     params.RenderPlanID,
		MessageID:        params.MessageID,
		Priority:         params.Priority,
		DedupeKey:        params.DedupeKey,
		Payload:          defaultJSON(params.Payload),
	})
}

type ClaimProducerPendingSignalsParams struct {
	WorkspaceID       pgtype.UUID
	ProducerThreadID  pgtype.UUID
	ClaimedByTaskID   pgtype.UUID
	Limit             int32
	StaleAfterSeconds int32
}

func (s *Service) ClaimProducerPendingSignals(ctx context.Context, params ClaimProducerPendingSignalsParams) ([]db.ProducerPendingSignal, error) {
	if s == nil || s.queries == nil {
		return nil, ErrInvalidConfig
	}
	if !params.WorkspaceID.Valid || !params.ProducerThreadID.Valid || !params.ClaimedByTaskID.Valid {
		return nil, ErrInvalidRequest
	}
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	if params.StaleAfterSeconds <= 0 {
		params.StaleAfterSeconds = 600
	}
	return s.queries.ClaimProducerPendingSignals(ctx, db.ClaimProducerPendingSignalsParams{
		WorkspaceID:       params.WorkspaceID,
		ProducerThreadID:  params.ProducerThreadID,
		ClaimedByTaskID:   params.ClaimedByTaskID,
		StaleAfterSeconds: params.StaleAfterSeconds,
		RowLimit:          params.Limit,
	})
}

func (s *Service) ListClaimedProducerSignalsByTask(ctx context.Context, workspaceID, producerThreadID, taskID pgtype.UUID) ([]db.ProducerPendingSignal, error) {
	if s == nil || s.queries == nil {
		return nil, ErrInvalidConfig
	}
	if !workspaceID.Valid || !producerThreadID.Valid || !taskID.Valid {
		return nil, ErrInvalidRequest
	}
	return s.queries.ListClaimedProducerSignalsByTask(ctx, db.ListClaimedProducerSignalsByTaskParams{
		WorkspaceID:      workspaceID,
		ProducerThreadID: producerThreadID,
		ClaimedByTaskID:  taskID,
	})
}

func (s *Service) MarkProducerPendingSignalProcessed(ctx context.Context, signalID, workspaceID, taskID pgtype.UUID) (db.ProducerPendingSignal, error) {
	if s == nil || s.queries == nil {
		return db.ProducerPendingSignal{}, ErrInvalidConfig
	}
	if !signalID.Valid || !workspaceID.Valid || !taskID.Valid {
		return db.ProducerPendingSignal{}, ErrInvalidRequest
	}
	return s.queries.MarkProducerPendingSignalProcessed(ctx, db.MarkProducerPendingSignalProcessedParams{
		ID:                signalID,
		WorkspaceID:       workspaceID,
		ProcessedByTaskID: taskID,
	})
}

func (s *Service) MarkProducerPendingSignalsProcessedByRenderPlan(ctx context.Context, workspaceID, renderPlanID, taskID pgtype.UUID) ([]db.ProducerPendingSignal, error) {
	if s == nil || s.queries == nil {
		return nil, ErrInvalidConfig
	}
	if !workspaceID.Valid || !renderPlanID.Valid || !taskID.Valid {
		return nil, ErrInvalidRequest
	}
	return s.queries.MarkProducerPendingSignalsProcessedByRenderPlan(ctx, db.MarkProducerPendingSignalsProcessedByRenderPlanParams{
		WorkspaceID:       workspaceID,
		RenderPlanID:      renderPlanID,
		ProcessedByTaskID: taskID,
	})
}

func (s *Service) MarkProducerPendingSignalIgnored(ctx context.Context, signalID, workspaceID, taskID pgtype.UUID, reason string) (db.ProducerPendingSignal, error) {
	if s == nil || s.queries == nil {
		return db.ProducerPendingSignal{}, ErrInvalidConfig
	}
	if !signalID.Valid || !workspaceID.Valid || !taskID.Valid || strings.TrimSpace(reason) == "" {
		return db.ProducerPendingSignal{}, ErrInvalidRequest
	}
	return s.queries.MarkProducerPendingSignalIgnored(ctx, db.MarkProducerPendingSignalIgnoredParams{
		ID:                signalID,
		WorkspaceID:       workspaceID,
		ProcessedByTaskID: taskID,
		LastError:         nullableText(reason),
	})
}

func (s *Service) ReleaseProducerPendingSignalsForTask(ctx context.Context, workspaceID, taskID pgtype.UUID, reason string) ([]db.ProducerPendingSignal, error) {
	if s == nil || s.queries == nil {
		return nil, ErrInvalidConfig
	}
	if !workspaceID.Valid || !taskID.Valid {
		return nil, ErrInvalidRequest
	}
	return s.queries.ReleaseProducerPendingSignalsForTask(ctx, db.ReleaseProducerPendingSignalsForTaskParams{
		WorkspaceID:     workspaceID,
		ClaimedByTaskID: taskID,
		LastError:       nullableText(reason),
	})
}

func (s *Service) ListPendingProducerSignals(ctx context.Context, workspaceID pgtype.UUID, limit int32) ([]db.ProducerPendingSignal, error) {
	if s == nil || s.queries == nil {
		return nil, ErrInvalidConfig
	}
	if !workspaceID.Valid {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListPendingProducerSignals(ctx, db.ListPendingProducerSignalsParams{
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
}

func (s *Service) ListPendingProducerSignalsByThread(ctx context.Context, workspaceID, producerThreadID pgtype.UUID, limit int32) ([]db.ProducerPendingSignal, error) {
	if s == nil || s.queries == nil {
		return nil, ErrInvalidConfig
	}
	if !workspaceID.Valid || !producerThreadID.Valid {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.queries.ListPendingProducerSignalsByThread(ctx, db.ListPendingProducerSignalsByThreadParams{
		WorkspaceID:      workspaceID,
		ProducerThreadID: producerThreadID,
		Limit:            limit,
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
	case "workspace", "shot", "final_output", "render_plan", "key_element_state", "audio_plan":
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
	case "workspace", "shot", "node", "job", "final_output", "render_plan", "key_element_state", "audio_plan":
		return true
	default:
		return false
	}
}

func validTaskType(value string) bool {
	switch value {
	case "producer_turn", "tool_call", "decision_resume", "craftsman_turn", "worker_generation", "reviewer_turn", "dependency_scheduler", "composer_turn":
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

func validProducerSignalSourceRole(value string) bool {
	switch value {
	case "craftsman", "worker", "reviewer", "composer", "system":
		return true
	default:
		return false
	}
}

func validProducerSignalScope(value string) bool {
	switch value {
	case "workspace", "shot", "render_plan", "final_output", "key_element_state", "audio_plan", "node", "job":
		return true
	default:
		return false
	}
}
