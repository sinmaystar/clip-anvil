package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

const toolSearchAgentHistory = "search_agent_history"

type SearchAgentHistoryNativeTool struct {
	store  contextcompact.Store
	config contextcompact.Config
}

type SearchAgentHistoryInput struct {
	CompactRef string `json:"compact_ref,omitempty" jsonschema_description:"精确恢复某条压缩记录的 compact_ref。"`
	Query      string `json:"query,omitempty" jsonschema_description:"按摘要、payload、media_ref 搜索历史压缩记录。"`
	MediaRef   string `json:"media_ref,omitempty" jsonschema_description:"按媒体引用搜索相关历史压缩记录。"`
	ThreadID   string `json:"thread_id,omitempty" jsonschema_description:"限定搜索的 Agent thread id；默认使用当前运行时 thread。"`
	Limit      int32  `json:"limit,omitempty" jsonschema_description:"最多返回多少条，默认使用 context compaction 配置。"`
}

type searchAgentHistoryOutput struct {
	Matches []searchAgentHistoryMatch `json:"matches"`
}

type searchAgentHistoryMatch struct {
	CompactRef       string   `json:"compact_ref"`
	Mode             string   `json:"mode"`
	Summary          string   `json:"summary"`
	DetailFiles      []string `json:"detail_files"`
	SourceMessageIDs []string `json:"source_message_ids"`
	SourceSeqStart   int64    `json:"source_seq_start"`
	SourceSeqEnd     int64    `json:"source_seq_end"`
	SourceMediaRefs  []string `json:"source_media_refs"`
	CreatedAt        string   `json:"created_at,omitempty"`
	Excerpt          string   `json:"excerpt,omitempty"`
}

func NewSearchAgentHistoryNativeTool(store contextcompact.Store, config contextcompact.Config) *SearchAgentHistoryNativeTool {
	return &SearchAgentHistoryNativeTool{store: store, config: config.WithDefaults()}
}

func (t *SearchAgentHistoryNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[SearchAgentHistoryInput](
		toolSearchAgentHistory,
		"搜索被上下文压缩归档的 Agent 历史记录。返回 compact_ref、summary 和 detail_file；需要完整原文时继续用 read_file 读取 detail_file。",
	)
}

func (t *SearchAgentHistoryNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolSearchAgentHistory, raw, validateSearchAgentHistoryInput)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolSearchAgentHistory)
	if !ok {
		return msg, nil
	}
	if t == nil || t.store == nil {
		return NaturalToolError(toolSearchAgentHistory, "context compaction store 未配置。", "请检查服务端 wiring 后重试。"), nil
	}
	var records []contextcompact.CompactionRecord
	if compactRef := strings.TrimSpace(input.CompactRef); compactRef != "" {
		record, err := t.store.GetBySemanticKey(ctx, runtime.WorkspaceID, compactRef)
		if errors.Is(err, contextcompact.ErrCompactionNotFound) {
			records = nil
		} else if err != nil {
			return NaturalToolError(toolSearchAgentHistory, err.Error(), "请稍后重试，或确认 compact_ref 来自当前 workspace。"), nil
		} else {
			records = []contextcompact.CompactionRecord{record}
		}
	} else {
		threadID := runtime.ThreadID
		if strings.TrimSpace(input.ThreadID) != "" {
			parsed, ok := pgUUIDFromString(input.ThreadID)
			if !ok {
				return NaturalToolError(toolSearchAgentHistory, "thread_id 不是合法 UUID。", "请省略 thread_id 使用当前线程，或复制真实 thread_id。"), nil
			}
			threadID = parsed
		}
		query := strings.TrimSpace(input.Query)
		if mediaRef := strings.TrimSpace(input.MediaRef); mediaRef != "" {
			query = mediaRef
		}
		found, err := t.store.Search(ctx, contextcompact.SearchInput{
			WorkspaceID: runtime.WorkspaceID,
			ThreadID:    threadID,
			Query:       query,
			Limit:       searchHistoryLimit(input.Limit, t.config.SearchMaxResults),
		})
		if err != nil {
			return NaturalToolError(toolSearchAgentHistory, err.Error(), "请缩小 query 或稍后重试。"), nil
		}
		records = found
	}
	out := searchAgentHistoryOutput{Matches: make([]searchAgentHistoryMatch, 0, len(records))}
	for _, record := range records {
		out.Matches = append(out.Matches, searchMatchFromRecord(record))
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func validateSearchAgentHistoryInput(input SearchAgentHistoryInput) error {
	if strings.TrimSpace(input.CompactRef) == "" && strings.TrimSpace(input.Query) == "" && strings.TrimSpace(input.MediaRef) == "" {
		return errors.New("compact_ref、query 或 media_ref 至少需要一个")
	}
	if input.Limit < 0 {
		return errors.New("limit must be >= 0")
	}
	return nil
}

func searchHistoryLimit(requested int32, defaultLimit int) int32 {
	if requested > 0 {
		if requested > 100 {
			return 100
		}
		return requested
	}
	if defaultLimit <= 0 {
		return 20
	}
	if defaultLimit > 100 {
		return 100
	}
	return int32(defaultLimit)
}

func searchMatchFromRecord(record contextcompact.CompactionRecord) searchAgentHistoryMatch {
	createdAt := ""
	if record.CreatedAt.Valid {
		createdAt = record.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
	}
	return searchAgentHistoryMatch{
		CompactRef:       record.SemanticKey,
		Mode:             record.Mode,
		Summary:          record.Summary,
		DetailFiles:      append([]string(nil), record.DetailFiles...),
		SourceMessageIDs: append([]string(nil), record.SourceMessageIDs...),
		SourceSeqStart:   record.SourceSeqStart,
		SourceSeqEnd:     record.SourceSeqEnd,
		SourceMediaRefs:  append([]string(nil), record.SourceMediaRefs...),
		CreatedAt:        createdAt,
		Excerpt:          compactExcerpt(record.Summary),
	}
}

func compactExcerpt(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if len(runes) > 120 {
		return string(runes[:120]) + "..."
	}
	return text
}
