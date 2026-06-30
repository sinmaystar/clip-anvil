package tools

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/sandbox"
)

func TestReadFileRequiresRuntimeWorkspace(t *testing.T) {
	tool := NewReadFileNativeTool(&fakeFileSandboxManager{}, &fakeFileSandboxClient{})
	got, err := tool.InvokableRun(context.Background(), `{"path":"/workspace/.clipanvil/notes/a.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "工具调用失败") {
		t.Fatalf("result = %q, want natural tool error", got)
	}
}

func TestReadFileReturnsChunkAndNextOffset(t *testing.T) {
	client := &fakeFileSandboxClient{files: map[string]string{"/workspace/.clipanvil/context/large.md": "abcdefghijklmnopqrstuvwxyz"}}
	tool := NewReadFileNativeTool(&fakeFileSandboxManager{}, client)
	got, err := tool.InvokableRun(fileToolContext(), `{"path":"/workspace/.clipanvil/context/large.md","offset":5,"limit":10}`)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Path       string `json:"path"`
		Content    string `json:"content"`
		Offset     int64  `json:"offset"`
		NextOffset int64  `json:"next_offset"`
		Truncated  bool   `json:"truncated"`
		BytesTotal int64  `json:"bytes_total"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal %q: %v", got, err)
	}
	if out.Path != "/workspace/.clipanvil/context/large.md" || out.Content != "fghijklmno" {
		t.Fatalf("unexpected read output: %#v", out)
	}
	if out.Offset != 5 || out.NextOffset != 15 || !out.Truncated || out.BytesTotal != 26 {
		t.Fatalf("unexpected chunk metadata: %#v", out)
	}
}

func TestEditFileCreateOrOverwriteUploadsContent(t *testing.T) {
	client := &fakeFileSandboxClient{files: map[string]string{}}
	tool := NewEditFileNativeTool(&fakeFileSandboxManager{}, client)
	got, err := tool.InvokableRun(fileToolContext(), `{"path":"/workspace/.clipanvil/notes/plan.md","mode":"create_or_overwrite","content":"hello","reason":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if client.files["/workspace/.clipanvil/notes/plan.md"] != "hello" {
		t.Fatalf("uploaded content = %q", client.files["/workspace/.clipanvil/notes/plan.md"])
	}
	if !strings.Contains(got, `"bytes_written":5`) {
		t.Fatalf("result missing bytes_written: %s", got)
	}
}

func TestEditFileAppendDownloadsThenUploads(t *testing.T) {
	client := &fakeFileSandboxClient{files: map[string]string{"/workspace/.clipanvil/notes/plan.md": "old"}}
	tool := NewEditFileNativeTool(&fakeFileSandboxManager{}, client)
	_, err := tool.InvokableRun(fileToolContext(), `{"path":"/workspace/.clipanvil/notes/plan.md","mode":"append","content":"\nnew"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := client.files["/workspace/.clipanvil/notes/plan.md"]; got != "old\nnew" {
		t.Fatalf("uploaded content = %q", got)
	}
}

func TestEditFileReplaceRejectsAmbiguousOldText(t *testing.T) {
	client := &fakeFileSandboxClient{files: map[string]string{"/workspace/.clipanvil/notes/plan.md": "one one"}}
	tool := NewEditFileNativeTool(&fakeFileSandboxManager{}, client)
	got, err := tool.InvokableRun(fileToolContext(), `{"path":"/workspace/.clipanvil/notes/plan.md","mode":"replace","old_text":"one","new_text":"two"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "old_text must match exactly once") {
		t.Fatalf("result = %q, want old_text uniqueness error", got)
	}
}

func TestSandboxFileToolInfosUseTypedSchemas(t *testing.T) {
	registry, err := NewNativeRegistry(
		NewReadFileNativeTool(nil, nil),
		NewEditFileNativeTool(nil, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := registry.ToolInfos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, info := range infos {
		got[info.Name] = info.ParamsOneOf != nil
	}
	for _, name := range []string{"read_file", "edit_file"} {
		if !got[name] {
			t.Fatalf("missing typed schema for %s in %#v", name, got)
		}
	}
}

func fileToolContext() context.Context {
	return WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: uuidWithByte(1)})
}

type fakeFileSandboxManager struct{}

func (fakeFileSandboxManager) EnsureSandbox(context.Context, pgtype.UUID) (sandbox.WorkspaceSandbox, error) {
	return sandbox.WorkspaceSandbox{SandboxID: "sandbox-1", VolumeName: "volume-1"}, nil
}

type fakeFileSandboxClient struct {
	files map[string]string
}

func (f *fakeFileSandboxClient) Create(context.Context, sandbox.CreateRequest) (sandbox.SandboxInfo, error) {
	return sandbox.SandboxInfo{}, nil
}

func (f *fakeFileSandboxClient) Get(context.Context, string) (sandbox.SandboxInfo, error) {
	return sandbox.SandboxInfo{}, nil
}

func (f *fakeFileSandboxClient) Ping(context.Context, string) error { return nil }

func (f *fakeFileSandboxClient) Exec(context.Context, string, sandbox.ExecRequest) (sandbox.ExecResult, error) {
	return sandbox.ExecResult{}, nil
}

func (f *fakeFileSandboxClient) Upload(_ context.Context, _ string, path string, r io.Reader) error {
	if f.files == nil {
		f.files = map[string]string{}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.files[path] = string(data)
	return nil
}

func (f *fakeFileSandboxClient) Download(_ context.Context, _ string, path string) (io.ReadCloser, sandbox.FileInfo, error) {
	content, ok := f.files[path]
	if !ok {
		return nil, sandbox.FileInfo{}, io.ErrUnexpectedEOF
	}
	return io.NopCloser(strings.NewReader(content)), sandbox.FileInfo{Path: path, SizeBytes: int64(len(content)), Mime: "text/plain"}, nil
}

func (f *fakeFileSandboxClient) Delete(context.Context, string) error { return nil }
