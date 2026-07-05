package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestAgentRemotionAttemptDirBuildsStableWorkspacePath(t *testing.T) {
	got, err := AgentRemotionAttemptDir("3f2f72c8-7ac7-4e30-b2f1-e0f7d61ef111", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/agent-remotion/3f2f72c8-7ac7-4e30-b2f1-e0f7d61ef111/2" {
		t.Fatalf("dir = %q", got)
	}
}

func TestAgentRemotionAttemptDirRejectsUnsafeSegments(t *testing.T) {
	for _, input := range []string{"", "../x", "x/y", ".hidden"} {
		if _, err := AgentRemotionAttemptDir(input, 1); err == nil {
			t.Fatalf("expected unsafe renderer id %q to fail", input)
		}
	}
	if _, err := AgentRemotionAttemptDir("artifact", 0); err == nil {
		t.Fatal("expected attempt_no <= 0 to fail")
	}
}

func TestNormalizeAgentRemotionRelativePathRejectsEscape(t *testing.T) {
	for _, input := range []string{"", "../x.tsx", "/workspace/x.tsx", "nested/../x.tsx", "node_modules/x.ts", "package.json"} {
		if _, err := NormalizeAgentRemotionRelativePath(input); err == nil {
			t.Fatalf("expected %q to fail", input)
		}
	}
}

func TestNormalizeAgentRemotionRelativePathAcceptsRendererFiles(t *testing.T) {
	for _, input := range []string{"GeneratedComposition.tsx", "styles.ts", "copy/content.json", "components/Card.tsx"} {
		got, err := NormalizeAgentRemotionRelativePath(input)
		if err != nil {
			t.Fatalf("NormalizeAgentRemotionRelativePath(%q) error = %v", input, err)
		}
		if got != input {
			t.Fatalf("path = %q, want %q", got, input)
		}
	}
}

func TestBuildAgentRemotionSnapshotHashesSortedFilesAndProps(t *testing.T) {
	snapshot, err := BuildAgentRemotionSnapshot("/workspace/agent-remotion/artifact/1", map[string]string{
		"GeneratedComposition.tsx": `import React from "react"; export default function Video(){ return <div/>; }`,
		"styles.ts":                `export const color = "#fff";`,
	}, []byte(`{"output":{"fps":30,"duration_sec":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SourceHash == "" || snapshot.PropsHash == "" {
		t.Fatalf("missing hashes: %#v", snapshot)
	}
	if len(snapshot.Files) != 2 || snapshot.Files[0].Path != "GeneratedComposition.tsx" || snapshot.Files[1].Path != "styles.ts" {
		t.Fatalf("files not sorted: %#v", snapshot.Files)
	}
	if snapshot.SourceSnapshot["file_count"] != 2 {
		t.Fatalf("source snapshot = %#v", snapshot.SourceSnapshot)
	}
}

func TestBuildAgentRemotionSnapshotRejectsOversizedInputs(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < DefaultAgentRemotionMaxFiles+1; i++ {
		files[fmt.Sprintf("file%d.ts", i)] = "export const x = 1;"
	}
	if _, err := BuildAgentRemotionSnapshot("/workspace/agent-remotion/artifact/1", files, []byte(`{}`)); err == nil {
		t.Fatal("expected too many files to fail")
	}
	large := strings.Repeat("x", DefaultAgentRemotionMaxFileBytes+1)
	if _, err := BuildAgentRemotionSnapshot("/workspace/agent-remotion/artifact/1", map[string]string{"GeneratedComposition.tsx": large}, []byte(`{}`)); err == nil {
		t.Fatal("expected oversized file to fail")
	}
}

func TestWriteAgentRemotionAttemptWorkspaceCreatesDirAndUploadsFiles(t *testing.T) {
	client := newAgentRemotionWorkspaceFakeClient()
	snapshot, err := WriteAgentRemotionAttemptWorkspace(context.Background(), client, "sandbox-1", AgentRemotionWorkspaceInput{
		RendererArtifactID: "artifact-1",
		AttemptNo:          1,
		Files: map[string]string{
			"GeneratedComposition.tsx": `export default function Video(){ return null; }`,
			"styles.ts":                `export const color = "#fff";`,
		},
		PropsJSON: []byte(`{"output":{"fps":30}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.WorkspaceDir != "/workspace/agent-remotion/artifact-1/1" || snapshot.SourceHash == "" || snapshot.PropsHash == "" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(client.execRequests) != 1 || !strings.Contains(client.execRequests[0].Command, "mkdir -p") {
		t.Fatalf("exec requests = %#v", client.execRequests)
	}
	for _, want := range []string{
		"/workspace/agent-remotion/artifact-1/1/GeneratedComposition.tsx",
		"/workspace/agent-remotion/artifact-1/1/styles.ts",
		"/workspace/agent-remotion/artifact-1/1/props.json",
	} {
		if _, ok := client.uploads[want]; !ok {
			t.Fatalf("missing upload %q in %#v", want, client.uploads)
		}
	}
}

func TestReadAgentRemotionAttemptWorkspaceReadsSnapshot(t *testing.T) {
	client := newAgentRemotionWorkspaceFakeClient()
	client.execResult = ExecResult{
		Stdout: strings.Join([]string{
			"/workspace/agent-remotion/artifact-1/1/GeneratedComposition.tsx",
			"/workspace/agent-remotion/artifact-1/1/props.json",
			"/workspace/agent-remotion/artifact-1/1/styles.ts",
		}, "\n"),
	}
	client.downloads["/workspace/agent-remotion/artifact-1/1/GeneratedComposition.tsx"] = `export default function Video(){ return null; }`
	client.downloads["/workspace/agent-remotion/artifact-1/1/styles.ts"] = `export const color = "#fff";`
	client.downloads["/workspace/agent-remotion/artifact-1/1/props.json"] = `{"output":{"fps":30}}`

	snapshot, err := ReadAgentRemotionAttemptWorkspace(context.Background(), client, "sandbox-1", "/workspace/agent-remotion/artifact-1/1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 2 || snapshot.Files[0].Path != "GeneratedComposition.tsx" || string(snapshot.PropsJSON) != `{"output":{"fps":30}}` {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestReadAgentRemotionAttemptWorkspaceRejectsEscapedFindResult(t *testing.T) {
	client := newAgentRemotionWorkspaceFakeClient()
	client.execResult = ExecResult{Stdout: "/workspace/agent-remotion/artifact-1/1/../evil.ts\n"}
	if _, err := ReadAgentRemotionAttemptWorkspace(context.Background(), client, "sandbox-1", "/workspace/agent-remotion/artifact-1/1"); err == nil {
		t.Fatal("expected escaped find result to fail")
	}
}

type agentRemotionWorkspaceFakeClient struct {
	execRequests []ExecRequest
	execResult   ExecResult
	uploads      map[string]string
	downloads    map[string]string
}

func newAgentRemotionWorkspaceFakeClient() *agentRemotionWorkspaceFakeClient {
	return &agentRemotionWorkspaceFakeClient{
		uploads:   map[string]string{},
		downloads: map[string]string{},
	}
}

func (f *agentRemotionWorkspaceFakeClient) Create(context.Context, CreateRequest) (SandboxInfo, error) {
	return SandboxInfo{}, errors.New("create should not be called")
}

func (f *agentRemotionWorkspaceFakeClient) Get(context.Context, string) (SandboxInfo, error) {
	return SandboxInfo{}, errors.New("get should not be called")
}

func (f *agentRemotionWorkspaceFakeClient) Ping(context.Context, string) error {
	return errors.New("ping should not be called")
}

func (f *agentRemotionWorkspaceFakeClient) Exec(_ context.Context, _ string, req ExecRequest) (ExecResult, error) {
	f.execRequests = append(f.execRequests, req)
	return f.execResult, nil
}

func (f *agentRemotionWorkspaceFakeClient) Upload(_ context.Context, _ string, path string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.uploads[path] = string(data)
	return nil
}

func (f *agentRemotionWorkspaceFakeClient) Download(_ context.Context, _ string, path string) (io.ReadCloser, FileInfo, error) {
	data, ok := f.downloads[path]
	if !ok {
		return nil, FileInfo{}, errors.New("missing fake download")
	}
	return io.NopCloser(strings.NewReader(data)), FileInfo{Path: path, SizeBytes: int64(len(data))}, nil
}

func (f *agentRemotionWorkspaceFakeClient) Delete(context.Context, string) error {
	return errors.New("delete should not be called")
}
