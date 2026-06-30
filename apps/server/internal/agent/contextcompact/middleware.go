package contextcompact

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"
)

type CompactionThresholds struct {
	MicroTriggerTokens          int
	MicroTargetTokens           int
	MicroMinReductionTokens     int
	PreserveRecentUserMessages  int
	PreserveRecentTotalMessages int
}

type Middleware interface {
	Project(ctx context.Context, input ProjectionInput) (ProjectionOutput, error)
}

type MiddlewareConfig struct {
	Config         Config
	Store          Store
	FileWriter     DetailFileWriter
	Counter        TokenCounter
	FullSummarizer FullSummarizer
}

type ContextCompactMiddleware struct {
	config     Config
	store      Store
	fileWriter DetailFileWriter
	counter    TokenCounter
	summarizer FullSummarizer
}

type ProjectionInput struct {
	WorkspaceID       pgtype.UUID
	ThreadID          pgtype.UUID
	TaskID            pgtype.UUID
	Role              string
	ModelID           string
	Messages          []*schema.Message
	MessageRefs       []SourceMessageRef
	ToolInfos         []*schema.ToolInfo
	MediaCards        []MediaCard
	Facts             []FullSummaryFact
	Trigger           string
	SameTurnFromIndex int
	PendingFromIndex  int
	ForceFullCompact  bool
}

type SourceMessageRef struct {
	MessageIndex int
	MessageID    pgtype.UUID
}

type ProjectionOutput struct {
	Messages          []*schema.Message
	Applied           []CompactionRecord
	TokenBefore       int
	TokenAfter        int
	OriginalUnchanged bool
	DetailFiles       []string
	CompactionRefs    []string
	CompactionMode    string
}

func NewMiddleware(config MiddlewareConfig) *ContextCompactMiddleware {
	counter := config.Counter
	if counter == nil {
		counter = NewTokenCounter()
	}
	return &ContextCompactMiddleware{
		config:     config.Config.WithDefaults(),
		store:      config.Store,
		fileWriter: config.FileWriter,
		counter:    counter,
		summarizer: config.FullSummarizer,
	}
}

func (m *ContextCompactMiddleware) Project(ctx context.Context, input ProjectionInput) (ProjectionOutput, error) {
	if m == nil {
		return ProjectionOutput{Messages: cloneMessages(input.Messages), OriginalUnchanged: true}, nil
	}
	messages := cloneMessages(input.Messages)
	before, err := m.counter.CountMessages(ctx, CountMessagesInput{ModelID: input.ModelID, Messages: input.Messages, ToolInfos: input.ToolInfos, MediaCards: input.MediaCards})
	if err != nil {
		return ProjectionOutput{}, err
	}
	out := ProjectionOutput{
		Messages:          messages,
		TokenBefore:       before.TotalTokens,
		TokenAfter:        before.TotalTokens,
		OriginalUnchanged: messagesUnchanged(input.Messages, messages),
	}
	if !m.config.Enabled || before.TotalTokens < m.config.MicroTriggerTokens {
		return out, nil
	}
	if m.store == nil || m.fileWriter == nil {
		return ProjectionOutput{}, errors.New("context compaction middleware is not configured")
	}
	protected := protectedMessageIndexes(input, m.config)
	for _, candidate := range candidatesForMessages(input.Messages) {
		if protected[candidate.MessageIndex] {
			continue
		}
		if candidate.EstimatedSavings < m.config.MicroMinReductionTokens {
			continue
		}
		record, err := m.compactMessage(ctx, input, candidate, messages[candidate.MessageIndex])
		if err != nil {
			return ProjectionOutput{}, err
		}
		messages[candidate.MessageIndex].Content = compactPlaceholder(record)
		out.Applied = append(out.Applied, record)
		out.CompactionRefs = append(out.CompactionRefs, record.SemanticKey)
		out.DetailFiles = append(out.DetailFiles, record.DetailFiles...)
		after, err := m.counter.CountMessages(ctx, CountMessagesInput{ModelID: input.ModelID, Messages: messages, ToolInfos: input.ToolInfos, MediaCards: input.MediaCards})
		if err != nil {
			return ProjectionOutput{}, err
		}
		out.TokenAfter = after.TotalTokens
		out.CompactionMode = "micro"
		if out.TokenAfter <= m.config.MicroTargetTokens {
			break
		}
	}
	if input.ForceFullCompact || out.TokenAfter >= m.config.FullTriggerTokens {
		full, err := m.fullCompact(ctx, input, messages, out)
		if err != nil {
			return ProjectionOutput{}, err
		}
		return full, nil
	}
	out.OriginalUnchanged = messagesUnchanged(input.Messages, messages)
	return out, nil
}

func (m *ContextCompactMiddleware) fullCompact(ctx context.Context, input ProjectionInput, messages []*schema.Message, current ProjectionOutput) (ProjectionOutput, error) {
	if m.summarizer == nil {
		return ProjectionOutput{}, errors.New("full context compaction summarizer is not configured")
	}
	protected := protectedMessageIndexes(input, m.config)
	compactedStart, compactedEnd := compactedRange(messages, protected)
	recentUserInstructions := recentUserInstructions(input.Messages, m.config.PreserveRecentUserMessages)
	summaryInput := FullSummaryInput{
		Role:                   strings.TrimSpace(input.Role),
		ModelID:                input.ModelID,
		Messages:               messagesInRange(messages, compactedStart, compactedEnd),
		Facts:                  append([]FullSummaryFact(nil), input.Facts...),
		MediaCards:             append([]MediaCard(nil), input.MediaCards...),
		RecentUserInstructions: recentUserInstructions,
		RecoveryRefs:           append([]string(nil), current.CompactionRefs...),
	}
	summaryOut, err := m.summarizer.Summarize(ctx, FullSummaryInput{
		Role:                   summaryInput.Role,
		ModelID:                summaryInput.ModelID,
		Messages:               summaryInput.Messages,
		Facts:                  summaryInput.Facts,
		MediaCards:             summaryInput.MediaCards,
		RecentUserInstructions: summaryInput.RecentUserInstructions,
		RecoveryRefs:           summaryInput.RecoveryRefs,
	})
	if err != nil {
		if !errors.Is(err, ErrInvalidFullSummary) {
			return ProjectionOutput{}, err
		}
		summaryOut = FullSummaryOutput{
			Summary: BuildFallbackFullSummary(summaryInput),
			ModelID: strings.TrimSpace(summaryOut.ModelID),
		}
	}
	summary := strings.TrimSpace(summaryOut.Summary)
	if err := ValidateFullSummaryMarkdown(summary); err != nil {
		if !errors.Is(err, ErrInvalidFullSummary) {
			return ProjectionOutput{}, err
		}
		summary = BuildFallbackFullSummary(summaryInput)
	}
	detail, err := m.fileWriter.WriteDetailFile(ctx, DetailFileInput{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        input.Role,
		SeqStart:    int64(compactedStart),
		SeqEnd:      int64(compactedEnd),
		MessageIDs:  sourceMessageIDsForRange(input.MessageRefs, compactedStart, compactedEnd),
		ToolName:    "full_compact_summary",
		Original:    summary,
	})
	if err != nil {
		return ProjectionOutput{}, err
	}
	semanticKey := fmt.Sprintf("ctxcmp:%s:%s:full:%d-%d:%s", safePathPart(input.Role), uuidKey(input.ThreadID), compactedStart, compactedEnd, detail.Hash[:16])
	record, err := m.store.CreateCompaction(ctx, CreateCompactionInput{
		WorkspaceID:            input.WorkspaceID,
		ThreadID:               input.ThreadID,
		TaskID:                 input.TaskID,
		Role:                   strings.TrimSpace(input.Role),
		Mode:                   "full",
		Trigger:                defaultTrigger(input.Trigger),
		SemanticKey:            semanticKey,
		SourceSeqStart:         int64(compactedStart),
		SourceSeqEnd:           int64(compactedEnd),
		SourceMessageIDs:       sourceMessageIDsForRange(input.MessageRefs, compactedStart, compactedEnd),
		OriginalTokenEstimate:  int64(current.TokenAfter),
		CompactedTokenEstimate: int64(heuristicTokens(summary)),
		OriginalBytes:          detail.Bytes,
		Summary:                summarizeFullSummary(summary),
		DetailFiles:            []string{detail.Path},
		Payload: map[string]any{
			"summary_model_id": strings.TrimSpace(summaryOut.ModelID),
			"hash":             detail.Hash,
			"recovery_refs":    current.CompactionRefs,
		},
	})
	if err != nil {
		return ProjectionOutput{}, err
	}
	for _, ref := range input.MessageRefs {
		if ref.MessageIndex < compactedStart || ref.MessageIndex > compactedEnd || !ref.MessageID.Valid {
			continue
		}
		if err := m.store.LinkMessage(ctx, LinkMessageInput{
			MessageID:     ref.MessageID,
			CompactionID:  record.ID,
			CompactedRole: strings.TrimSpace(input.Role),
		}); err != nil {
			return ProjectionOutput{}, err
		}
	}
	projected := rebuildFullCompactMessages(messages, protected, fullSummaryMessage(record, summary, detail.Path))
	after, err := m.counter.CountMessages(ctx, CountMessagesInput{ModelID: input.ModelID, Messages: projected, ToolInfos: input.ToolInfos, MediaCards: input.MediaCards})
	if err != nil {
		return ProjectionOutput{}, err
	}
	current.Messages = projected
	current.Applied = append(current.Applied, record)
	current.CompactionRefs = append(current.CompactionRefs, record.SemanticKey)
	current.DetailFiles = append(current.DetailFiles, record.DetailFiles...)
	current.TokenAfter = after.TotalTokens
	current.CompactionMode = "full"
	current.OriginalUnchanged = messagesUnchanged(input.Messages, projected)
	return current, nil
}

func (m *ContextCompactMiddleware) compactMessage(ctx context.Context, input ProjectionInput, candidate Candidate, msg *schema.Message) (CompactionRecord, error) {
	original := messageOriginalForCompaction(msg)
	seq := int64(candidate.MessageIndex)
	summary := summarizeForPlaceholder(original)
	messageIDs := sourceMessageIDsForIndex(input.MessageRefs, candidate.MessageIndex)
	detail, err := m.fileWriter.WriteDetailFile(ctx, DetailFileInput{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        input.Role,
		SeqStart:    seq,
		SeqEnd:      seq,
		MessageIDs:  messageIDs,
		ToolName:    msg.ToolName,
		ToolCallID:  msg.ToolCallID,
		Original:    original,
	})
	if err != nil {
		return CompactionRecord{}, err
	}
	semanticKey := fmt.Sprintf("ctxcmp:%s:%s:%d-%d:%s", safePathPart(input.Role), uuidKey(input.ThreadID), seq, seq, detail.Hash[:16])
	if existing, err := m.store.GetBySemanticKey(ctx, input.WorkspaceID, semanticKey); err == nil {
		if err := m.linkSourceMessages(ctx, input, candidate.MessageIndex, existing.ID); err != nil {
			return CompactionRecord{}, err
		}
		return existing, nil
	} else if !errors.Is(err, ErrCompactionNotFound) {
		return CompactionRecord{}, err
	}
	record, err := m.store.CreateCompaction(ctx, CreateCompactionInput{
		WorkspaceID:            input.WorkspaceID,
		ThreadID:               input.ThreadID,
		TaskID:                 input.TaskID,
		Role:                   strings.TrimSpace(input.Role),
		Mode:                   "micro",
		Trigger:                defaultTrigger(input.Trigger),
		SemanticKey:            semanticKey,
		SourceSeqStart:         seq,
		SourceSeqEnd:           seq,
		SourceMessageIDs:       messageIDs,
		OriginalTokenEstimate:  int64(candidate.OriginalTokenEstimate),
		CompactedTokenEstimate: int64(heuristicTokens(compactPlaceholderText(semanticKey, summary, detail.Path))),
		OriginalBytes:          detail.Bytes,
		Summary:                summary,
		DetailFiles:            []string{detail.Path},
		Payload: map[string]any{
			"tool_name":    msg.ToolName,
			"tool_call_id": msg.ToolCallID,
			"reason":       candidate.Reason,
			"hash":         detail.Hash,
		},
	})
	if err != nil {
		return CompactionRecord{}, err
	}
	if err := m.linkSourceMessages(ctx, input, candidate.MessageIndex, record.ID); err != nil {
		return CompactionRecord{}, err
	}
	return record, nil
}

func (m *ContextCompactMiddleware) linkSourceMessages(ctx context.Context, input ProjectionInput, messageIndex int, compactionID pgtype.UUID) error {
	for _, ref := range input.MessageRefs {
		if ref.MessageIndex != messageIndex || !ref.MessageID.Valid {
			continue
		}
		if err := m.store.LinkMessage(ctx, LinkMessageInput{
			MessageID:     ref.MessageID,
			CompactionID:  compactionID,
			CompactedRole: strings.TrimSpace(input.Role),
		}); err != nil {
			return err
		}
	}
	return nil
}

func compactedRange(messages []*schema.Message, protected map[int]bool) (int, int) {
	start := 1
	if len(messages) <= 1 {
		return 0, 0
	}
	end := len(messages) - 1
	for end >= start && protected[end] {
		end--
	}
	if end < start {
		return start, start
	}
	return start, end
}

func messagesInRange(messages []*schema.Message, start int, end int) []*schema.Message {
	if len(messages) == 0 || end < start {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if end >= len(messages) {
		end = len(messages) - 1
	}
	out := make([]*schema.Message, 0, end-start+1)
	for i := start; i <= end; i++ {
		out = append(out, cloneMessage(messages[i]))
	}
	return out
}

func rebuildFullCompactMessages(messages []*schema.Message, protected map[int]bool, summary *schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return []*schema.Message{summary}
	}
	out := []*schema.Message{cloneMessage(messages[0]), summary}
	for index := 1; index < len(messages); index++ {
		if protected[index] {
			out = append(out, cloneMessage(messages[index]))
		}
	}
	return out
}

func fullSummaryMessage(record CompactionRecord, summary string, detailFile string) *schema.Message {
	text := strings.TrimSpace(fmt.Sprintf(`Compacted handoff summary.
compact_ref: %s
detail_file: %s

%s

Recovery: use search_agent_history(compact_ref=...) or read_file(path=detail_file) when exact historical evidence is needed.`, record.SemanticKey, detailFile, strings.TrimSpace(summary)))
	return schema.UserMessage(text)
}

func recentUserInstructions(messages []*schema.Message, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for i := len(messages) - 1; i >= 0 && len(out) < limit; i-- {
		if messages[i] == nil || messages[i].Role != schema.User {
			continue
		}
		text := strings.TrimSpace(messageText(messages[i]))
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func summarizeFullSummary(summary string) string {
	summary = strings.Join(strings.Fields(strings.TrimSpace(summary)), " ")
	if len([]rune(summary)) <= 500 {
		return summary
	}
	return string([]rune(summary)[:500])
}

func candidatesForMessages(messages []*schema.Message) []Candidate {
	candidates := make([]Candidate, 0)
	for i, msg := range messages {
		if msg == nil || !isCandidateRole(msg.Role) {
			continue
		}
		text := messageOriginalForCompaction(msg)
		if len([]rune(strings.TrimSpace(text))) < 1000 {
			continue
		}
		tokens := heuristicTokens(text)
		savings := tokens - 64
		if savings < 1 {
			savings = 1
		}
		candidates = append(candidates, Candidate{
			MessageIndex:          i,
			Role:                  string(msg.Role),
			OriginalTokenEstimate: tokens,
			EstimatedSavings:      savings,
			Reason:                "old long tool or assistant result is recoverable through context compaction",
		})
	}
	return candidates
}

func protectedMessageIndexes(input ProjectionInput, config Config) map[int]bool {
	protected := map[int]bool{}
	if len(input.Messages) == 0 {
		return protected
	}
	protected[0] = true
	if input.SameTurnFromIndex > 0 {
		for i := input.SameTurnFromIndex; i < len(input.Messages); i++ {
			protected[i] = true
		}
	}
	if input.PendingFromIndex > 0 {
		for i := input.PendingFromIndex; i < len(input.Messages); i++ {
			protected[i] = true
		}
	}
	if config.PreserveRecentTotalMessages > 0 {
		start := len(input.Messages) - config.PreserveRecentTotalMessages
		if start < 0 {
			start = 0
		}
		for i := start; i < len(input.Messages); i++ {
			protected[i] = true
		}
	}
	if config.PreserveRecentUserMessages > 0 {
		remaining := config.PreserveRecentUserMessages
		for i := len(input.Messages) - 1; i >= 0 && remaining > 0; i-- {
			if input.Messages[i] != nil && input.Messages[i].Role == schema.User {
				protected[i] = true
				remaining--
			}
		}
	}
	return protected
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, cloneMessage(msg))
	}
	return out
}

func cloneMessage(msg *schema.Message) *schema.Message {
	if msg == nil {
		return nil
	}
	next := *msg
	if len(msg.ToolCalls) > 0 {
		next.ToolCalls = append([]schema.ToolCall(nil), msg.ToolCalls...)
	}
	if len(msg.MultiContent) > 0 {
		next.MultiContent = slices.Clone(msg.MultiContent)
	}
	if len(msg.UserInputMultiContent) > 0 {
		next.UserInputMultiContent = slices.Clone(msg.UserInputMultiContent)
	}
	return &next
}

func messagesUnchanged(original []*schema.Message, projected []*schema.Message) bool {
	for i, msg := range original {
		if msg == nil {
			continue
		}
		if i >= len(projected) {
			return false
		}
		if msg == projected[i] {
			return false
		}
	}
	return true
}

func messageOriginalForCompaction(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	return msg.Content
}

func compactPlaceholder(record CompactionRecord) string {
	path := ""
	if len(record.DetailFiles) > 0 {
		path = record.DetailFiles[0]
	}
	return compactPlaceholderText(record.SemanticKey, record.Summary, path)
}

func compactPlaceholderText(ref string, summary string, detailFile string) string {
	return strings.TrimSpace(fmt.Sprintf(`历史工具结果已压缩。
compact_ref: %s
summary: %s
detail_file: %s
恢复方式: 使用 read_file 读取 detail_file；不知道路径时用 search_agent_history(compact_ref=...)。`, ref, summary, detailFile))
}

func summarizeForPlaceholder(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return "原始工具结果为空。"
	}
	return fmt.Sprintf("原始工具结果较长，已归档为 detail file；约 %d bytes。", len([]byte(text)))
}

func defaultTrigger(trigger string) string {
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		return "micro_threshold"
	}
	return trigger
}

func uuidKey(id pgtype.UUID) string {
	if !id.Valid {
		return "none"
	}
	return fmt.Sprintf("%x", id.Bytes)
}

func sourceMessageIDsForIndex(refs []SourceMessageRef, index int) []string {
	out := make([]string, 0, 1)
	for _, ref := range refs {
		if ref.MessageIndex == index && ref.MessageID.Valid {
			out = append(out, uuidKey(ref.MessageID))
		}
	}
	return out
}

func sourceMessageIDsForRange(refs []SourceMessageRef, start int, end int) []string {
	out := make([]string, 0)
	for _, ref := range refs {
		if ref.MessageIndex >= start && ref.MessageIndex <= end && ref.MessageID.Valid {
			out = append(out, uuidKey(ref.MessageID))
		}
	}
	return out
}
