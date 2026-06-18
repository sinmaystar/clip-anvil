package sandbox

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestMediaTypeForArtifactMIME(t *testing.T) {
	if got, ok := MediaTypeForArtifactMIME("image/png"); !ok || got != "image" {
		t.Fatalf("image/png = %q, %v", got, ok)
	}
	if _, ok := MediaTypeForArtifactMIME("text/plain"); ok {
		t.Fatal("text/plain must not be accepted")
	}
}

func TestValidateArtifactSize(t *testing.T) {
	if err := ValidateArtifactSize(MaxArtifactBytes + 1); err == nil {
		t.Fatal("expected oversized artifact to fail")
	}
	if err := ValidateArtifactSize(1024); err != nil {
		t.Fatalf("small artifact failed: %v", err)
	}
}

func TestArtifactServiceCreatesAssetAndAgentNode(t *testing.T) {
	client := &artifactFakeClient{
		inspectStdout: "20\nimage/png",
	}
	repo := &artifactFakeRepository{}
	storage := &artifactFakeStorage{}
	broadcaster := &artifactFakeBroadcaster{}
	service := NewArtifactService(client, repo, storage, broadcaster)

	result, err := service.Submit(context.Background(), "sandbox-1", testWorkspaceID(), ArtifactInput{
		Path:  "/workspace/output/result.png",
		Title: "Result",
	})
	if err != nil {
		t.Fatalf("Submit error = %v", err)
	}
	if result.AssetID == "" || result.NodeID == "" {
		t.Fatalf("result ids must be set: %#v", result)
	}
	if repo.createdAsset.Mime != "image/png" {
		t.Fatalf("asset mime = %q", repo.createdAsset.Mime)
	}
	if repo.createdNode.Source != "agent" || repo.createdNode.Status != db.NodeStatusSucceeded {
		t.Fatalf("agent node = source %q status %q", repo.createdNode.Source, repo.createdNode.Status)
	}
	if !strings.HasPrefix(storage.objectName, "artifacts/") {
		t.Fatalf("stored object = %q", storage.objectName)
	}
	if client.downloadCalled {
		t.Fatal("artifact submit must not download sandbox output through Go backend")
	}
	if !client.hasExec("curl -sS -f -L -X PUT -T") {
		t.Fatalf("expected sandbox curl upload, execs = %#v", client.execCommands)
	}
	if !broadcaster.has("AssetCreated") || !broadcaster.has("NodeCreated") {
		t.Fatalf("events = %#v", broadcaster.events)
	}
}

func TestArtifactServiceUpdatesExistingNode(t *testing.T) {
	client := &artifactFakeClient{
		inspectStdout: "20\nimage/png",
	}
	repo := &artifactFakeRepository{}
	service := NewArtifactService(client, repo, &artifactFakeStorage{}, &artifactFakeBroadcaster{})
	nodeID := "00112233-4455-6677-8899-aabbccddeeff"

	if _, err := service.Submit(context.Background(), "sandbox-1", testWorkspaceID(), ArtifactInput{
		Path:   "/workspace/output/result.png",
		NodeID: nodeID,
	}); err != nil {
		t.Fatalf("Submit error = %v", err)
	}
	if !repo.updatedExistingNode {
		t.Fatal("expected existing node to be updated")
	}
}

func TestArtifactServiceRejectsUnsupportedMIME(t *testing.T) {
	client := &artifactFakeClient{
		inspectStdout: "10\ntext/plain",
	}
	service := NewArtifactService(client, &artifactFakeRepository{}, &artifactFakeStorage{}, &artifactFakeBroadcaster{})

	if _, err := service.Submit(context.Background(), "sandbox-1", testWorkspaceID(), ArtifactInput{
		Path: "/workspace/output/result.txt",
	}); err == nil {
		t.Fatal("expected unsupported MIME to fail")
	}
}

type artifactFakeClient struct {
	inspectStdout  string
	execCommands   []string
	downloadCalled bool
}

func (f *artifactFakeClient) Create(ctx context.Context, req CreateRequest) (SandboxInfo, error) {
	return SandboxInfo{}, nil
}

func (f *artifactFakeClient) Get(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	return SandboxInfo{}, nil
}

func (f *artifactFakeClient) Ping(ctx context.Context, sandboxID string) error {
	return nil
}

func (f *artifactFakeClient) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error) {
	f.execCommands = append(f.execCommands, req.Command)
	if strings.Contains(req.Command, "stat -c%s") {
		return ExecResult{ExitCode: 0, Stdout: f.inspectStdout}, nil
	}
	return ExecResult{ExitCode: 0}, nil
}

func (f *artifactFakeClient) Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error {
	return nil
}

func (f *artifactFakeClient) Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error) {
	f.downloadCalled = true
	return nil, FileInfo{}, errors.New("download should not be called")
}

func (f *artifactFakeClient) Delete(ctx context.Context, sandboxID string) error {
	return nil
}

func (f *artifactFakeClient) hasExec(fragment string) bool {
	for _, command := range f.execCommands {
		if strings.Contains(command, fragment) {
			return true
		}
	}
	return false
}

type artifactFakeRepository struct {
	createdAsset        db.MediaAsset
	createdNode         db.MediaNode
	updatedExistingNode bool
}

func (r *artifactFakeRepository) CreateMediaAsset(ctx context.Context, arg db.CreateMediaAssetParams) (db.MediaAsset, error) {
	r.createdAsset = db.MediaAsset{
		ID:          pgtype.UUID{Bytes: [16]byte{0x11}, Valid: true},
		WorkspaceID: arg.WorkspaceID,
		Type:        arg.Type,
		Mime:        arg.Mime,
		StorageUrl:  arg.StorageUrl,
		SizeBytes:   arg.SizeBytes,
	}
	return r.createdAsset, nil
}

func (r *artifactFakeRepository) CreateAgentMediaNode(ctx context.Context, arg db.CreateAgentMediaNodeParams) (db.MediaNode, error) {
	r.createdNode = db.MediaNode{
		ID:          pgtype.UUID{Bytes: [16]byte{0x22}, Valid: true},
		WorkspaceID: arg.WorkspaceID,
		NodeType:    arg.NodeType,
		Title:       arg.Title,
		Status:      db.NodeStatusSucceeded,
		Source:      "agent",
		AssetID:     arg.AssetID,
	}
	return r.createdNode, nil
}

func (r *artifactFakeRepository) UpdateMediaNodeAsset(ctx context.Context, arg db.UpdateMediaNodeAssetParams) (db.MediaNode, error) {
	r.updatedExistingNode = true
	return db.MediaNode{ID: arg.ID, WorkspaceID: testWorkspaceID(), AssetID: arg.AssetID}, nil
}

func (r *artifactFakeRepository) UpdateMediaNodeStatus(ctx context.Context, arg db.UpdateMediaNodeStatusParams) (db.MediaNode, error) {
	return db.MediaNode{ID: arg.ID, WorkspaceID: testWorkspaceID(), Status: arg.Status}, nil
}

type artifactFakeStorage struct {
	objectName string
}

func (s *artifactFakeStorage) EnsureBucket(ctx context.Context, workspaceID pgtype.UUID) error {
	return nil
}

func (s *artifactFakeStorage) PresignedSandboxPutURL(ctx context.Context, workspaceID pgtype.UUID, objectName string, expiry time.Duration) (string, error) {
	s.objectName = objectName
	return "http://host.docker.internal:9000/workspace/artifacts/result.png?sig=put", nil
}

func (s *artifactFakeStorage) PresignedGetURL(ctx context.Context, workspaceID pgtype.UUID, objectName string, expiry time.Duration) (string, error) {
	return "http://localhost:9000/workspace/artifacts/result.png?sig=get", nil
}

func (s *artifactFakeStorage) StorageURL(workspaceID pgtype.UUID, objectName string) string {
	return "workspace/" + objectName
}

type artifactFakeBroadcaster struct {
	events []string
}

func (b *artifactFakeBroadcaster) Broadcast(workspaceID pgtype.UUID, event string, payload map[string]any) {
	b.events = append(b.events, event)
}

func (b *artifactFakeBroadcaster) has(event string) bool {
	for _, got := range b.events {
		if got == event {
			return true
		}
	}
	return false
}
