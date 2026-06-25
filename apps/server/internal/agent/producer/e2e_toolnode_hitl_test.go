package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agenthitl "github.com/sinmaystar/clip-anvil/internal/agent/hitl"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestE2EProducerExplicitToolNodeHITLAndDatabase(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("CLIPANVIL_E2E_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set CLIPANVIL_E2E_DATABASE_URL to run database e2e")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	queries := db.New(pool)
	runtimeSvc, err := agentruntime.NewService(pool, queries)
	if err != nil {
		t.Fatal(err)
	}
	email := fmt.Sprintf("producer-toolnode-hitl-%d@example.test", time.Now().UnixNano())
	account, err := queries.CreateAccount(ctx, db.CreateAccountParams{
		Email:        email,
		PasswordHash: "not-used",
		Name:         "Producer ToolNode HITL E2E",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM workspace WHERE owner_id = $1", account.ID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM account WHERE id = $1", account.ID)
	})
	workspace, err := queries.CreateWorkspace(ctx, db.CreateWorkspaceParams{
		Name:    "producer-toolnode-hitl-e2e",
		OwnerID: account.ID,
		Mode:    db.WorkspaceModeAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := runtimeSvc.GetOrCreateProducerThread(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	userContent, err := uimessage.BuildUserMessageContent(uimessage.UserMessageInput{
		Text:            "请先保存 brief，然后请求人工确认，最后继续。",
		ClientMessageID: "producer-toolnode-hitl-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}
	userMessage, err := runtimeSvc.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: workspace.ID,
		ThreadID:    thread.ID,
		Role:        "user",
		MessageType: "text",
		Content:     userContent,
	})
	if err != nil {
		t.Fatal(err)
	}

	nativeToolRegistry, err := agenttools.NewNativeRegistry(
		&testNativeTool{name: "create_agent_text_node"},
		agenttools.NewRequestUserDecisionNativeTool(agenthitl.NewToolDecisionRequester(agenthitl.NewService(runtimeSvc))),
	)
	if err != nil {
		t.Fatal(err)
	}
	responder := &sequenceResponder{outputs: []ProducerTurnOutput{
		nativeToolCallOutput("call-text", "create_agent_text_node", `{"title":"brief","text":"hello"}`),
		nativeToolCallOutput("call-decision", "request_user_decision", `{"title":"确认","message":"继续吗"}`),
		{AssistantText: "已根据人工确认继续。"},
	}}
	graphInfo := agenteino.NewGraphInfoRegistry()
	graph, err := NewGraph(GraphConfig{
		Mode: ProducerGraphModeExplicitToolLoop,
		Loader: RuntimeContextLoader{
			Runtime: runtimeSvc,
		},
		Responder:          responder,
		NativeToolRegistry: nativeToolRegistry,
		CheckPointStore:    agenteino.NewCheckpointStore(runtimeSvc, slog.Default()),
		CompileCallbacks:   []compose.GraphCompileCallback{graphInfo.CompileCallback()},
	})
	if err != nil {
		t.Fatal(err)
	}
	info, ok := graphInfo.Get("producer_turn")
	if !ok || info.Nodes["execute_tools"].Instance == nil {
		t.Fatalf("execute_tools node missing from graph info: %#v", info)
	}
	firstTask, err := runtimeSvc.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: workspace.ID,
		ThreadID:    thread.ID,
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "producer_turn",
		MaxAttempts: 1,
		Input:       mustJSON(map[string]any{"trigger": "e2e"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor(ExecutorConfig{
		Runtime:      runtimeSvc,
		Graph:        graph,
		MaxToolCalls: 5,
		ToolTimeout:  5 * time.Second,
	})
	if err := executor.RunTask(ctx, RunTaskInput{
		WorkspaceID:      workspace.ID,
		ThreadID:         thread.ID,
		TaskID:           firstTask.ID,
		TriggerMessageID: userMessage.ID,
	}); err != nil {
		t.Fatal(err)
	}
	firstTaskAfterInterrupt, err := queries.GetAgentTaskByID(ctx, firstTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstTaskAfterInterrupt.Status != "waiting_for_user" {
		t.Fatalf("first task status = %s, want waiting_for_user", firstTaskAfterInterrupt.Status)
	}
	threadAfterInterrupt, err := queries.GetAgentThreadByID(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !threadAfterInterrupt.CurrentCheckpointKey.Valid || threadAfterInterrupt.CurrentCheckpointKey.String == "" {
		t.Fatalf("thread checkpoint = %#v", threadAfterInterrupt.CurrentCheckpointKey)
	}
	interruptID := e2eInterruptID(t, queries, workspace.ID)

	resumeTask, err := runtimeSvc.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: workspace.ID,
		ThreadID:    thread.ID,
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "decision_resume",
		MaxAttempts: 1,
		Input:       mustJSON(map[string]any{"interrupt_id": interruptID}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.RunTask(ctx, RunTaskInput{
		WorkspaceID:        workspace.ID,
		ThreadID:           thread.ID,
		TaskID:             resumeTask.ID,
		ResumeCheckpointID: threadAfterInterrupt.CurrentCheckpointKey.String,
		ResumeData: map[string]any{
			interruptID: map[string]any{"selected_option_id": "continue"},
		},
		OriginalTaskID: firstTask.ID,
	}); err != nil {
		t.Fatal(err)
	}

	tasks, err := queries.ListAgentTasksByWorkspace(ctx, db.ListAgentTasksByWorkspaceParams{WorkspaceID: workspace.ID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertE2ETask(t, tasks, "producer_turn", "succeeded", 1)
	assertE2ETask(t, tasks, "decision_resume", "succeeded", 1)

	events, err := queries.ListAgentEventsByWorkspace(ctx, db.ListAgentEventsByWorkspaceParams{WorkspaceID: workspace.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []string{"producer_turn_started", "decision_requested", "graph_interrupted", "producer_turn_completed"} {
		if !e2eHasEvent(events, eventType) {
			t.Fatalf("event %q missing from %#v", eventType, eventTypes(events))
		}
	}
	messages, err := queries.ListAgentMessagesByThread(ctx, db.ListAgentMessagesByThreadParams{ThreadID: thread.ID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertE2EMessage(t, messages, "user", "text")
	assertE2EMessage(t, messages, "assistant", "tool_call")
	assertE2EMessage(t, messages, "tool", "tool_result")
	assertE2EMessage(t, messages, "assistant", "ui_card")
	assertE2EMessage(t, messages, "assistant", "text")
	if !e2eAssistantTextContains(messages, "已根据人工确认继续。") {
		t.Fatalf("final assistant message missing from %#v", messages)
	}
}

func e2eInterruptID(t *testing.T, queries *db.Queries, workspaceID pgtype.UUID) string {
	t.Helper()
	events, err := queries.ListAgentEventsByWorkspace(context.Background(), db.ListAgentEventsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.EventType != "graph_interrupted" {
			continue
		}
		var payload struct {
			InterruptIDs []string `json:"interrupt_ids"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.InterruptIDs) > 0 && payload.InterruptIDs[0] != "" {
			return payload.InterruptIDs[0]
		}
	}
	t.Fatalf("graph_interrupted event with interrupt id not found in %#v", eventTypes(events))
	return ""
}

func assertE2ETask(t *testing.T, tasks []db.AgentTask, taskType string, status string, minCount int) {
	t.Helper()
	count := 0
	for _, task := range tasks {
		if task.TaskType == taskType && task.Status == status {
			count++
		}
	}
	if count < minCount {
		t.Fatalf("task %s/%s count = %d, want at least %d; tasks=%#v", taskType, status, count, minCount, tasks)
	}
}

func e2eHasEvent(events []db.AgentEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func eventTypes(events []db.AgentEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.EventType)
	}
	return out
}

func assertE2EMessage(t *testing.T, messages []db.AgentMessage, role string, messageType string) {
	t.Helper()
	for _, message := range messages {
		if message.Role == role && message.MessageType == messageType {
			return
		}
	}
	t.Fatalf("message %s/%s missing from %#v", role, messageType, messages)
}

func e2eAssistantTextContains(messages []db.AgentMessage, text string) bool {
	for _, message := range messages {
		if message.Role == "assistant" && message.MessageType == "text" && strings.Contains(string(message.Content), text) {
			return true
		}
	}
	return false
}
