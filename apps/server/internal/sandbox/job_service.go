package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type SandboxJobInput struct {
	WorkspaceID    pgtype.UUID
	TargetNodeID   pgtype.UUID
	JobType        string
	OperationType  string
	Command        string
	Cwd            string
	TimeoutSeconds int
	Input          map[string]any
}

type SandboxJobResult struct {
	Job   db.SandboxJob
	Exec  ExecResult
	Asset ArtifactObject
	MIME  string
	Size  int64
}

type SandboxAssetInput struct {
	AssetID    string
	StorageURL string
	Mime       string
}

type FrameMode string

const (
	FrameFirst FrameMode = "first"
	FrameLast  FrameMode = "last"
)

type ExtractFrameInput struct {
	WorkspaceID  pgtype.UUID
	TargetNodeID pgtype.UUID
	Source       SandboxAssetInput
	Mode         FrameMode
}

type JobRepository interface {
	CreateSandboxJob(ctx context.Context, arg db.CreateSandboxJobParams) (db.SandboxJob, error)
	MarkSandboxJobRunning(ctx context.Context, arg db.MarkSandboxJobRunningParams) (db.SandboxJob, error)
	MarkSandboxJobSucceeded(ctx context.Context, arg db.MarkSandboxJobSucceededParams) (db.SandboxJob, error)
	MarkSandboxJobFailed(ctx context.Context, arg db.MarkSandboxJobFailedParams) (db.SandboxJob, error)
}

type JobStorage interface {
	EnsureBucket(ctx context.Context, workspaceID pgtype.UUID) error
	PresignedSandboxGetURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error)
	PresignedSandboxPutURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error)
	StorageURL(workspaceID pgtype.UUID, key string) string
}

type JobService struct {
	manager *Manager
	client  Client
	repo    JobRepository
	storage JobStorage
}

func NewJobService(manager *Manager, client Client, repo JobRepository, storage JobStorage) *JobService {
	return &JobService{manager: manager, client: client, repo: repo, storage: storage}
}

func (s *JobService) RunCommand(ctx context.Context, input SandboxJobInput) (SandboxJobResult, error) {
	if s.manager == nil || s.client == nil || s.repo == nil {
		return SandboxJobResult{}, errors.New("sandbox job service is not configured")
	}
	if strings.TrimSpace(input.JobType) == "" {
		input.JobType = "sandbox_exec"
	}
	if strings.TrimSpace(input.OperationType) == "" {
		input.OperationType = "shell"
	}
	inputJSON, err := json.Marshal(mapWithDefaults(input.Input, map[string]any{
		"command": input.Command,
		"cwd":     input.Cwd,
	}))
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err := s.repo.CreateSandboxJob(ctx, db.CreateSandboxJobParams{
		WorkspaceID:   input.WorkspaceID,
		TargetNodeID:  input.TargetNodeID,
		JobType:       input.JobType,
		OperationType: input.OperationType,
		Input:         inputJSON,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	workspaceSandbox, err := s.manager.EnsureSandbox(ctx, input.WorkspaceID)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_unavailable", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed}, err
	}
	if err := EnsureWorkspaceLayout(ctx, s.client, workspaceSandbox.SandboxID); err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_layout_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed}, err
	}
	cwd := strings.TrimSpace(input.Cwd)
	if cwd == "" {
		cwd = DefaultWorkdir
	}
	job, err = s.repo.MarkSandboxJobRunning(ctx, db.MarkSandboxJobRunningParams{
		ID:        job.ID,
		SandboxID: pgtype.Text{String: workspaceSandbox.SandboxID, Valid: true},
		Command:   input.Command,
		Cwd:       cwd,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	result, err := RunExec(ctx, s.client, workspaceSandbox.SandboxID, ExecInput{
		Command:        input.Command,
		Cwd:            input.Cwd,
		TimeoutSeconds: input.TimeoutSeconds,
	})
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, result, "sandbox_exec_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: result}, err
	}
	if result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = fmt.Sprintf("sandbox command exited with code %d", result.ExitCode)
		}
		failed, failErr := s.markFailed(ctx, job.ID, result, "sandbox_command_failed", message, nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: result}, fmt.Errorf("sandbox command failed: %s", message)
	}
	outputJSON, err := json.Marshal(map[string]any{"exit_code": result.ExitCode})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err = s.repo.MarkSandboxJobSucceeded(ctx, db.MarkSandboxJobSucceededParams{
		ID:         job.ID,
		Output:     outputJSON,
		ExitCode:   pgtype.Int4{Int32: int32(result.ExitCode), Valid: true},
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		DurationMs: int32(result.DurationMS),
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	return SandboxJobResult{Job: job, Exec: result}, nil
}

func (s *JobService) ExtractFrame(ctx context.Context, input ExtractFrameInput) (SandboxJobResult, error) {
	if s.manager == nil || s.client == nil || s.repo == nil || s.storage == nil {
		return SandboxJobResult{}, errors.New("sandbox frame extraction is not configured")
	}
	mode, err := normalizeFrameMode(input.Mode)
	if err != nil {
		return SandboxJobResult{}, err
	}
	inputJSON, err := json.Marshal(map[string]any{
		"source_asset_id": input.Source.AssetID,
		"source_mime":     input.Source.Mime,
		"mode":            mode,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err := s.repo.CreateSandboxJob(ctx, db.CreateSandboxJobParams{
		WorkspaceID:   input.WorkspaceID,
		TargetNodeID:  input.TargetNodeID,
		JobType:       "internal_media",
		OperationType: "extract_" + string(mode) + "_frame",
		Input:         inputJSON,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	workspaceSandbox, err := s.manager.EnsureSandbox(ctx, input.WorkspaceID)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_unavailable", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed}, err
	}
	if err := EnsureWorkspaceLayout(ctx, s.client, workspaceSandbox.SandboxID); err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_layout_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed}, err
	}

	sourceKey, err := storage.KeyFromStorageURL(input.WorkspaceID, input.Source.StorageURL)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_input_invalid", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed}, err
	}
	getURL, err := s.storage.PresignedSandboxGetURL(ctx, input.WorkspaceID, sourceKey, time.Hour)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_input_presign_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed}, err
	}

	sourcePath := path.Join(AssetsDir, safeInputName(input.Source.AssetID, input.Source.Mime))
	downloadResult, err := DownloadFromMinIO(ctx, s.client, workspaceSandbox.SandboxID, getURL, sourcePath)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, downloadResult, "sandbox_input_download_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: downloadResult}, err
	}
	if downloadResult.ExitCode != 0 {
		message := strings.TrimSpace(downloadResult.Stderr)
		failed, failErr := s.markFailed(ctx, job.ID, downloadResult, "sandbox_input_download_failed", message, nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: downloadResult}, fmt.Errorf("sandbox input download failed: %s", message)
	}

	outputPath := path.Join(OutputDir, "frame-"+uuidString(job.ID)+".png")
	command := frameExtractCommand(sourcePath, outputPath, mode)
	job, err = s.repo.MarkSandboxJobRunning(ctx, db.MarkSandboxJobRunningParams{
		ID:        job.ID,
		SandboxID: pgtype.Text{String: workspaceSandbox.SandboxID, Valid: true},
		Command:   command,
		Cwd:       DefaultWorkdir,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	execResult, err := RunExec(ctx, s.client, workspaceSandbox.SandboxID, ExecInput{
		Command:        command,
		Cwd:            DefaultWorkdir,
		TimeoutSeconds: MaxExecTimeoutSeconds,
	})
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_ffmpeg_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	if execResult.ExitCode != 0 {
		message := strings.TrimSpace(execResult.Stderr)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_ffmpeg_failed", message, nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, fmt.Errorf("sandbox ffmpeg failed: %s", message)
	}

	info, err := InspectArtifact(ctx, s.client, workspaceSandbox.SandboxID, outputPath)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_inspect_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	if info.Mime != "image/png" {
		err := fmt.Errorf("unexpected extracted frame MIME %q", info.Mime)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_invalid", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	objectKey := "production/" + uuidString(input.TargetNodeID) + "/" + uuidString(job.ID) + ".png"
	if err := s.storage.EnsureBucket(ctx, input.WorkspaceID); err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_upload_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	putURL, err := s.storage.PresignedSandboxPutURL(ctx, input.WorkspaceID, objectKey, time.Hour)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_upload_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	uploadResult, err := UploadToMinIO(ctx, s.client, workspaceSandbox.SandboxID, outputPath, putURL)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, uploadResult, "sandbox_output_upload_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: uploadResult}, err
	}
	if uploadResult.ExitCode != 0 {
		message := strings.TrimSpace(uploadResult.Stderr)
		failed, failErr := s.markFailed(ctx, job.ID, uploadResult, "sandbox_output_upload_failed", message, nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: uploadResult}, fmt.Errorf("sandbox output upload failed: %s", message)
	}

	asset := ArtifactObject{StorageURL: s.storage.StorageURL(input.WorkspaceID, objectKey)}
	outputJSON, err := json.Marshal(map[string]any{
		"output_path": outputPath,
		"storage_url": asset.StorageURL,
		"mime":        info.Mime,
		"size_bytes":  info.SizeBytes,
		"mode":        mode,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err = s.repo.MarkSandboxJobSucceeded(ctx, db.MarkSandboxJobSucceededParams{
		ID:         job.ID,
		Output:     outputJSON,
		ExitCode:   pgtype.Int4{Int32: int32(execResult.ExitCode), Valid: true},
		Stdout:     execResult.Stdout,
		Stderr:     execResult.Stderr,
		DurationMs: int32(execResult.DurationMS),
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	return SandboxJobResult{Job: job, Exec: execResult, Asset: asset, MIME: info.Mime, Size: info.SizeBytes}, nil
}

func (s *JobService) markFailed(ctx context.Context, jobID pgtype.UUID, result ExecResult, code string, message string, output map[string]any) (db.SandboxJob, error) {
	if output == nil {
		output = map[string]any{}
	}
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return db.SandboxJob{}, err
	}
	return s.repo.MarkSandboxJobFailed(ctx, db.MarkSandboxJobFailedParams{
		ID:           jobID,
		Output:       outputJSON,
		ExitCode:     pgtype.Int4{Int32: int32(result.ExitCode), Valid: true},
		Stdout:       result.Stdout,
		Stderr:       result.Stderr,
		DurationMs:   int32(result.DurationMS),
		ErrorCode:    pgtype.Text{String: code, Valid: code != ""},
		ErrorMessage: pgtype.Text{String: strings.TrimSpace(message), Valid: strings.TrimSpace(message) != ""},
	})
}

func frameExtractCommand(sourcePath string, outputPath string, mode FrameMode) string {
	args := []string{"ffmpeg", "-y"}
	if mode == FrameLast {
		args = append(args, "-sseof", "-1")
	}
	args = append(args, "-i", shellQuote(sourcePath), "-frames:v", "1", "-update", "1", "-f", "image2", shellQuote(outputPath))
	return strings.Join(args, " ")
}

func normalizeFrameMode(mode FrameMode) (FrameMode, error) {
	switch mode {
	case FrameFirst, "":
		return FrameFirst, nil
	case FrameLast:
		return FrameLast, nil
	default:
		return "", fmt.Errorf("unsupported frame mode %q", mode)
	}
}

func safeInputName(assetID string, mime string) string {
	name := SafeAssetName(strings.TrimSpace(assetID))
	if name == "" {
		name = "input"
	}
	return name + extensionForMIME(mime)
}

func extensionForMIME(mime string) string {
	switch strings.TrimSpace(mime) {
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	default:
		return ".bin"
	}
}

func mapWithDefaults(values map[string]any, defaults map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range defaults {
		if value != nil && value != "" {
			out[key] = value
		}
	}
	for key, value := range values {
		out[key] = value
	}
	return out
}
