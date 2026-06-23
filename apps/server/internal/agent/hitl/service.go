package hitl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var ErrInvalidDecisionResponse = errors.New("invalid decision response")

type Runtime interface {
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	UpsertCheckpoint(ctx context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
	MarkTaskWaitingForUser(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	GetAgentEventForWorkspace(ctx context.Context, eventID, workspaceID pgtype.UUID) (db.AgentEvent, error)
	ListAgentEventsByWorkspace(ctx context.Context, workspaceID pgtype.UUID, limit int32) ([]db.AgentEvent, error)
	MarkEventHandled(ctx context.Context, eventID pgtype.UUID) (db.AgentEvent, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
}

type Broadcaster interface {
	BroadcastAgentMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
	BroadcastAgentEvent(workspaceID pgtype.UUID, event db.AgentEvent)
	BroadcastAgentTask(workspaceID pgtype.UUID, task db.AgentTask)
}

type Service struct {
	runtime     Runtime
	broadcaster Broadcaster
}

type RequestDecisionInput struct {
	WorkspaceID     pgtype.UUID
	ThreadID        pgtype.UUID
	TaskID          pgtype.UUID
	CheckpointKey   string
	Arguments       map[string]any
	CheckpointValue []byte
}

type RequestDecisionOutput struct {
	EventID string
	Message db.AgentMessage
	Task    db.AgentTask
}

type RespondDecisionInput struct {
	WorkspaceID       pgtype.UUID
	EventID           pgtype.UUID
	SelectedOptionID  string
	FreeText          string
	ClientResponseID  string
	AllowNaturalText  bool
	ResumeTaskThread  pgtype.UUID
	ResumeTaskOwnerID pgtype.UUID
}

type RespondDecisionOutput struct {
	DecisionEvent db.AgentEvent
	ResolvedEvent db.AgentEvent
	Message       db.AgentMessage
	Task          db.AgentTask
}

func NewService(runtime Runtime, broadcasters ...Broadcaster) *Service {
	var broadcaster Broadcaster
	if len(broadcasters) > 0 {
		broadcaster = broadcasters[0]
	}
	return &Service{runtime: runtime, broadcaster: broadcaster}
}

func (s *Service) RequestUserDecision(ctx context.Context, input RequestDecisionInput) (RequestDecisionOutput, error) {
	payload := decisionPayload(input.Arguments, input.CheckpointKey)
	event, err := s.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "decision_requested",
		SourceRole:  "producer",
		TargetRole:  "user",
		Payload:     payload,
	})
	if err != nil {
		return RequestDecisionOutput{}, err
	}
	s.broadcastEvent(input.WorkspaceID, event)
	card := map[string]any{}
	_ = json.Unmarshal(payload, &card)
	cardContent, err := decisionCardContent(event.ID, card, "pending")
	if err != nil {
		return RequestDecisionOutput{}, err
	}
	message, err := s.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "assistant",
		MessageType: "ui_card",
		Content:     cardContent,
		EventID:     event.ID,
		TaskID:      input.TaskID,
	})
	if err != nil {
		return RequestDecisionOutput{}, err
	}
	s.broadcastMessage(input.WorkspaceID, message, event)
	if _, err := s.runtime.UpsertCheckpoint(ctx, agentruntime.UpsertCheckpointParams{
		Key:         input.CheckpointKey,
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		Value:       input.CheckpointValue,
		Metadata:    checkpointMetadata(CheckpointScope{WorkspaceID: input.WorkspaceID, ThreadID: input.ThreadID, TaskID: input.TaskID, InterruptType: "request_user_decision"}),
	}); err != nil {
		return RequestDecisionOutput{}, err
	}
	if _, err := s.runtime.SetThreadCheckpoint(ctx, input.ThreadID, input.CheckpointKey); err != nil {
		return RequestDecisionOutput{}, err
	}
	task, err := s.runtime.MarkTaskWaitingForUser(ctx, input.TaskID)
	if err != nil {
		return RequestDecisionOutput{}, err
	}
	s.broadcastTask(input.WorkspaceID, task)
	return RequestDecisionOutput{EventID: uuidString(event.ID), Message: message, Task: task}, nil
}

func (s *Service) broadcastMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	if s.broadcaster != nil {
		s.broadcaster.BroadcastAgentMessage(workspaceID, message, event)
	}
}

func (s *Service) broadcastEvent(workspaceID pgtype.UUID, event db.AgentEvent) {
	if s.broadcaster != nil {
		s.broadcaster.BroadcastAgentEvent(workspaceID, event)
	}
}

func (s *Service) broadcastTask(workspaceID pgtype.UUID, task db.AgentTask) {
	if s.broadcaster != nil {
		s.broadcaster.BroadcastAgentTask(workspaceID, task)
	}
}

func (s *Service) RespondDecision(ctx context.Context, input RespondDecisionInput) (RespondDecisionOutput, error) {
	event, err := s.runtime.GetAgentEventForWorkspace(ctx, input.EventID, input.WorkspaceID)
	if err != nil {
		return RespondDecisionOutput{}, err
	}
	if event.EventType != "decision_requested" || event.Status != "pending" {
		return RespondDecisionOutput{}, ErrInvalidDecisionResponse
	}
	if !decisionAllowsResponse(event.Payload, input.SelectedOptionID, input.FreeText, input.AllowNaturalText) {
		return RespondDecisionOutput{}, ErrInvalidDecisionResponse
	}
	message, err := s.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    event.ThreadID,
		Role:        "user",
		MessageType: "text",
		Content: decisionResponseContent(
			event.Payload,
			input.SelectedOptionID,
			input.FreeText,
			input.ClientResponseID,
		),
		EventID: event.ID,
	})
	if err != nil {
		return RespondDecisionOutput{}, err
	}
	handled, err := s.runtime.MarkEventHandled(ctx, event.ID)
	if err != nil {
		return RespondDecisionOutput{}, err
	}
	resolved, err := s.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    event.ThreadID,
		TaskID:      event.TaskID,
		EventType:   "decision_resolved",
		SourceRole:  "user",
		TargetRole:  "producer",
		Payload: mustJSON(map[string]any{
			"decision_id":        uuidString(event.ID),
			"selected_option_id": input.SelectedOptionID,
			"free_text":          strings.TrimSpace(input.FreeText),
		}),
	})
	if err != nil {
		return RespondDecisionOutput{}, err
	}
	checkpointKey, interruptIDs := resumeCheckpointFromEvents(ctx, s.runtime, event)
	resumeDecision := map[string]any{
		"decision_event_id":  uuidString(event.ID),
		"resolved_event_id":  uuidString(resolved.ID),
		"original_task_id":   uuidString(event.TaskID),
		"checkpoint_key":     checkpointKey,
		"interrupt_ids":      interruptIDs,
		"selected_option_id": input.SelectedOptionID,
		"free_text":          strings.TrimSpace(input.FreeText),
	}
	task, err := s.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    event.ThreadID,
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "decision_resume",
		MaxAttempts: 1,
		Input:       mustJSON(resumeDecision),
	})
	if err != nil {
		return RespondDecisionOutput{}, err
	}
	return RespondDecisionOutput{DecisionEvent: handled, ResolvedEvent: resolved, Message: message, Task: task}, nil
}

func resumeCheckpointFromEvents(ctx context.Context, runtime Runtime, decisionEvent db.AgentEvent) (string, []string) {
	events, err := runtime.ListAgentEventsByWorkspace(ctx, decisionEvent.WorkspaceID, 50)
	if err != nil {
		return checkpointKeyFromPayload(decisionEvent.Payload), nil
	}
	decisionCheckpoint := checkpointKeyFromPayload(decisionEvent.Payload)
	for _, event := range events {
		if event.EventType != "graph_interrupted" || event.TaskID != decisionEvent.TaskID {
			continue
		}
		var payload struct {
			CheckpointKey string   `json:"checkpoint_key"`
			InterruptIDs  []string `json:"interrupt_ids"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		if payload.CheckpointKey == "" || (decisionCheckpoint != "" && payload.CheckpointKey != decisionCheckpoint) {
			continue
		}
		return payload.CheckpointKey, payload.InterruptIDs
	}
	return decisionCheckpoint, nil
}

func checkpointKeyFromPayload(raw []byte) string {
	var payload struct {
		CheckpointKey string `json:"checkpoint_key"`
	}
	_ = json.Unmarshal(raw, &payload)
	return strings.TrimSpace(payload.CheckpointKey)
}

func decisionPayload(args map[string]any, checkpointKey string) []byte {
	payload := map[string]any{
		"title":          strings.TrimSpace(stringArg(args, "title")),
		"message":        strings.TrimSpace(stringArg(args, "message")),
		"checkpoint_key": checkpointKey,
	}
	if options, ok := args["options"]; ok {
		payload["options"] = options
	}
	if allow, ok := args["allow_free_text"].(bool); ok {
		payload["allow_free_text"] = allow
	}
	return mustJSON(payload)
}

func decisionAllowsResponse(raw []byte, optionID string, freeText string, allowNaturalText bool) bool {
	var payload struct {
		Options []struct {
			ID string `json:"id"`
		} `json:"options"`
		AllowFreeText bool `json:"allow_free_text"`
	}
	_ = json.Unmarshal(raw, &payload)
	if allowNaturalText && strings.TrimSpace(freeText) != "" {
		return true
	}
	if strings.TrimSpace(optionID) != "" {
		for _, option := range payload.Options {
			if option.ID == optionID {
				return true
			}
		}
	}
	return payload.AllowFreeText && strings.TrimSpace(freeText) != ""
}

func decisionResponseText(raw []byte, optionID string, freeText string) string {
	var payload struct {
		Options []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"options"`
	}
	_ = json.Unmarshal(raw, &payload)
	if strings.TrimSpace(optionID) == "" {
		return strings.TrimSpace(freeText)
	}
	label := optionID
	for _, option := range payload.Options {
		if option.ID == optionID && option.Label != "" {
			label = option.Label
			break
		}
	}
	if strings.TrimSpace(freeText) == "" {
		return "选择：" + label
	}
	return "选择：" + label + "。补充：" + strings.TrimSpace(freeText)
}

func decisionResponseContent(raw []byte, optionID string, freeText string, clientResponseID string) []byte {
	content, err := uimessage.BuildUserMessageContent(uimessage.UserMessageInput{
		Text:            decisionResponseText(raw, optionID, freeText),
		ClientMessageID: clientResponseID,
	})
	if err != nil {
		return []byte("{}")
	}
	return content
}

func decisionCardContent(eventID pgtype.UUID, card map[string]any, status string) ([]byte, error) {
	return uimessage.BuildDecisionCardMessageContent(uimessage.DecisionCardInput{
		DecisionID:    uuidString(eventID),
		Title:         strings.TrimSpace(stringFromAny(card["title"])),
		Message:       strings.TrimSpace(stringFromAny(card["message"])),
		Options:       decisionCardOptions(card["options"]),
		AllowFreeText: boolFromAny(card["allow_free_text"]),
		Status:        status,
	})
}

func decisionCardOptions(value any) []uimessage.DecisionOption {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var options []struct {
		ID          string `json:"id"`
		Label       string `json:"label"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &options); err == nil {
		out := make([]uimessage.DecisionOption, 0, len(options))
		for index, option := range options {
			label := strings.TrimSpace(option.Label)
			if label == "" {
				label = strings.TrimSpace(option.ID)
			}
			if label == "" {
				continue
			}
			id := strings.TrimSpace(option.ID)
			if id == "" {
				id = fmt.Sprintf("option_%d", index+1)
			}
			out = append(out, uimessage.DecisionOption{
				ID:          id,
				Label:       label,
				Description: strings.TrimSpace(option.Description),
			})
		}
		return out
	}

	var labels []string
	if err := json.Unmarshal(raw, &labels); err != nil {
		return nil
	}
	out := make([]uimessage.DecisionOption, 0, len(labels))
	for index, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		out = append(out, uimessage.DecisionOption{
			ID:    fmt.Sprintf("option_%d", index+1),
			Label: label,
		})
	}
	return out
}

func stringFromAny(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func boolFromAny(value any) bool {
	got, ok := value.(bool)
	return ok && got
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
