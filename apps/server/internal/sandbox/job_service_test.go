package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/motionshot"
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

func TestJobServiceComposeVideosExecutesFFmpegConcatAndUploadsOutput(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{
		result: ExecResult{ExitCode: 0, Stdout: "done", DurationMS: 88},
		inspect: FileInfo{
			Path:      "/workspace/output/final-node-1.mp4",
			SizeBytes: 456,
			Mime:      "video/mp4",
		},
	}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	storage := &fakeSandboxJobStorage{}
	service := NewJobService(manager, client, repo, storage)

	result, err := service.ComposeVideos(context.Background(), ComposeVideosInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		Sources: []SandboxAssetInput{
			{AssetID: "shot-1", StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/shot-1.mp4", Mime: "video/mp4"},
			{AssetID: "shot-2", StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/shot-2.mp4", Mime: "video/mp4"},
		},
	})
	if err != nil {
		t.Fatalf("ComposeVideos error = %v", err)
	}
	if result.Job.Status != db.JobStatusSucceeded || result.MIME != "video/mp4" {
		t.Fatalf("result = %#v", result)
	}
	if len(storage.putKeys) != 1 || !strings.HasSuffix(storage.putKeys[0], ".mp4") {
		t.Fatalf("put keys = %#v", storage.putKeys)
	}
	joined := strings.Join(client.commands, "\n")
	if strings.Count(joined, "curl -sS -f -L -o") < 2 {
		t.Fatalf("expected two sandbox download commands, got %q", joined)
	}
	if !strings.Contains(joined, "ffmpeg -y -f concat -safe 0 -i") || !strings.Contains(joined, "/workspace/output/final-") {
		t.Fatalf("expected ffmpeg concat command, got %q", joined)
	}
}

func TestJobServiceComposeVideosMixesAudioTracks(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{
		result: ExecResult{ExitCode: 0, Stdout: "done", DurationMS: 88},
		inspect: FileInfo{
			Path:      "/workspace/output/final-node-1.mp4",
			SizeBytes: 456,
			Mime:      "video/mp4",
		},
	}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	storage := &fakeSandboxJobStorage{}
	service := NewJobService(manager, client, repo, storage)

	result, err := service.ComposeVideos(context.Background(), ComposeVideosInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		Sources: []SandboxAssetInput{
			{AssetID: "shot-1", StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/shot-1.mp4", Mime: "video/mp4"},
			{AssetID: "shot-2", StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/shot-2.mp4", Mime: "video/mp4"},
		},
		AudioTracks: []ComposeAudioTrackInput{
			{Role: "voiceover", Source: SandboxAssetInput{AssetID: "voiceover", StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/voiceover.mp3", Mime: "audio/mpeg"}, Volume: 1, DurationSec: 8.2, FadeInSec: 0.05, FadeOutSec: 0.1},
			{Role: "bgm", Source: SandboxAssetInput{AssetID: "bgm", StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/bgm.mp3", Mime: "audio/mpeg"}, Volume: 0.28, DurationSec: 8.2, FadeInSec: 0.5, FadeOutSec: 1.2, Ducking: ComposeAudioDuckingInput{SidechainRole: "voiceover", Threshold: 0.08, Ratio: 8, AttackMS: 20, ReleaseMS: 250}},
		},
	})
	if err != nil {
		t.Fatalf("ComposeVideos error = %v", err)
	}
	if result.Job.Status != db.JobStatusSucceeded || result.MIME != "video/mp4" {
		t.Fatalf("result = %#v", result)
	}
	joined := strings.Join(client.commands, "\n")
	if strings.Count(joined, "curl -sS -f -L -o") < 4 {
		t.Fatalf("expected video and audio downloads, got %q", joined)
	}
	for _, want := range []string{"voiceover.mp3", "bgm.mp3", "concat=n=2:v=1:a=0", "asplit=2", "sidechaincompress", "[aout]", "-c:a aac", "-shortest"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected compose audio command containing %q, got %q", want, joined)
		}
	}
}

func TestJobServiceRenderMotionShotRunsRemotionAndUploadsMP4(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{
		result: ExecResult{ExitCode: 0, Stdout: "rendered", DurationMS: 88},
		inspect: FileInfo{
			Path:      "/workspace/output/motion-node-1.mp4",
			SizeBytes: 456,
			Mime:      "video/mp4",
		},
	}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	storage := &fakeSandboxJobStorage{}
	service := NewJobService(manager, client, repo, storage)

	result, err := service.RenderMotionShot(context.Background(), RenderMotionShotInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		Plan: motionshot.Plan{
			DurationSec:    5,
			Width:          1080,
			Height:         1920,
			FPS:            30,
			DurationFrames: 150,
			MotionStyle:    "premium_product_ad",
			VisualLayers:   []motionshot.VisualLayer{{Role: "product", InputRef: "assets/product.png", Fit: "contain", Motion: "slow_push_in", StartSec: 0, EndSec: 5}},
		},
		Meta:   MotionShotMeta{DurationSec: 5, Width: 1080, Height: 1920, FPS: 30},
		Assets: []RenderMotionAssetInput{{AssetID: "product", StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/source/product.png", Mime: "image/png", FileName: "product.png", WorkspacePath: "assets/product.png"}},
	})
	if err != nil {
		t.Fatalf("RenderMotionShot error = %v", err)
	}
	if result.Job.Status != db.JobStatusSucceeded || result.Job.OperationType != "image_to_motion_video" || result.MIME != "video/mp4" {
		t.Fatalf("result = %#v", result)
	}
	if len(storage.putKeys) != 1 || !strings.HasSuffix(storage.putKeys[0], ".mp4") {
		t.Fatalf("put keys = %#v", storage.putKeys)
	}
	joined := strings.Join(client.commands, "\n")
	for _, want := range []string{"curl -sS -f -L -o", "motion-plan.json", "remotion render", "/workspace/output/motion-"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected motion render command containing %q, got %q", want, joined)
		}
	}
	if !strings.Contains(result.Job.Cwd, "/workspace/motion-shot/") {
		t.Fatalf("cwd = %q", result.Job.Cwd)
	}
	var uploadedPlan motionshot.Plan
	for uploadPath, body := range client.uploadBodies {
		if strings.HasSuffix(uploadPath, "motion-plan.json") {
			if err := json.Unmarshal([]byte(body), &uploadedPlan); err != nil {
				t.Fatalf("uploaded plan JSON error = %v", err)
			}
		}
	}
	if len(uploadedPlan.VisualLayers) != 1 {
		t.Fatalf("uploaded visual layers = %#v", uploadedPlan.VisualLayers)
	}
	if got, want := uploadedPlan.VisualLayers[0].InputRef, "assets/product.png"; got != want {
		t.Fatalf("uploaded visual input_ref = %q, want %q", got, want)
	}
}

func TestJobServiceStageMediaInputsDownloadsToWorkspaceInput(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{result: ExecResult{ExitCode: 0, Stdout: "done", DurationMS: 12}}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	storage := &fakeSandboxJobStorage{}
	service := NewJobService(manager, client, repo, storage)

	result, err := service.StageMediaInputs(context.Background(), StageMediaInputsInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		Assets: []StageMediaAssetInput{
			{
				AssetID:   pgtype.UUID{Bytes: [16]byte{0x10}, Valid: true},
				SourceURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/shot-1.mp4",
				FileName:  "Hero Shot.mp4",
				MimeType:  "video/mp4",
				SizeBytes: 101,
			},
			{
				AssetID:   pgtype.UUID{Bytes: [16]byte{0x20}, Valid: true},
				SourceURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/shot-2.mp4",
				FileName:  "Hero Shot.mp4",
				MimeType:  "video/mp4",
				SizeBytes: 202,
			},
		},
	})
	if err != nil {
		t.Fatalf("StageMediaInputs error = %v", err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("files = %#v", result.Files)
	}
	if result.Files[0].WorkspacePath != "/workspace/input/Hero-Shot.mp4" {
		t.Fatalf("first path = %q", result.Files[0].WorkspacePath)
	}
	if result.Files[1].WorkspacePath == result.Files[0].WorkspacePath || !strings.HasPrefix(result.Files[1].WorkspacePath, "/workspace/input/Hero-Shot-") {
		t.Fatalf("duplicate path = %q", result.Files[1].WorkspacePath)
	}
	if result.Files[1].MimeType != "video/mp4" || result.Files[1].SizeBytes != 202 {
		t.Fatalf("second file metadata = %#v", result.Files[1])
	}
	joined := strings.Join(client.commands, "\n")
	if !strings.Contains(joined, "mkdir -p /workspace/input") {
		t.Fatalf("expected input directory creation, got %q", joined)
	}
	if strings.Count(joined, "curl -sS -f -L -o") != 2 {
		t.Fatalf("expected two downloads, got %q", joined)
	}
}

func TestBuildFFmpegCommandAllowsOnlyFFmpegInsideWorkspace(t *testing.T) {
	if _, _, err := buildFFmpegCommand(RunFFmpegCommandInput{
		Executable: "ffmpeg",
		Cwd:        "/workspace",
		Args:       []string{"-y", "-i", "input/clip.mp4", "-c:v", "libx264", "output/final.mp4"},
	}); err != nil {
		t.Fatalf("buildFFmpegCommand valid ffmpeg error = %v", err)
	}
	if _, _, err := buildFFmpegCommand(RunFFmpegCommandInput{
		Executable: "ffprobe",
		Cwd:        "/workspace",
		Args:       []string{"-v", "error", "-print_format", "json", "/workspace/input/clip.mp4"},
	}); err != nil {
		t.Fatalf("buildFFmpegCommand valid ffprobe error = %v", err)
	}
	for _, tc := range []RunFFmpegCommandInput{
		{Executable: "bash", Cwd: "/workspace", Args: []string{"-lc", "echo nope"}},
		{Executable: "sh", Cwd: "/workspace", Args: []string{"-c", "echo nope"}},
		{Executable: "ffmpeg", Cwd: "/tmp", Args: []string{"-version"}},
		{Executable: "ffmpeg", Cwd: "/workspace", Args: []string{"-i", "/tmp/input.mp4", "/workspace/output/final.mp4"}},
		{Executable: "ffmpeg", Cwd: "/workspace", Args: []string{"-i", "../secret.mp4", "output/final.mp4"}},
	} {
		if _, _, err := buildFFmpegCommand(tc); err == nil {
			t.Fatalf("expected buildFFmpegCommand to reject %#v", tc)
		}
	}
}

func TestJobServiceRunFFmpegCommandPersistsSandboxJob(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{result: ExecResult{ExitCode: 0, Stdout: "ffmpeg ok", DurationMS: 33}}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	service := NewJobService(manager, client, repo, nil)

	result, err := service.RunFFmpegCommand(context.Background(), RunFFmpegCommandInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		Executable:   "ffmpeg",
		Cwd:          "/workspace",
		Args:         []string{"-y", "-i", "input/clip.mp4", "-c:v", "libx264", "output/final.mp4"},
		TimeoutSec:   300,
	})
	if err != nil {
		t.Fatalf("RunFFmpegCommand error = %v", err)
	}
	if result.Job.Status != db.JobStatusSucceeded || result.Job.OperationType != "run_ffmpeg_command" {
		t.Fatalf("job = %#v", result.Job)
	}
	joined := strings.Join(client.commands, "\n")
	if !strings.Contains(joined, "ffmpeg") || !strings.Contains(joined, "input/clip.mp4") || !strings.Contains(joined, "output/final.mp4") {
		t.Fatalf("expected ffmpeg command, got %q", joined)
	}
}

func TestJobServiceProbeMediaParsesFFprobeJSON(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{result: ExecResult{
		ExitCode: 0,
		Stdout:   `{"format":{"duration":"1.500"},"streams":[{"codec_type":"video","width":1280,"height":720}]}`,
	}}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	service := NewJobService(manager, client, repo, nil)

	result, err := service.ProbeMedia(context.Background(), ProbeMediaInput{
		WorkspaceID:   testWorkspaceID(),
		TargetNodeID:  testNodeID(),
		WorkspacePath: "/workspace/input/clip.mp4",
	})
	if err != nil {
		t.Fatalf("ProbeMedia error = %v", err)
	}
	if result.Format["duration"] != "1.500" || len(result.Streams) != 1 || result.Streams[0]["codec_type"] != "video" {
		t.Fatalf("probe result = %#v", result)
	}
}

func TestJobServiceImportRemoteAssetDownloadsInsideSandboxAndUploadsOutput(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{
		result: ExecResult{ExitCode: 0, Stdout: "done", DurationMS: 49},
		inspect: FileInfo{
			Path:      "/workspace/assets/provider-node.mp4",
			SizeBytes: 456,
			Mime:      "video/mp4",
		},
	}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	storage := &fakeSandboxJobStorage{}
	service := NewJobService(manager, client, repo, storage)

	result, err := service.ImportRemoteAsset(context.Background(), RemoteAssetInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		SourceURL:    "https://provider.example/output/video.mp4?token=secret",
		MimeHint:     "video/mp4",
	})
	if err != nil {
		t.Fatalf("ImportRemoteAsset error = %v", err)
	}
	if result.Job.Status != db.JobStatusSucceeded {
		t.Fatalf("job status = %q, want succeeded", result.Job.Status)
	}
	if result.Asset.StorageURL == "" || result.Thumbnail.StorageURL == "" || result.MIME != "video/mp4" || result.Size != 123 {
		t.Fatalf("result = %#v", result)
	}
	if len(storage.putKeys) != 2 || !strings.HasPrefix(storage.putKeys[0], "production/") || !strings.HasSuffix(storage.putKeys[0], "-thumbnail.png") || !strings.HasSuffix(storage.putKeys[1], ".mp4") {
		t.Fatalf("put keys = %#v", storage.putKeys)
	}
	joined := strings.Join(client.commands, "\n")
	if !strings.Contains(joined, "curl -sS -f -L -o") || !strings.Contains(joined, "https://provider.example/output/video.mp4?token=secret") {
		t.Fatalf("expected sandbox provider download command, got %q", joined)
	}
	if !strings.Contains(joined, "curl -sS -f -L -X PUT -T") || !strings.Contains(joined, "/workspace/assets/provider-") {
		t.Fatalf("expected sandbox upload command, got %q", joined)
	}
	if !strings.Contains(joined, "ffmpeg -y -i") || !strings.Contains(joined, "/workspace/output/thumbnail-") {
		t.Fatalf("expected sandbox thumbnail extraction command, got %q", joined)
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
	result       ExecResult
	inspect      FileInfo
	commands     []string
	uploads      []string
	uploadBodies map[string]string
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
		if strings.Contains(req.Command, "thumbnail-") {
			return ExecResult{ExitCode: 0, Stdout: "123\nimage/png"}, nil
		}
		if f.inspect.SizeBytes == 0 {
			f.inspect = FileInfo{SizeBytes: 123, Mime: "image/png"}
		}
		return ExecResult{ExitCode: 0, Stdout: "123\n" + f.inspect.Mime}, nil
	}
	return f.result, nil
}

func (f *jobServiceFakeClient) Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error {
	f.uploads = append(f.uploads, path)
	if f.uploadBodies == nil {
		f.uploadBodies = map[string]string{}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.uploadBodies[path] = string(data)
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
