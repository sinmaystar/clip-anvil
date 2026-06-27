package api

import (
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/modelselection"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type agentThreadResponse struct {
	ID                   string  `json:"id"`
	WorkspaceID          string  `json:"workspace_id"`
	Role                 string  `json:"role"`
	ScopeType            string  `json:"scope_type"`
	ScopeID              *string `json:"scope_id"`
	RuntimeProvider      string  `json:"runtime_provider"`
	RuntimeAgentName     string  `json:"runtime_agent_name"`
	CurrentCheckpointKey *string `json:"current_checkpoint_key"`
	Status               string  `json:"status"`
	Summary              string  `json:"summary"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

type agentObservedThreadResponse struct {
	agentThreadResponse
	DisplayName          string             `json:"display_name"`
	ScopeLabel           string             `json:"scope_label"`
	ScopeTitle           string             `json:"scope_title,omitempty"`
	LatestTask           *agentTaskResponse `json:"latest_task,omitempty"`
	LatestMessageAt      string             `json:"latest_message_at,omitempty"`
	LatestMessagePreview string             `json:"latest_message_preview,omitempty"`
	Metadata             map[string]any     `json:"metadata,omitempty"`
}

type agentMessageResponse struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id"`
	ThreadID    string         `json:"thread_id"`
	Seq         int64          `json:"seq"`
	Role        string         `json:"role"`
	MessageType string         `json:"message_type"`
	Content     map[string]any `json:"content"`
	RawMessage  map[string]any `json:"raw_message"`
	TaskID      *string        `json:"task_id"`
	EventID     *string        `json:"event_id"`
	CreatedAt   string         `json:"created_at"`
}

type agentEventResponse struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id"`
	ThreadID    *string        `json:"thread_id"`
	TaskID      *string        `json:"task_id"`
	EventType   string         `json:"event_type"`
	SourceRole  string         `json:"source_role"`
	TargetRole  *string        `json:"target_role"`
	Scope       map[string]any `json:"scope"`
	Payload     map[string]any `json:"payload"`
	Status      string         `json:"status"`
	CreatedAt   string         `json:"created_at"`
	HandledAt   *string        `json:"handled_at"`
}

type agentTaskResponse struct {
	ID           string         `json:"id"`
	WorkspaceID  string         `json:"workspace_id"`
	ThreadID     *string        `json:"thread_id"`
	Role         string         `json:"role"`
	ScopeType    string         `json:"scope_type"`
	ScopeID      *string        `json:"scope_id"`
	TaskType     string         `json:"task_type"`
	Status       string         `json:"status"`
	Attempt      int32          `json:"attempt"`
	MaxAttempts  int32          `json:"max_attempts"`
	Input        map[string]any `json:"input"`
	Output       map[string]any `json:"output"`
	ErrorCode    *string        `json:"error_code"`
	ErrorMessage *string        `json:"error_message"`
	CreatedAt    string         `json:"created_at"`
	StartedAt    *string        `json:"started_at"`
	CompletedAt  *string        `json:"completed_at"`
}

type agentModelRefResponse struct {
	ProviderID      string `json:"provider_id"`
	ModelID         string `json:"model_id"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type agentModelSelectionResponse struct {
	Producer agentModelRefResponse `json:"producer"`
}

type agentModelOptionResponse struct {
	ProviderID             string         `json:"provider_id"`
	ModelID                string         `json:"model_id"`
	DisplayName            string         `json:"display_name"`
	Limits                 map[string]any `json:"limits"`
	Pricing                map[string]any `json:"pricing"`
	SupportsThinking       bool           `json:"supports_thinking"`
	ReasoningEfforts       []string       `json:"reasoning_efforts"`
	DefaultReasoningEffort string         `json:"default_reasoning_effort"`
}

type getAgentModelSelectionResponse struct {
	Selection agentModelSelectionResponse `json:"selection"`
	Defaults  agentModelSelectionResponse `json:"defaults"`
	Options   []agentModelOptionResponse  `json:"options"`
}

type postAgentMessageRequest struct {
	Text            string                   `json:"text"`
	ClientMessageID string                   `json:"client_message_id"`
	Attachments     []agentMessageAttachment `json:"attachments"`
}

type postAgentDecisionRequest struct {
	SelectedOptionID string `json:"selected_option_id"`
	FreeText         string `json:"free_text"`
	ClientResponseID string `json:"client_response_id"`
}

type putAgentModelSelectionRequest struct {
	Producer agentModelRefResponse `json:"producer"`
}

func (r putAgentModelSelectionRequest) valid() bool {
	return strings.TrimSpace(r.Producer.ProviderID) != "" &&
		strings.TrimSpace(r.Producer.ModelID) != "" &&
		validReasoningEffort(r.Producer.ReasoningEffort)
}

func (r postAgentMessageRequest) valid() bool {
	text := strings.TrimSpace(r.Text)
	return text != "" && len([]rune(text)) <= 8000 &&
		len([]rune(r.ClientMessageID)) <= 128 &&
		len(r.Attachments) <= 12 &&
		agentMessageAttachmentsValid(r.Attachments)
}

func (r postAgentDecisionRequest) valid() bool {
	return (strings.TrimSpace(r.SelectedOptionID) != "" || strings.TrimSpace(r.FreeText) != "") &&
		len([]rune(r.SelectedOptionID)) <= 128 &&
		len([]rune(r.FreeText)) <= 2000 &&
		len([]rune(r.ClientResponseID)) <= 128
}

func (r postAgentMessageRequest) trimmedText() string {
	return strings.TrimSpace(r.Text)
}

func agentMessageAttachmentsValid(attachments []agentMessageAttachment) bool {
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.AssetID) == "" ||
			strings.TrimSpace(attachment.NodeID) == "" ||
			strings.TrimSpace(attachment.Name) == "" {
			return false
		}
		switch attachment.Kind {
		case "text", "image", "video":
		default:
			return false
		}
	}
	return true
}

func toAgentThreadResponse(thread db.AgentThread) agentThreadResponse {
	return agentThreadResponse{
		ID:                   uuidToString(thread.ID),
		WorkspaceID:          uuidToString(thread.WorkspaceID),
		Role:                 thread.Role,
		ScopeType:            thread.ScopeType,
		ScopeID:              nullableUUIDString(thread.ScopeID),
		RuntimeProvider:      thread.RuntimeProvider,
		RuntimeAgentName:     thread.RuntimeAgentName,
		CurrentCheckpointKey: nullableTextString(thread.CurrentCheckpointKey),
		Status:               thread.Status,
		Summary:              thread.Summary,
		CreatedAt:            timestamptzString(thread.CreatedAt),
		UpdatedAt:            timestamptzString(thread.UpdatedAt),
	}
}

func toObservedAgentThreadResponse(thread db.AgentThread, latestTask *db.AgentTask, latestMessage *db.AgentMessage) agentObservedThreadResponse {
	base := toAgentThreadResponse(thread)
	out := agentObservedThreadResponse{
		agentThreadResponse: base,
		DisplayName:         observedThreadDisplayName(thread),
		ScopeLabel:          observedThreadScopeLabel(thread),
	}
	if latestTask != nil {
		task := toAgentTaskResponse(*latestTask)
		out.LatestTask = &task
	}
	if latestMessage != nil {
		out.LatestMessageAt = timestamptzString(latestMessage.CreatedAt)
		out.LatestMessagePreview = agentMessagePreview(*latestMessage)
	}
	return out
}

func observedThreadDisplayName(thread db.AgentThread) string {
	role := observedThreadRoleName(thread.Role)
	label := observedThreadScopeLabel(thread)
	if label == "" {
		return role
	}
	return role + " · " + label
}

func observedThreadRoleName(role string) string {
	switch role {
	case "craftsman":
		return "Craftsman"
	case "reviewer":
		return "Reviewer"
	case "composer":
		return "Composer"
	case "producer":
		return "Producer"
	default:
		return role
	}
}

func observedThreadScopeLabel(thread db.AgentThread) string {
	if thread.ScopeID.Valid {
		return thread.ScopeType + ":" + uuidToString(thread.ScopeID)
	}
	return thread.ScopeType
}

func agentMessagePreview(message db.AgentMessage) string {
	return trimPreviewText(agentMessageContentText(jsonObjectMap(message.Content)))
}

func agentMessageContentText(content map[string]any) string {
	blocks, _ := content["blocks"].([]any)
	lines := make([]string, 0, len(blocks))
	for _, block := range blocks {
		value, _ := block.(map[string]any)
		text, _ := value["text"].(string)
		if strings.TrimSpace(text) != "" {
			lines = append(lines, strings.TrimSpace(text))
		}
	}
	if len(lines) > 0 {
		return strings.Join(lines, "\n")
	}
	text, _ := content["text"].(string)
	return strings.TrimSpace(text)
}

func trimPreviewText(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= 160 {
		return string(runes)
	}
	return string(runes[:160])
}

func toAgentMessageResponse(msg db.AgentMessage) agentMessageResponse {
	return agentMessageResponse{
		ID:          uuidToString(msg.ID),
		WorkspaceID: uuidToString(msg.WorkspaceID),
		ThreadID:    uuidToString(msg.ThreadID),
		Seq:         msg.Seq,
		Role:        msg.Role,
		MessageType: msg.MessageType,
		Content:     jsonObjectMap(msg.Content),
		RawMessage:  jsonObjectMap(msg.RawMessage),
		TaskID:      nullableUUIDString(msg.TaskID),
		EventID:     nullableUUIDString(msg.EventID),
		CreatedAt:   timestamptzString(msg.CreatedAt),
	}
}

func toAgentEventResponse(event db.AgentEvent) agentEventResponse {
	return agentEventResponse{
		ID:          uuidToString(event.ID),
		WorkspaceID: uuidToString(event.WorkspaceID),
		ThreadID:    nullableUUIDString(event.ThreadID),
		TaskID:      nullableUUIDString(event.TaskID),
		EventType:   event.EventType,
		SourceRole:  event.SourceRole,
		TargetRole:  nullableTextString(event.TargetRole),
		Scope:       jsonObjectMap(event.Scope),
		Payload:     jsonObjectMap(event.Payload),
		Status:      event.Status,
		CreatedAt:   timestamptzString(event.CreatedAt),
		HandledAt:   nullableTimestamptzString(event.HandledAt),
	}
}

func toAgentTaskResponse(task db.AgentTask) agentTaskResponse {
	return agentTaskResponse{
		ID:           uuidToString(task.ID),
		WorkspaceID:  uuidToString(task.WorkspaceID),
		ThreadID:     nullableUUIDString(task.ThreadID),
		Role:         task.Role,
		ScopeType:    task.ScopeType,
		ScopeID:      nullableUUIDString(task.ScopeID),
		TaskType:     task.TaskType,
		Status:       task.Status,
		Attempt:      task.Attempt,
		MaxAttempts:  task.MaxAttempts,
		Input:        jsonObjectMap(task.Input),
		Output:       jsonObjectMap(task.Output),
		ErrorCode:    nullableTextString(task.ErrorCode),
		ErrorMessage: nullableTextString(task.ErrorMessage),
		CreatedAt:    timestamptzString(task.CreatedAt),
		StartedAt:    nullableTimestamptzString(task.StartedAt),
		CompletedAt:  nullableTimestamptzString(task.CompletedAt),
	}
}

func toAgentTasksResponse(tasks []db.AgentTask) agentTasksResponse {
	out := make([]agentTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, toAgentTaskResponse(task))
	}
	return agentTasksResponse{Tasks: out}
}

func toAgentModelSelectionResponse(resolved modelselection.Resolved) getAgentModelSelectionResponse {
	options := make([]agentModelOptionResponse, 0, len(resolved.Options))
	for _, option := range resolved.Options {
		options = append(options, agentModelOptionResponse{
			ProviderID:             option.ProviderID,
			ModelID:                option.ModelID,
			DisplayName:            option.DisplayName,
			Limits:                 option.Limits,
			Pricing:                option.Pricing,
			SupportsThinking:       option.SupportsThinking,
			ReasoningEfforts:       option.ReasoningEfforts,
			DefaultReasoningEffort: option.DefaultReasoningEffort,
		})
	}
	return getAgentModelSelectionResponse{
		Selection: agentModelSelectionResponse{Producer: agentModelRefResponse{
			ProviderID:      resolved.Selection.Producer.ProviderID,
			ModelID:         resolved.Selection.Producer.ModelID,
			ReasoningEffort: resolved.Selection.Producer.ReasoningEffort,
		}},
		Defaults: agentModelSelectionResponse{Producer: agentModelRefResponse{
			ProviderID:      resolved.Defaults.Producer.ProviderID,
			ModelID:         resolved.Defaults.Producer.ModelID,
			ReasoningEffort: resolved.Defaults.Producer.ReasoningEffort,
		}},
		Options: options,
	}
}

func validReasoningEffort(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	switch value {
	case "minimal", "low", "medium", "high":
		return true
	default:
		return false
	}
}

func jsonObjectMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func nullableUUIDString(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	out := uuidToString(value)
	return &out
}

func nullableTextString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timestamptzString(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
}

func nullableTimestamptzString(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	out := timestamptzString(value)
	return &out
}
