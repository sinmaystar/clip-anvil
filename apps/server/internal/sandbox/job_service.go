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
	Job       db.SandboxJob
	Exec      ExecResult
	Asset     ArtifactObject
	Thumbnail ArtifactObject
	MIME      string
	Size      int64
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

type ComposeVideosInput struct {
	WorkspaceID  pgtype.UUID
	TargetNodeID pgtype.UUID
	Sources      []SandboxAssetInput
	AudioTracks  []ComposeAudioTrackInput
}

type ComposeAudioTrackInput struct {
	Role        string
	Source      SandboxAssetInput
	StartSec    float64
	DurationSec float64
	Volume      float64
	FadeInSec   float64
	FadeOutSec  float64
	Ducking     ComposeAudioDuckingInput
}

type ComposeAudioDuckingInput struct {
	SidechainRole string
	Threshold     float64
	Ratio         float64
	AttackMS      int
	ReleaseMS     int
}

type RemoteAssetInput struct {
	WorkspaceID  pgtype.UUID
	TargetNodeID pgtype.UUID
	SourceURL    string
	MimeHint     string
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

func (s *JobService) ComposeVideos(ctx context.Context, input ComposeVideosInput) (SandboxJobResult, error) {
	if s.manager == nil || s.client == nil || s.repo == nil || s.storage == nil {
		return SandboxJobResult{}, errors.New("sandbox video composition is not configured")
	}
	if len(input.Sources) == 0 {
		return SandboxJobResult{}, errors.New("compose videos requires at least one source")
	}
	inputJSON, err := json.Marshal(map[string]any{"source_count": len(input.Sources), "audio_track_count": len(input.AudioTracks)})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err := s.repo.CreateSandboxJob(ctx, db.CreateSandboxJobParams{
		WorkspaceID:   input.WorkspaceID,
		TargetNodeID:  input.TargetNodeID,
		JobType:       "internal_media",
		OperationType: "compose_final_video",
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
	sourcePaths := make([]string, 0, len(input.Sources))
	for index, source := range input.Sources {
		sourceKey, err := storage.KeyFromStorageURL(input.WorkspaceID, source.StorageURL)
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
		sourcePath := path.Join(AssetsDir, fmt.Sprintf("compose-%02d-%s%s", index+1, safePathComponent(source.AssetID), extensionForMIME(source.Mime)))
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
		sourcePaths = append(sourcePaths, sourcePath)
	}
	audioTracks := make([]composeAudioTrackPath, 0, len(input.AudioTracks))
	for index, track := range input.AudioTracks {
		source := track.Source
		sourceKey, err := storage.KeyFromStorageURL(input.WorkspaceID, source.StorageURL)
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
		role := normalizeComposeAudioRole(track.Role)
		sourcePath := path.Join(AssetsDir, fmt.Sprintf("%s%s", role, extensionForMIME(source.Mime)))
		if role == "audio" {
			sourcePath = path.Join(AssetsDir, fmt.Sprintf("compose-audio-%02d-%s%s", index+1, safePathComponent(source.AssetID), extensionForMIME(source.Mime)))
		}
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
		track.Role = role
		audioTracks = append(audioTracks, composeAudioTrackPath{ComposeAudioTrackInput: track, WorkspacePath: sourcePath})
	}

	outputPath := path.Join(OutputDir, "final-"+uuidString(job.ID)+".mp4")
	command := composeVideosCommand(sourcePaths, outputPath, path.Join(AssetsDir, "concat-"+uuidString(job.ID)+".txt"))
	if len(audioTracks) > 0 {
		command = composeVideosWithAudioCommand(sourcePaths, audioTracks, outputPath)
	}
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
	if info.Mime != "video/mp4" {
		err := fmt.Errorf("unexpected composed video MIME %q", info.Mime)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_invalid", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	objectKey := "production/" + uuidString(input.TargetNodeID) + "/" + uuidString(job.ID) + ".mp4"
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
		"output_path":       outputPath,
		"storage_url":       asset.StorageURL,
		"mime":              info.Mime,
		"size_bytes":        info.SizeBytes,
		"audio_track_count": len(audioTracks),
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

func (s *JobService) ImportRemoteAsset(ctx context.Context, input RemoteAssetInput) (SandboxJobResult, error) {
	if s.manager == nil || s.client == nil || s.repo == nil || s.storage == nil {
		return SandboxJobResult{}, errors.New("sandbox remote asset import is not configured")
	}
	sourceURL := strings.TrimSpace(input.SourceURL)
	if err := validatePresignedURL(sourceURL); err != nil {
		return SandboxJobResult{}, err
	}
	inputJSON, err := json.Marshal(map[string]any{
		"source_url": sourceURL,
		"mime_hint":  strings.TrimSpace(input.MimeHint),
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err := s.repo.CreateSandboxJob(ctx, db.CreateSandboxJobParams{
		WorkspaceID:   input.WorkspaceID,
		TargetNodeID:  input.TargetNodeID,
		JobType:       "provider_asset",
		OperationType: "import_remote_asset",
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

	assetPath := path.Join(AssetsDir, "provider-"+uuidString(job.ID)+extensionForMIME(input.MimeHint))
	job, err = s.repo.MarkSandboxJobRunning(ctx, db.MarkSandboxJobRunningParams{
		ID:        job.ID,
		SandboxID: pgtype.Text{String: workspaceSandbox.SandboxID, Valid: true},
		Command:   "curl -sS -f -L -o " + shellQuote(assetPath) + " " + shellQuote(sourceURL),
		Cwd:       DefaultWorkdir,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	downloadResult, err := DownloadURLToSandbox(ctx, s.client, workspaceSandbox.SandboxID, sourceURL, assetPath)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, downloadResult, "sandbox_provider_download_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: downloadResult}, err
	}
	if downloadResult.ExitCode != 0 {
		message := strings.TrimSpace(downloadResult.Stderr)
		failed, failErr := s.markFailed(ctx, job.ID, downloadResult, "sandbox_provider_download_failed", message, nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: downloadResult}, fmt.Errorf("sandbox provider download failed: %s", message)
	}
	info, err := InspectArtifact(ctx, s.client, workspaceSandbox.SandboxID, assetPath)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, downloadResult, "sandbox_provider_asset_inspect_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: downloadResult}, err
	}
	if err := ValidateArtifactSize(info.SizeBytes); err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, downloadResult, "sandbox_provider_asset_too_large", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: downloadResult}, err
	}
	if err := s.storage.EnsureBucket(ctx, input.WorkspaceID); err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, downloadResult, "sandbox_output_upload_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: downloadResult}, err
	}
	var thumbnail ArtifactObject
	if isVideoMIME(info.Mime) {
		thumbnailPath := path.Join(OutputDir, "thumbnail-"+uuidString(job.ID)+".png")
		execResult, err := RunExec(ctx, s.client, workspaceSandbox.SandboxID, ExecInput{
			Command:        frameExtractCommand(assetPath, thumbnailPath, FrameFirst),
			Cwd:            DefaultWorkdir,
			TimeoutSeconds: MaxExecTimeoutSeconds,
		})
		if err != nil {
			failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_thumbnail_extract_failed", err.Error(), nil)
			if failErr != nil {
				return SandboxJobResult{}, failErr
			}
			return SandboxJobResult{Job: failed, Exec: execResult}, err
		}
		if execResult.ExitCode != 0 {
			message := strings.TrimSpace(execResult.Stderr)
			failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_thumbnail_extract_failed", message, nil)
			if failErr != nil {
				return SandboxJobResult{}, failErr
			}
			return SandboxJobResult{Job: failed, Exec: execResult}, fmt.Errorf("sandbox thumbnail extraction failed: %s", message)
		}
		thumbnailInfo, err := InspectArtifact(ctx, s.client, workspaceSandbox.SandboxID, thumbnailPath)
		if err != nil {
			failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_thumbnail_inspect_failed", err.Error(), nil)
			if failErr != nil {
				return SandboxJobResult{}, failErr
			}
			return SandboxJobResult{Job: failed, Exec: execResult}, err
		}
		if thumbnailInfo.Mime != "image/png" {
			err := fmt.Errorf("unexpected thumbnail MIME %q", thumbnailInfo.Mime)
			failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_thumbnail_invalid", err.Error(), nil)
			if failErr != nil {
				return SandboxJobResult{}, failErr
			}
			return SandboxJobResult{Job: failed, Exec: execResult}, err
		}
		thumbnailKey := "production/" + uuidString(input.TargetNodeID) + "/" + uuidString(job.ID) + "-thumbnail.png"
		putURL, err := s.storage.PresignedSandboxPutURL(ctx, input.WorkspaceID, thumbnailKey, time.Hour)
		if err != nil {
			failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_thumbnail_upload_failed", err.Error(), nil)
			if failErr != nil {
				return SandboxJobResult{}, failErr
			}
			return SandboxJobResult{Job: failed, Exec: execResult}, err
		}
		uploadResult, err := UploadToMinIO(ctx, s.client, workspaceSandbox.SandboxID, thumbnailPath, putURL)
		if err != nil {
			failed, failErr := s.markFailed(ctx, job.ID, uploadResult, "sandbox_thumbnail_upload_failed", err.Error(), nil)
			if failErr != nil {
				return SandboxJobResult{}, failErr
			}
			return SandboxJobResult{Job: failed, Exec: uploadResult}, err
		}
		if uploadResult.ExitCode != 0 {
			message := strings.TrimSpace(uploadResult.Stderr)
			failed, failErr := s.markFailed(ctx, job.ID, uploadResult, "sandbox_thumbnail_upload_failed", message, nil)
			if failErr != nil {
				return SandboxJobResult{}, failErr
			}
			return SandboxJobResult{Job: failed, Exec: uploadResult}, fmt.Errorf("sandbox thumbnail upload failed: %s", message)
		}
		thumbnail = ArtifactObject{StorageURL: s.storage.StorageURL(input.WorkspaceID, thumbnailKey)}
	}
	objectKey := "production/" + uuidString(input.TargetNodeID) + "/" + uuidString(job.ID) + extensionForMIME(info.Mime)
	putURL, err := s.storage.PresignedSandboxPutURL(ctx, input.WorkspaceID, objectKey, time.Hour)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, downloadResult, "sandbox_output_upload_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: downloadResult}, err
	}
	uploadResult, err := UploadToMinIO(ctx, s.client, workspaceSandbox.SandboxID, assetPath, putURL)
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
		"source_url":    sourceURL,
		"asset_path":    assetPath,
		"storage_url":   asset.StorageURL,
		"thumbnail_url": thumbnail.StorageURL,
		"mime":          info.Mime,
		"size_bytes":    info.SizeBytes,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err = s.repo.MarkSandboxJobSucceeded(ctx, db.MarkSandboxJobSucceededParams{
		ID:         job.ID,
		Output:     outputJSON,
		ExitCode:   pgtype.Int4{Int32: int32(uploadResult.ExitCode), Valid: true},
		Stdout:     uploadResult.Stdout,
		Stderr:     uploadResult.Stderr,
		DurationMs: int32(uploadResult.DurationMS),
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	return SandboxJobResult{Job: job, Exec: uploadResult, Asset: asset, Thumbnail: thumbnail, MIME: info.Mime, Size: info.SizeBytes}, nil
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

func composeVideosCommand(sourcePaths []string, outputPath string, concatListPath string) string {
	var list strings.Builder
	for _, sourcePath := range sourcePaths {
		fmt.Fprintf(&list, "file %s\n", shellQuote(sourcePath))
	}
	return "cat > " + shellQuote(concatListPath) + " <<'EOF'\n" +
		list.String() +
		"EOF\n" +
		"ffmpeg -y -f concat -safe 0 -i " + shellQuote(concatListPath) + " -c copy " + shellQuote(outputPath)
}

type composeAudioTrackPath struct {
	ComposeAudioTrackInput
	WorkspacePath string
}

func composeVideosWithAudioCommand(sourcePaths []string, audioTracks []composeAudioTrackPath, outputPath string) string {
	args := []string{"ffmpeg", "-y"}
	for _, sourcePath := range sourcePaths {
		args = append(args, "-i", shellQuote(sourcePath))
	}
	for _, track := range audioTracks {
		if track.Role == "bgm" {
			args = append(args, "-stream_loop", "-1")
		}
		args = append(args, "-i", shellQuote(track.WorkspacePath))
	}
	filter := composeVideosAudioFilterGraph(len(sourcePaths), audioTracks)
	args = append(args,
		"-filter_complex", shellQuote(filter),
		"-map", shellQuote("[vout]"),
		"-map", shellQuote("[aout]"),
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-shortest",
		shellQuote(outputPath),
	)
	return strings.Join(args, " ")
}

func composeVideosAudioFilterGraph(videoCount int, audioTracks []composeAudioTrackPath) string {
	parts := []string{}
	if videoCount == 1 {
		parts = append(parts, "[0:v]setpts=PTS-STARTPTS,format=yuv420p,setsar=1[vout]")
	} else {
		videoInputs := make([]string, 0, videoCount)
		for index := 0; index < videoCount; index++ {
			videoInputs = append(videoInputs, fmt.Sprintf("[%d:v]", index))
		}
		parts = append(parts, strings.Join(videoInputs, "")+fmt.Sprintf("concat=n=%d:v=1:a=0[vcat]", videoCount))
		parts = append(parts, "[vcat]format=yuv420p,setsar=1[vout]")
	}
	audioLabels := []string{}
	voiceoverLabel := ""
	duckingLabels := map[string]ComposeAudioDuckingInput{}
	for index, track := range audioTracks {
		inputIndex := videoCount + index
		label := fmt.Sprintf("a%d", index)
		volume := track.Volume
		if volume <= 0 {
			volume = 1
		}
		filter := fmt.Sprintf("[%d:a]atrim=start=0", inputIndex)
		if track.DurationSec > 0 {
			filter += fmt.Sprintf(":duration=%.3f", track.DurationSec)
		}
		filter += ",asetpts=PTS-STARTPTS"
		if track.StartSec > 0 {
			delayMS := int(track.StartSec * 1000)
			filter += fmt.Sprintf(",adelay=%d|%d", delayMS, delayMS)
		}
		filter += fmt.Sprintf(",volume=%.3f", volume)
		if track.FadeInSec > 0 {
			filter += fmt.Sprintf(",afade=t=in:st=0:d=%.3f", track.FadeInSec)
		}
		if track.FadeOutSec > 0 && track.DurationSec > 0 {
			start := track.DurationSec - track.FadeOutSec
			if start < 0 {
				start = 0
			}
			filter += fmt.Sprintf(",afade=t=out:st=%.3f:d=%.3f", start, track.FadeOutSec)
		}
		filter += fmt.Sprintf("[%s]", label)
		parts = append(parts, filter)
		if track.Role == "voiceover" && voiceoverLabel == "" {
			voiceoverLabel = label
		}
		if track.Role == "bgm" && track.Ducking.SidechainRole == "voiceover" {
			duckingLabels[label] = track.Ducking
			continue
		}
		audioLabels = append(audioLabels, label)
	}
	for label, ducking := range duckingLabels {
		if voiceoverLabel == "" {
			audioLabels = append(audioLabels, label)
			continue
		}
		if ducking.Threshold <= 0 {
			ducking.Threshold = 0.08
		}
		if ducking.Ratio <= 0 {
			ducking.Ratio = 8
		}
		if ducking.AttackMS <= 0 {
			ducking.AttackMS = 20
		}
		if ducking.ReleaseMS <= 0 {
			ducking.ReleaseMS = 250
		}
		out := label + "duck"
		parts = append(parts, fmt.Sprintf("[%s][%s]sidechaincompress=threshold=%.3f:ratio=%.3f:attack=%d:release=%d[%s]", label, voiceoverLabel, ducking.Threshold, ducking.Ratio, ducking.AttackMS, ducking.ReleaseMS, out))
		audioLabels = append(audioLabels, out)
	}
	if len(audioLabels) == 1 {
		parts = append(parts, fmt.Sprintf("[%s]anull[aout]", audioLabels[0]))
	} else {
		inputs := make([]string, 0, len(audioLabels))
		for _, label := range audioLabels {
			inputs = append(inputs, fmt.Sprintf("[%s]", label))
		}
		parts = append(parts, strings.Join(inputs, "")+fmt.Sprintf("amix=inputs=%d:duration=shortest:dropout_transition=0[aout]", len(audioLabels)))
	}
	return strings.Join(parts, ";")
}

func normalizeComposeAudioRole(role string) string {
	switch strings.TrimSpace(role) {
	case "voiceover":
		return "voiceover"
	case "bgm":
		return "bgm"
	default:
		return "audio"
	}
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

func safePathComponent(value string) string {
	name := SafeAssetName(strings.TrimSpace(value))
	if name == "" {
		return "input"
	}
	return name
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
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "audio/L16":
		return ".pcm"
	default:
		return ".bin"
	}
}

func isVideoMIME(mime string) bool {
	switch strings.TrimSpace(mime) {
	case "video/mp4", "video/quicktime", "video/webm":
		return true
	default:
		return false
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
