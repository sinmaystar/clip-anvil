package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/sandbox"
)

const (
	toolReadFile = "read_file"
	toolEditFile = "edit_file"

	defaultReadFileLimit = int64(20000)
	maxReadFileLimit     = int64(200000)
)

type SandboxEnsurer interface {
	EnsureSandbox(ctx context.Context, workspaceID pgtype.UUID) (sandbox.WorkspaceSandbox, error)
}

type ReadFileNativeTool struct {
	manager SandboxEnsurer
	client  sandbox.Client
}

type EditFileNativeTool struct {
	manager SandboxEnsurer
	client  sandbox.Client
}

type ReadFileInput struct {
	Path   string `json:"path" jsonschema:"required" jsonschema_description:"要读取的 sandbox 文本文件路径，必须位于 /workspace 内。"`
	Offset int64  `json:"offset,omitempty" jsonschema_description:"从哪个 byte offset 开始读取。默认 0。"`
	Limit  int64  `json:"limit,omitempty" jsonschema_description:"最多返回多少 bytes。默认 20000，最大 200000。"`
}

type EditFileInput struct {
	Path    string `json:"path" jsonschema:"required" jsonschema_description:"要写入或编辑的 sandbox 文本文件路径，必须位于 /workspace 内。"`
	Mode    string `json:"mode" jsonschema:"required,enum=create,enum=create_or_overwrite,enum=append,enum=replace" jsonschema_description:"编辑模式：create、create_or_overwrite、append、replace。"`
	Content string `json:"content,omitempty" jsonschema_description:"create、create_or_overwrite、append 使用的文本内容。"`
	OldText string `json:"old_text,omitempty" jsonschema_description:"replace 模式要替换的旧文本，必须唯一匹配。"`
	NewText string `json:"new_text,omitempty" jsonschema_description:"replace 模式的新文本。"`
	Reason  string `json:"reason,omitempty" jsonschema_description:"为什么要写入或编辑该文件，用于审计。"`
}

type readFileOutput struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Offset     int64  `json:"offset"`
	Limit      int64  `json:"limit"`
	BytesTotal int64  `json:"bytes_total"`
	NextOffset int64  `json:"next_offset,omitempty"`
	Truncated  bool   `json:"truncated"`
}

type editFileOutput struct {
	Path         string `json:"path"`
	Mode         string `json:"mode"`
	BytesWritten int64  `json:"bytes_written"`
	Reason       string `json:"reason,omitempty"`
}

func NewReadFileNativeTool(manager SandboxEnsurer, client sandbox.Client) *ReadFileNativeTool {
	return &ReadFileNativeTool{manager: manager, client: client}
}

func NewEditFileNativeTool(manager SandboxEnsurer, client sandbox.Client) *EditFileNativeTool {
	return &EditFileNativeTool{manager: manager, client: client}
}

func (t *ReadFileNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[ReadFileInput](
		toolReadFile,
		"读取当前 workspace sandbox 内的 UTF-8 文本文件。只读 /workspace 内路径；目录枚举请使用 exec shell 的 ls/find/grep/rg；业务事实仍必须通过结构化工具落库。",
	)
}

func (t *ReadFileNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolReadFile, raw, validateReadFileInput)
	if !ok {
		return msg, nil
	}
	if input.Path, msg, ok = normalizeSandboxFileToolPath(toolReadFile, input.Path); !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolReadFile)
	if !ok {
		return msg, nil
	}
	workspaceSandbox, msg, ok := t.ensureSandbox(ctx, runtime.WorkspaceID, toolReadFile)
	if !ok {
		return msg, nil
	}
	file, info, err := t.client.Download(ctx, workspaceSandbox.SandboxID, input.Path)
	if err != nil {
		return NaturalToolError(toolReadFile, "读取 sandbox 文件失败："+err.Error(), "请确认 path 存在于当前 workspace sandbox 内，必要时先用 exec shell 查看文件。"), nil
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		return NaturalToolError(toolReadFile, "读取 sandbox 文件内容失败："+err.Error(), "请稍后重试，或检查 sandbox 文件是否可读。"), nil
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	limit := normalizeReadLimit(input.Limit)
	total := int64(len(data))
	if info.SizeBytes > total {
		total = info.SizeBytes
	}
	start := minInt64(offset, int64(len(data)))
	end := minInt64(start+limit, int64(len(data)))
	truncated := end < int64(len(data))
	out := readFileOutput{
		Path:       input.Path,
		Content:    string(data[start:end]),
		Offset:     start,
		Limit:      limit,
		BytesTotal: total,
		Truncated:  truncated,
	}
	if truncated {
		out.NextOffset = end
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (t *EditFileNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[EditFileInput](
		toolEditFile,
		"创建、覆盖、追加或局部替换当前 workspace sandbox 内的 UTF-8 文本文件。只写 /workspace 内路径；目录枚举请使用 exec shell；业务事实仍必须通过结构化工具落库。",
	)
}

func (t *EditFileNativeTool) InvokableRun(ctx context.Context, raw string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolEditFile, raw, validateEditFileInput)
	if !ok {
		return msg, nil
	}
	if input.Path, msg, ok = normalizeSandboxFileToolPath(toolEditFile, input.Path); !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolEditFile)
	if !ok {
		return msg, nil
	}
	workspaceSandbox, msg, ok := t.ensureSandbox(ctx, runtime.WorkspaceID, toolEditFile)
	if !ok {
		return msg, nil
	}
	existing, exists, msg, ok := t.readExisting(ctx, workspaceSandbox.SandboxID, input.Path, input.Mode)
	if !ok {
		return msg, nil
	}
	if input.Mode == "create" && exists {
		return NaturalToolError(toolEditFile, "文件已存在，create 模式不会覆盖："+input.Path, "如需覆盖请使用 create_or_overwrite；如需保留原文请使用 append 或 replace。"), nil
	}
	next, err := sandbox.ApplyTextEdit(existing, sandbox.TextEditInput{
		Mode:    input.Mode,
		Content: input.Content,
		OldText: input.OldText,
		NewText: input.NewText,
	})
	if err != nil {
		return NaturalToolError(toolEditFile, err.Error(), "请修正 mode、old_text 或 content 后重试。"), nil
	}
	if err := ensureSandboxParentDir(ctx, t.client, workspaceSandbox.SandboxID, input.Path); err != nil {
		return NaturalToolError(toolEditFile, "创建 sandbox 父目录失败："+err.Error(), "请稍后重试，或检查 sandbox 是否可用。"), nil
	}
	if err := t.client.Upload(ctx, workspaceSandbox.SandboxID, input.Path, strings.NewReader(next)); err != nil {
		return NaturalToolError(toolEditFile, "写入 sandbox 文件失败："+err.Error(), "请稍后重试，或检查 sandbox 是否可用。"), nil
	}
	out, err := json.Marshal(editFileOutput{
		Path:         input.Path,
		Mode:         input.Mode,
		BytesWritten: int64(len([]byte(next))),
		Reason:       strings.TrimSpace(input.Reason),
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *ReadFileNativeTool) ensureSandbox(ctx context.Context, workspaceID pgtype.UUID, toolName string) (sandbox.WorkspaceSandbox, string, bool) {
	if t.manager == nil || t.client == nil {
		return sandbox.WorkspaceSandbox{}, NaturalToolError(toolName, "sandbox 文件工具未配置。", "请检查服务端 wiring 后重试。"), false
	}
	workspaceSandbox, err := t.manager.EnsureSandbox(ctx, workspaceID)
	if err != nil {
		return sandbox.WorkspaceSandbox{}, NaturalToolError(toolName, "确保 workspace sandbox 失败："+err.Error(), "请稍后重试，或让工程侧检查 OpenSandbox 状态。"), false
	}
	return workspaceSandbox, "", true
}

func (t *EditFileNativeTool) ensureSandbox(ctx context.Context, workspaceID pgtype.UUID, toolName string) (sandbox.WorkspaceSandbox, string, bool) {
	if t.manager == nil || t.client == nil {
		return sandbox.WorkspaceSandbox{}, NaturalToolError(toolName, "sandbox 文件工具未配置。", "请检查服务端 wiring 后重试。"), false
	}
	workspaceSandbox, err := t.manager.EnsureSandbox(ctx, workspaceID)
	if err != nil {
		return sandbox.WorkspaceSandbox{}, NaturalToolError(toolName, "确保 workspace sandbox 失败："+err.Error(), "请稍后重试，或让工程侧检查 OpenSandbox 状态。"), false
	}
	return workspaceSandbox, "", true
}

func (t *EditFileNativeTool) readExisting(ctx context.Context, sandboxID string, filePath string, mode string) (string, bool, string, bool) {
	if mode == "create_or_overwrite" {
		return "", false, "", true
	}
	file, _, err := t.client.Download(ctx, sandboxID, filePath)
	if err != nil {
		if mode == "replace" {
			return "", false, NaturalToolError(toolEditFile, "读取待替换文件失败："+err.Error(), "请确认 path 存在，或改用 create/create_or_overwrite。"), false
		}
		return "", false, "", true
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		return "", true, NaturalToolError(toolEditFile, "读取已有文件内容失败："+err.Error(), "请稍后重试，或检查 sandbox 文件是否可读。"), false
	}
	return string(data), true, "", true
}

func validateReadFileInput(input ReadFileInput) error {
	if _, err := sandbox.ValidateWorkspaceTextPath(input.Path); err != nil {
		return err
	}
	if input.Limit < 0 {
		return errors.New("limit must be >= 0")
	}
	if input.Offset < 0 {
		return errors.New("offset must be >= 0")
	}
	return nil
}

func normalizeSandboxFileToolPath(toolName string, value string) (string, string, bool) {
	clean, err := sandbox.ValidateWorkspaceTextPath(value)
	if err != nil {
		return "", NaturalToolError(toolName, err.Error(), "请使用 /workspace 内的文本文件路径。"), false
	}
	return clean, "", true
}

func validateEditFileInput(input EditFileInput) error {
	if _, err := sandbox.ValidateWorkspaceTextPath(input.Path); err != nil {
		return err
	}
	return requireMode(input.Mode, "create", "create_or_overwrite", "append", "replace")
}

func normalizeReadLimit(limit int64) int64 {
	if limit <= 0 {
		return defaultReadFileLimit
	}
	if limit > maxReadFileLimit {
		return maxReadFileLimit
	}
	return limit
}

func ensureSandboxParentDir(ctx context.Context, client sandbox.Client, sandboxID string, filePath string) error {
	dir := path.Dir(filePath)
	if dir == "." || dir == "/" || dir == sandbox.DefaultWorkdir {
		return nil
	}
	_, err := sandbox.RunExec(ctx, client, sandboxID, sandbox.ExecInput{
		Command:        "mkdir -p " + shellQuoteForSandboxFile(dir),
		Cwd:            sandbox.DefaultWorkdir,
		TimeoutSeconds: 30,
	})
	return err
}

func shellQuoteForSandboxFile(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func minInt64(a int64, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
