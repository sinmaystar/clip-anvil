package contextcompact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

const detailFileDir = "/workspace/.clipanvil/context"

type ContextSandbox struct {
	SandboxID  string
	VolumeName string
}

type ContextSandboxEnsurer interface {
	EnsureContextSandbox(ctx context.Context, workspaceID pgtype.UUID) (ContextSandbox, error)
}

type ContextSandboxFileClient interface {
	Exec(ctx context.Context, sandboxID string, command string, cwd string, timeoutSeconds int) error
	Upload(ctx context.Context, sandboxID string, path string, reader io.Reader) error
	Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, error)
}

type DetailFileWriter interface {
	WriteDetailFile(ctx context.Context, input DetailFileInput) (DetailFileResult, error)
}

type DetailFileInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	Role        string
	SeqStart    int64
	SeqEnd      int64
	MessageIDs  []string
	ToolName    string
	ToolCallID  string
	Original    string
}

type DetailFileResult struct {
	Path       string
	Hash       string
	Bytes      int64
	Reused     bool
	SandboxID  string
	VolumeName string
}

type SandboxDetailFileWriter struct {
	manager ContextSandboxEnsurer
	client  ContextSandboxFileClient
}

func NewSandboxDetailFileWriter(manager ContextSandboxEnsurer, client ContextSandboxFileClient) *SandboxDetailFileWriter {
	return &SandboxDetailFileWriter{manager: manager, client: client}
}

func (w *SandboxDetailFileWriter) WriteDetailFile(ctx context.Context, input DetailFileInput) (DetailFileResult, error) {
	if w == nil || w.manager == nil || w.client == nil {
		return DetailFileResult{}, fmt.Errorf("detail file writer is not configured")
	}
	workspaceSandbox, err := w.manager.EnsureContextSandbox(ctx, input.WorkspaceID)
	if err != nil {
		return DetailFileResult{}, fmt.Errorf("ensure workspace sandbox: %w", err)
	}
	result := detailFileResult(input)
	result.SandboxID = workspaceSandbox.SandboxID
	result.VolumeName = workspaceSandbox.VolumeName
	if existing, err := w.client.Download(ctx, workspaceSandbox.SandboxID, result.Path); err == nil {
		defer func() { _ = existing.Close() }()
		data, readErr := io.ReadAll(existing)
		if readErr == nil && strings.Contains(string(data), "sha256: "+result.Hash) {
			result.Reused = true
			return result, nil
		}
	}
	if err := w.client.Exec(ctx, workspaceSandbox.SandboxID, "mkdir -p "+detailFileDir, "/workspace", 30); err != nil {
		return DetailFileResult{}, fmt.Errorf("create context detail dir: %w", err)
	}
	if err := w.client.Upload(ctx, workspaceSandbox.SandboxID, result.Path, strings.NewReader(detailFileContent(input, result.Hash))); err != nil {
		return DetailFileResult{}, fmt.Errorf("upload detail file: %w", err)
	}
	return result, nil
}

func detailFileResult(input DetailFileInput) DetailFileResult {
	hash := sha256.Sum256([]byte(input.Original))
	encoded := hex.EncodeToString(hash[:])
	short := encoded
	if len(short) > 16 {
		short = short[:16]
	}
	role := safePathPart(input.Role)
	if role == "" {
		role = "agent"
	}
	return DetailFileResult{
		Path:  fmt.Sprintf("%s/%s-%d-%d-%s.md", detailFileDir, role, input.SeqStart, input.SeqEnd, short),
		Hash:  encoded,
		Bytes: int64(len([]byte(input.Original))),
	}
}

func detailFileContent(input DetailFileInput, hash string) string {
	var b strings.Builder
	b.WriteString("# Agent Context Detail\n\n")
	b.WriteString("role: " + strings.TrimSpace(input.Role) + "\n")
	fmt.Fprintf(&b, "seq_start: %d\n", input.SeqStart)
	fmt.Fprintf(&b, "seq_end: %d\n", input.SeqEnd)
	if input.ToolName != "" {
		b.WriteString("tool_name: " + strings.TrimSpace(input.ToolName) + "\n")
	}
	if input.ToolCallID != "" {
		b.WriteString("tool_call_id: " + strings.TrimSpace(input.ToolCallID) + "\n")
	}
	if len(input.MessageIDs) > 0 {
		b.WriteString("message_ids: " + strings.Join(input.MessageIDs, ",") + "\n")
	}
	b.WriteString("sha256: " + hash + "\n")
	fmt.Fprintf(&b, "bytes: %d\n\n", len([]byte(input.Original)))
	b.WriteString("## Original\n\n")
	b.WriteString(input.Original)
	if !strings.HasSuffix(input.Original, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

func safePathPart(input string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(input)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
