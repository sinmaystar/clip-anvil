package api

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sinmaystar/clip-anvil/internal/agent/modelselection"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestAgentMessageResponseMapsTextContent(t *testing.T) {
	msg := db.AgentMessage{
		ID:          testUUID(0x01),
		WorkspaceID: testUUID(0x02),
		ThreadID:    testUUID(0x03),
		Seq:         7,
		Role:        "user",
		MessageType: "text",
		Content:     []byte(`{"text":"hello"}`),
		RawMessage:  []byte(`{}`),
	}

	got := toAgentMessageResponse(msg)

	if got.ID != uuidToString(testUUID(0x01)) {
		t.Fatalf("id = %q", got.ID)
	}
	if got.Seq != 7 || got.Role != "user" || got.MessageType != "text" {
		t.Fatalf("message response = %#v", got)
	}
	if got.Content["text"] != "hello" {
		t.Fatalf("content = %#v", got.Content)
	}
}

func TestAgentMessageResponseDefaultsInvalidJSONToObject(t *testing.T) {
	msg := db.AgentMessage{
		ID:          testUUID(0x01),
		WorkspaceID: testUUID(0x02),
		ThreadID:    testUUID(0x03),
		Role:        "user",
		MessageType: "text",
		Content:     []byte(`not-json`),
		RawMessage:  nil,
	}

	got := toAgentMessageResponse(msg)

	if len(got.Content) != 0 {
		t.Fatalf("content = %#v, want empty object", got.Content)
	}
	if len(got.RawMessage) != 0 {
		t.Fatalf("raw_message = %#v, want empty object", got.RawMessage)
	}
}

func TestAgentTaskResponseMapsTask(t *testing.T) {
	task := db.AgentTask{
		ID:          testUUID(0x11),
		WorkspaceID: testUUID(0x12),
		ThreadID:    testUUID(0x13),
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "producer_turn",
		Status:      "queued",
		Attempt:     0,
		MaxAttempts: 1,
		Input:       []byte(`{"trigger_message_seq":3}`),
		Output:      []byte(`{}`),
	}

	got := toAgentTaskResponse(task)

	if got.ID != uuidToString(testUUID(0x11)) || got.TaskType != "producer_turn" || got.Status != "queued" {
		t.Fatalf("task response = %#v", got)
	}
	if got.Input["trigger_message_seq"].(float64) != 3 {
		t.Fatalf("task input = %#v", got.Input)
	}
}

func TestAgentTasksResponseMapsActiveTasks(t *testing.T) {
	tasks := []db.AgentTask{
		{
			ID:          testUUID(0x21),
			WorkspaceID: testUUID(0x22),
			ThreadID:    testUUID(0x23),
			Role:        "producer",
			ScopeType:   "workspace",
			TaskType:    "producer_turn",
			Status:      "running",
			Input:       []byte(`{"trigger_message_seq":9}`),
			Output:      []byte(`{}`),
		},
	}

	got := toAgentTasksResponse(tasks)

	if len(got.Tasks) != 1 || got.Tasks[0].Status != "running" || got.Tasks[0].TaskType != "producer_turn" {
		t.Fatalf("tasks response = %#v", got)
	}
}

func TestAgentModelSelectionResponseMapsResolvedSelection(t *testing.T) {
	resolved := modelselection.Resolved{
		Selection: modelselection.Selection{Producer: modelselection.ModelRef{ProviderID: "volcengine", ModelID: "doubao-mini", ReasoningEffort: "high"}},
		Defaults:  modelselection.Selection{Producer: modelselection.ModelRef{ProviderID: "volcengine", ModelID: "env-default"}},
		Options: []modelselection.Option{
			{
				ProviderID:             "volcengine",
				ModelID:                "doubao-mini",
				DisplayName:            "Doubao Mini",
				Limits:                 map[string]any{"max_prompt_chars": float64(16000)},
				Pricing:                map[string]any{"tier": "real"},
				SupportsThinking:       true,
				ReasoningEfforts:       []string{"minimal", "low", "medium", "high"},
				DefaultReasoningEffort: "medium",
			},
		},
	}

	got := toAgentModelSelectionResponse(resolved)

	if got.Selection.Producer.ModelID != "doubao-mini" {
		t.Fatalf("selection = %#v", got.Selection)
	}
	if got.Selection.Producer.ReasoningEffort != "high" {
		t.Fatalf("selection reasoning_effort = %q, want high", got.Selection.Producer.ReasoningEffort)
	}
	if got.Defaults.Producer.ModelID != "env-default" {
		t.Fatalf("defaults = %#v", got.Defaults)
	}
	if len(got.Options) != 1 || got.Options[0].DisplayName != "Doubao Mini" {
		t.Fatalf("options = %#v", got.Options)
	}
	if !got.Options[0].SupportsThinking || got.Options[0].DefaultReasoningEffort != "medium" {
		t.Fatalf("thinking option = %#v", got.Options[0])
	}
	if len(got.Options[0].ReasoningEfforts) != 4 || got.Options[0].ReasoningEfforts[3] != "high" {
		t.Fatalf("reasoning efforts = %#v", got.Options[0].ReasoningEfforts)
	}
}

func TestPutAgentModelSelectionRequestValidatesProducer(t *testing.T) {
	if (putAgentModelSelectionRequest{}).valid() {
		t.Fatal("empty producer selection must be invalid")
	}
	req := putAgentModelSelectionRequest{Producer: agentModelRefResponse{
		ProviderID:      "volcengine",
		ModelID:         "doubao-mini",
		ReasoningEffort: "medium",
	}}
	if !req.valid() {
		t.Fatal("valid producer model selection was rejected")
	}
	invalidEffort := putAgentModelSelectionRequest{Producer: agentModelRefResponse{
		ProviderID:      "volcengine",
		ModelID:         "doubao-mini",
		ReasoningEffort: "deep",
	}}
	if invalidEffort.valid() {
		t.Fatal("unknown reasoning effort must be invalid")
	}
}

func TestPostAgentDecisionRequestValidatesChoiceOrFreeText(t *testing.T) {
	if (postAgentDecisionRequest{}).valid() {
		t.Fatal("empty decision response must be invalid")
	}
	if !(postAgentDecisionRequest{SelectedOptionID: "a"}).valid() {
		t.Fatal("selected option should be valid")
	}
	if !(postAgentDecisionRequest{FreeText: "补充说明"}).valid() {
		t.Fatal("free text should be valid")
	}
}

func TestAgentBusyReasonBlocksActiveTasks(t *testing.T) {
	for _, status := range []string{"queued", "running"} {
		if reason := agentBusyReason([]db.AgentTask{{TaskType: "producer_turn", Status: status}}); reason == "" {
			t.Fatalf("status %s should be busy", status)
		}
	}
	if reason := agentBusyReason([]db.AgentTask{{TaskType: "producer_turn", Status: "succeeded"}}); reason != "" {
		t.Fatalf("succeeded task busy reason = %q", reason)
	}
	if reason := agentBusyReason([]db.AgentTask{{TaskType: "craftsman_turn", Status: "running"}}); reason != "" {
		t.Fatalf("async craftsman task should not block chat input, reason = %q", reason)
	}
	if reason := agentBusyReason([]db.AgentTask{{TaskType: "worker_generation", Status: "queued"}}); reason != "" {
		t.Fatalf("async worker task should not block chat input, reason = %q", reason)
	}
	if reason := agentBusyReason([]db.AgentTask{{TaskType: "producer_turn", Status: "waiting_for_user"}}); reason != "" {
		t.Fatalf("pending decision should be handled by decision UI, not processing lock, reason = %q", reason)
	}
}

func TestAgentProcessingReasonOnlyBlocksProducerExecution(t *testing.T) {
	if reason := agentProcessingReason([]db.AgentTask{{TaskType: "producer_turn", Status: "running"}}); reason == "" {
		t.Fatal("running producer turn should block posting a new message")
	}
	if reason := agentProcessingReason([]db.AgentTask{{TaskType: "decision_resume", Status: "queued"}}); reason == "" {
		t.Fatal("queued decision resume should block posting a new message")
	}
	if reason := agentProcessingReason([]db.AgentTask{{TaskType: "craftsman_turn", Status: "running"}}); reason != "" {
		t.Fatalf("running craftsman task should not block posting a message, reason = %q", reason)
	}
	if reason := agentProcessingReason([]db.AgentTask{{TaskType: "worker_generation", Status: "queued"}}); reason != "" {
		t.Fatalf("queued worker task should not block posting a message, reason = %q", reason)
	}
}

func TestListMessagesUsesProducerThreadOnly(t *testing.T) {
	source, err := os.ReadFile("agent_handler.go")
	if err != nil {
		t.Fatalf("read agent_handler.go: %v", err)
	}
	body := string(source)
	start := strings.Index(body, "func (h *AgentHandler) ListMessages")
	if start < 0 {
		t.Fatal("AgentHandler.ListMessages must exist")
	}
	end := strings.Index(body[start:], "\nfunc (h *AgentHandler) ListActiveTasks")
	if end < 0 {
		t.Fatal("AgentHandler.ListActiveTasks must follow ListMessages")
	}
	listMessagesBody := body[start : start+end]
	if strings.Contains(listMessagesBody, "ListWorkspaceMessages") {
		t.Fatal("AgentHandler.ListMessages must not list workspace-level messages for the chat dialog")
	}
	if !strings.Contains(listMessagesBody, "h.runtime.ListMessages(ctx, thread.ID") {
		t.Fatalf("AgentHandler.ListMessages must list messages from the Producer thread, got:\n%s", listMessagesBody)
	}
}

func TestPostAgentMessageRequestRejectsBlankText(t *testing.T) {
	req := postAgentMessageRequest{Text: "   "}

	if req.valid() {
		t.Fatal("blank text must be invalid")
	}
}

func TestPostAgentMessageRequestRejectsLongClientMessageID(t *testing.T) {
	req := postAgentMessageRequest{Text: "hello", ClientMessageID: string(make([]rune, 129))}

	if req.valid() {
		t.Fatal("long client message id must be invalid")
	}
}

func TestAgentMessageContentIncludesClientMessageIDBlockMetadata(t *testing.T) {
	body := agentMessageContent("hello", "client-1")

	var got struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"blocks"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if got.Schema != uimessage.SchemaV1 {
		t.Fatalf("schema = %q", got.Schema)
	}
	if len(got.Blocks) != 1 || got.Blocks[0].Type != "markdown" || got.Blocks[0].Text != "hello" {
		t.Fatalf("blocks = %#v", got.Blocks)
	}
	if got.Metadata["client_message_id"] != "client-1" {
		t.Fatalf("metadata = %#v", got.Metadata)
	}
}

func TestAgentMessageContentIncludesAttachments(t *testing.T) {
	attachments := []agentMessageAttachment{
		{
			AssetID:   "asset-1",
			NodeID:    "node-1",
			Kind:      "text",
			Name:      "brief.txt",
			Mime:      "text/plain; charset=utf-8",
			SizeBytes: 128,
		},
	}

	body := agentMessageContent("hello", "client-1", attachments)

	var got struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type        string                   `json:"type"`
			Text        string                   `json:"text"`
			Attachments []agentMessageAttachment `json:"attachments"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if got.Schema != uimessage.SchemaV1 {
		t.Fatalf("schema = %q", got.Schema)
	}
	if len(got.Blocks) != 2 || got.Blocks[0].Text != "hello" || got.Blocks[1].Type != "attachment" {
		t.Fatalf("blocks = %#v", got.Blocks)
	}
	if len(got.Blocks[1].Attachments) != 1 || got.Blocks[1].Attachments[0].NodeID != "node-1" {
		t.Fatalf("attachments = %#v", got.Blocks[1].Attachments)
	}
}

func TestHydrateDecisionCardStatusUpdatesNestedBlock(t *testing.T) {
	content := map[string]any{
		"schema": uimessage.SchemaV1,
		"blocks": []any{
			map[string]any{
				"id":          "blk_decision",
				"type":        "decision_card",
				"decision_id": "decision-1",
				"title":       "确认",
				"message":     "继续吗",
				"status":      "pending",
			},
		},
	}

	hydrateDecisionCardStatus(content, "handled")

	blocks := content["blocks"].([]any)
	block := blocks[0].(map[string]any)
	if block["status"] != "handled" {
		t.Fatalf("block status = %#v", block["status"])
	}
}

func TestHydrateDecisionCardFromEventPayloadBackfillsOptions(t *testing.T) {
	content := map[string]any{
		"schema": uimessage.SchemaV1,
		"blocks": []any{
			map[string]any{
				"id":              "blk_decision",
				"type":            "decision_card",
				"decision_id":     "decision-1",
				"title":           "确认",
				"message":         "继续吗",
				"status":          "pending",
				"options":         []any{},
				"allow_free_text": false,
			},
		},
	}
	event := db.AgentEvent{
		Status: "pending",
		Payload: []byte(`{
			"title":"是否启动生成",
			"message":"请选择后续动作",
			"options":["立即生成","稍后再说"],
			"allow_free_text":true
		}`),
	}

	hydrateDecisionCardFromEvent(content, event)

	block := content["blocks"].([]any)[0].(map[string]any)
	options := block["options"].([]any)
	first := options[0].(map[string]any)
	if len(options) != 2 || first["id"] != "option_1" || first["label"] != "立即生成" {
		t.Fatalf("options = %#v", options)
	}
	if block["allow_free_text"] != true || block["title"] != "是否启动生成" || block["message"] != "请选择后续动作" {
		t.Fatalf("block = %#v", block)
	}
}

func TestAgentMessageAttachmentAllowsHydratedRenderURLs(t *testing.T) {
	attachment := agentMessageAttachment{
		AssetID:      "asset-1",
		NodeID:       "node-1",
		Kind:         "image",
		Name:         "design.png",
		Mime:         "image/png",
		SizeBytes:    128,
		URL:          "http://localhost/image.png",
		ThumbnailURL: "http://localhost/thumb.png",
	}

	raw, err := json.Marshal(attachment)
	if err != nil {
		t.Fatalf("marshal attachment: %v", err)
	}
	if !strings.Contains(string(raw), `"url":"http://localhost/image.png"`) {
		t.Fatalf("raw = %s", raw)
	}
	if !strings.Contains(string(raw), `"thumbnail_url":"http://localhost/thumb.png"`) {
		t.Fatalf("raw = %s", raw)
	}
}

func TestAgentMessageContentDoesNotPersistRenderURLs(t *testing.T) {
	body := agentMessageContent("hello", "client-1", []agentMessageAttachment{
		{
			AssetID:      "asset-1",
			NodeID:       "node-1",
			Kind:         "image",
			Name:         "design.png",
			Mime:         "image/png",
			SizeBytes:    128,
			URL:          "http://localhost/image.png",
			ThumbnailURL: "http://localhost/thumb.png",
		},
	})

	if strings.Contains(string(body), `"url"`) || strings.Contains(string(body), `"thumbnail_url"`) {
		t.Fatalf("content must not persist render URLs: %s", body)
	}
}

func TestAgentMessageContentOmitsBlankClientMessageID(t *testing.T) {
	body := agentMessageContent("hello", "")

	var got struct {
		Schema   string         `json:"schema"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if got.Schema != uimessage.SchemaV1 {
		t.Fatalf("schema = %q", got.Schema)
	}
	if got.Metadata != nil {
		t.Fatalf("metadata = %#v, want omitted", got.Metadata)
	}
}

func TestAgentAttachmentKindForMIMERejectsUnsupportedMIME(t *testing.T) {
	if _, ok := agentAttachmentKindForMIME("application/pdf"); ok {
		t.Fatal("pdf must not be accepted as an agent attachment in M6.4")
	}
}

func TestAgentAttachmentKindForMIMEAcceptsTextPlainWithCharset(t *testing.T) {
	got, ok := agentAttachmentKindForMIME("text/plain; charset=utf-8")
	if !ok {
		t.Fatal("text/plain with charset must be accepted")
	}
	if got != "text" {
		t.Fatalf("kind = %q, want text", got)
	}
}

func TestProducerTurnTaskInputIncludesTriggerMessage(t *testing.T) {
	msg := db.AgentMessage{ID: testUUID(0x31), Seq: 9}

	body := producerTurnTaskInput(msg)

	if !strings.Contains(string(body), `"trigger_message_id":"31000000-0000-0000-0000-000000000000"`) {
		t.Fatalf("task input = %s", body)
	}
	if !strings.Contains(string(body), `"trigger_message_seq":9`) {
		t.Fatalf("task input = %s", body)
	}
}

func TestNewAgentWSHandlerConstructs(t *testing.T) {
	handler := NewAgentWSHandler(db.New(fakeAgentDBTX{}), NewAgentHub(), "secret")

	if handler == nil {
		t.Fatal("handler should be constructed")
	}
}

func TestAgentProductionOverviewRouteContract(t *testing.T) {
	handlerSource, err := os.ReadFile("agent_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handlerSource), "func (h *AgentHandler) GetProductionOverview") {
		t.Fatal("AgentHandler.GetProductionOverview must be implemented")
	}
	if !strings.Contains(string(handlerSource), "func (h *AgentHandler) ListActiveTasks") {
		t.Fatal("AgentHandler.ListActiveTasks must be implemented")
	}

	serverSource, err := os.ReadFile("../../cmd/server/main.go")
	if err != nil {
		t.Fatal(err)
	}
	wantRoute := `GET("/api/agent/workspaces/:workspaceID/production-overview", authMiddleware, agentHandler.GetProductionOverview)`
	if !strings.Contains(string(serverSource), wantRoute) {
		t.Fatalf("server route %q is not registered", wantRoute)
	}
	wantTasksRoute := `GET("/api/agent/workspaces/:workspaceID/tasks", authMiddleware, agentHandler.ListActiveTasks)`
	if !strings.Contains(string(serverSource), wantTasksRoute) {
		t.Fatalf("server route %q is not registered", wantTasksRoute)
	}
}

func TestAgentCanvasWorkbenchRouteContract(t *testing.T) {
	handlerSource, err := os.ReadFile("agent_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handlerSource), "func (h *AgentHandler) GetCanvasWorkbench") {
		t.Fatal("AgentHandler.GetCanvasWorkbench must be implemented")
	}
	if !strings.Contains(string(handlerSource), "func (h *AgentHandler) GetCanvasDetail") {
		t.Fatal("AgentHandler.GetCanvasDetail must be implemented")
	}

	serverSource, err := os.ReadFile("../../cmd/server/main.go")
	if err != nil {
		t.Fatal(err)
	}
	wantRoute := `GET("/api/agent/workspaces/:workspaceID/canvas/workbench", authMiddleware, agentHandler.GetCanvasWorkbench)`
	if !strings.Contains(string(serverSource), wantRoute) {
		t.Fatalf("server route %q is not registered", wantRoute)
	}
	wantDetailRoute := `GET("/api/agent/workspaces/:workspaceID/canvas/details", authMiddleware, agentHandler.GetCanvasDetail)`
	if !strings.Contains(string(serverSource), wantDetailRoute) {
		t.Fatalf("server route %q is not registered", wantDetailRoute)
	}
}

func TestAgentThreadObserverRouteContract(t *testing.T) {
	handlerSource, err := os.ReadFile("agent_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handlerSource), "func (h *AgentHandler) ListThreads") {
		t.Fatal("AgentHandler.ListThreads must be implemented")
	}
	if !strings.Contains(string(handlerSource), "func (h *AgentHandler) ListThreadMessages") {
		t.Fatal("AgentHandler.ListThreadMessages must be implemented")
	}

	serverSource, err := os.ReadFile("../../cmd/server/main.go")
	if err != nil {
		t.Fatal(err)
	}
	wantThreadsRoute := `GET("/api/agent/workspaces/:workspaceID/threads", authMiddleware, agentHandler.ListThreads)`
	if !strings.Contains(string(serverSource), wantThreadsRoute) {
		t.Fatalf("server route %q is not registered", wantThreadsRoute)
	}
	wantMessagesRoute := `GET("/api/agent/workspaces/:workspaceID/threads/:threadID/messages", authMiddleware, agentHandler.ListThreadMessages)`
	if !strings.Contains(string(serverSource), wantMessagesRoute) {
		t.Fatalf("server route %q is not registered", wantMessagesRoute)
	}
}

type fakeAgentDBTX struct{}

func (fakeAgentDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (fakeAgentDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (fakeAgentDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return nil
}
