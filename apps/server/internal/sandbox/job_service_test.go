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

func TestJobServiceRunCommandPersistsSucceededSandboxJob(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{result: ExecResult{ExitCode: 0, Stdout: "ok", DurationMS: 42}}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	service := NewJobService(manager, client, repo, nil)

	result, err := service.RunCommand(context.Background(), SandboxJobInput{
		WorkspaceID:   testWorkspaceID(),
		TargetNodeID:  testNodeID(),
		JobType:       "agent_shell",
		OperationType: "shell",
		Command:       "echo ok",
	})
	if err != nil {
		t.Fatalf("RunCommand error = %v", err)
	}
	if result.Job.Status != db.JobStatusSucceeded {
		t.Fatalf("job status = %q, want succeeded", result.Job.Status)
	}
	if result.Job.SandboxID.String != "sandbox-1" {
		t.Fatalf("sandbox id = %q", result.Job.SandboxID.String)
	}
	if result.Job.Stdout != "ok" {
		t.Fatalf("stdout = %q, want ok", result.Job.Stdout)
	}
	if len(repo.calls) != 3 || repo.calls[0] != "create" || repo.calls[1] != "running" || repo.calls[2] != "succeeded" {
		t.Fatalf("calls = %#v", repo.calls)
	}
	if !strings.Contains(client.commands[0], "mkdir -p /workspace/assets /workspace/scripts /workspace/tmp /workspace/output") {
		t.Fatalf("layout command = %q", client.commands[0])
	}
	if !strings.Contains(client.commands[1], "echo ok") {
		t.Fatalf("command = %q", client.commands[1])
	}
}

func TestJobServiceRunCommandPersistsFailedSandboxJobOnNonZeroExit(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{result: ExecResult{ExitCode: 2, Stderr: "ffmpeg failed", DurationMS: 17}}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	service := NewJobService(manager, client, repo, nil)

	_, err := service.RunCommand(context.Background(), SandboxJobInput{
		WorkspaceID:   testWorkspaceID(),
		TargetNodeID:  testNodeID(),
		JobType:       "internal_media",
		OperationType: "extract_first_frame",
		Command:       "ffmpeg -version",
	})
	if err == nil {
		t.Fatal("expected non-zero exit to return error")
	}
	if repo.latest.Status != db.JobStatusFailed {
		t.Fatalf("job status = %q, want failed", repo.latest.Status)
	}
	if repo.latest.ExitCode.Int32 != 2 {
		t.Fatalf("exit code = %d, want 2", repo.latest.ExitCode.Int32)
	}
	if repo.latest.ErrorCode.String != "sandbox_command_failed" {
		t.Fatalf("error code = %q", repo.latest.ErrorCode.String)
	}
	if !strings.Contains(repo.latest.ErrorMessage.String, "ffmpeg failed") {
		t.Fatalf("error message = %q", repo.latest.ErrorMessage.String)
	}
}

func TestJobServiceExtractFrameExecutesFFmpegInsideSandboxAndUploadsOutput(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{
		result: ExecResult{ExitCode: 0, Stdout: "done", DurationMS: 55},
		inspect: FileInfo{
			Path:      "/workspace/output/frame-node-1.png",
			SizeBytes: 123,
			Mime:      "image/png",
		},
	}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	storage := &fakeSandboxJobStorage{}
	service := NewJobService(manager, client, repo, storage)

	result, err := service.ExtractFrame(context.Background(), ExtractFrameInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		Source: SandboxAssetInput{
			AssetID:    "asset-video",
			StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/video.mp4",
			Mime:       "video/mp4",
		},
		Mode: FrameFirst,
	})
	if err != nil {
		t.Fatalf("ExtractFrame error = %v", err)
	}
	if result.Job.Status != db.JobStatusSucceeded {
		t.Fatalf("job status = %q, want succeeded", result.Job.Status)
	}
	if result.Asset.StorageURL == "" {
		t.Fatal("expected output storage url")
	}
	if len(storage.putKeys) != 1 || !strings.HasPrefix(storage.putKeys[0], "production/") {
		t.Fatalf("put keys = %#v", storage.putKeys)
	}
	joined := strings.Join(client.commands, "\n")
	if !strings.Contains(joined, "curl -sS -f -L -o") || !strings.Contains(joined, "/workspace/assets/asset-video.mp4") {
		t.Fatalf("expected sandbox download command, got %q", joined)
	}
	if !strings.Contains(joined, "ffmpeg -y -i") || !strings.Contains(joined, "-frames:v 1") || !strings.Contains(joined, "/workspace/output/frame-") {
		t.Fatalf("expected ffmpeg sandbox command, got %q", joined)
	}
	if !strings.Contains(joined, "curl -sS -f -L -X PUT -T") || !strings.Contains(joined, "/workspace/output/frame-") {
		t.Fatalf("expected sandbox upload command, got %q", joined)
	}
}

func testNodeID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{0xbb, 0xcc, 0xdd, 0xee}, Valid: true}
}

type fakeSandboxJobRepository struct {
	calls  []string
	nextID byte
	latest db.SandboxJob
}

func newFakeSandboxJobRepository() *fakeSandboxJobRepository {
	return &fakeSandboxJobRepository{nextID: 1}
}

func (r *fakeSandboxJobRepository) CreateSandboxJob(ctx context.Context, arg db.CreateSandboxJobParams) (db.SandboxJob, error) {
	r.calls = append(r.calls, "create")
	job := db.SandboxJob{
		ID:            pgtype.UUID{Bytes: [16]byte{r.nextID}, Valid: true},
		WorkspaceID:   arg.WorkspaceID,
		TargetNodeID:  arg.TargetNodeID,
		JobType:       arg.JobType,
		OperationType: arg.OperationType,
		Status:        db.JobStatusPending,
		Input:         arg.Input,
		Output:        []byte("{}"),
		CreatedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	r.nextID++
	r.latest = job
	return job, nil
}

func (r *fakeSandboxJobRepository) MarkSandboxJobRunning(ctx context.Context, arg db.MarkSandboxJobRunningParams) (db.SandboxJob, error) {
	r.calls = append(r.calls, "running")
	r.latest.Status = db.JobStatusRunning
	r.latest.SandboxID = arg.SandboxID
	r.latest.Command = arg.Command
	r.latest.Cwd = arg.Cwd
	return r.latest, nil
}

func (r *fakeSandboxJobRepository) MarkSandboxJobSucceeded(ctx context.Context, arg db.MarkSandboxJobSucceededParams) (db.SandboxJob, error) {
	r.calls = append(r.calls, "succeeded")
	r.latest.Status = db.JobStatusSucceeded
	r.latest.Output = arg.Output
	r.latest.ExitCode = arg.ExitCode
	r.latest.Stdout = arg.Stdout
	r.latest.Stderr = arg.Stderr
	r.latest.DurationMs = arg.DurationMs
	return r.latest, nil
}

func (r *fakeSandboxJobRepository) MarkSandboxJobFailed(ctx context.Context, arg db.MarkSandboxJobFailedParams) (db.SandboxJob, error) {
	r.calls = append(r.calls, "failed")
	r.latest.Status = db.JobStatusFailed
	r.latest.Output = arg.Output
	r.latest.ExitCode = arg.ExitCode
	r.latest.Stdout = arg.Stdout
	r.latest.Stderr = arg.Stderr
	r.latest.DurationMs = arg.DurationMs
	r.latest.ErrorCode = arg.ErrorCode
	r.latest.ErrorMessage = arg.ErrorMessage
	return r.latest, nil
}

type jobServiceFakeClient struct {
	result   ExecResult
	inspect  FileInfo
	commands []string
}

func (f *jobServiceFakeClient) Create(ctx context.Context, req CreateRequest) (SandboxInfo, error) {
	return SandboxInfo{ID: "sandbox-1", State: StatusRunning}, nil
}

func (f *jobServiceFakeClient) Get(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	return SandboxInfo{ID: sandboxID, State: StatusRunning}, nil
}

func (f *jobServiceFakeClient) Ping(ctx context.Context, sandboxID string) error {
	return nil
}

func (f *jobServiceFakeClient) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error) {
	f.commands = append(f.commands, req.Command)
	if strings.Contains(req.Command, "stat -c%s") {
		if f.inspect.SizeBytes == 0 {
			f.inspect = FileInfo{SizeBytes: 123, Mime: "image/png"}
		}
		return ExecResult{ExitCode: 0, Stdout: "123\n" + f.inspect.Mime}, nil
	}
	return f.result, nil
}

func (f *jobServiceFakeClient) Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error {
	return nil
}

func (f *jobServiceFakeClient) Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error) {
	return io.NopCloser(strings.NewReader("")), FileInfo{}, nil
}

func (f *jobServiceFakeClient) Delete(ctx context.Context, sandboxID string) error {
	return nil
}

type fakeSandboxJobStorage struct {
	putKeys []string
}

func (s *fakeSandboxJobStorage) EnsureBucket(ctx context.Context, workspaceID pgtype.UUID) error {
	return nil
}

func (s *fakeSandboxJobStorage) PresignedSandboxGetURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error) {
	if key == "" {
		return "", errors.New("missing key")
	}
	return "http://host.docker.internal:9000/get/" + key + "?X-Amz-Signature=test", nil
}

func (s *fakeSandboxJobStorage) PresignedSandboxPutURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error) {
	if key == "" {
		return "", errors.New("missing key")
	}
	s.putKeys = append(s.putKeys, key)
	return "http://host.docker.internal:9000/put/" + key + "?X-Amz-Signature=test", nil
}

func (s *fakeSandboxJobStorage) StorageURL(workspaceID pgtype.UUID, key string) string {
	return "workspace-aabbccdd-0000-0000-0000-000000000000/" + key
}
