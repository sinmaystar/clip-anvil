package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type RenderTemplateVideoInput struct {
	WorkspaceID  pgtype.UUID
	TargetNodeID pgtype.UUID
	TemplateKey  string
	HTML         string
	Meta         TemplateVideoMeta
	Variables    map[string]any
	Assets       []RenderTemplateAssetInput
}

type RenderTemplateAssetInput struct {
	AssetID       string
	StorageURL    string
	Mime          string
	FileName      string
	WorkspacePath string
}

type TemplateVideoMeta struct {
	DurationSec int `json:"duration_sec"`
	Width       int `json:"width"`
	Height      int `json:"height"`
	FPS         int `json:"fps"`
}

func (s *JobService) RenderTemplateVideo(ctx context.Context, input RenderTemplateVideoInput) (SandboxJobResult, error) {
	if s.manager == nil || s.client == nil || s.repo == nil || s.storage == nil {
		return SandboxJobResult{}, fmt.Errorf("sandbox template video renderer is not configured")
	}
	fps := input.Meta.FPS
	if fps == 0 {
		fps = 24
	}
	inputJSON, err := json.Marshal(map[string]any{
		"template_key": strings.TrimSpace(input.TemplateKey),
		"duration_sec": input.Meta.DurationSec,
		"width":        input.Meta.Width,
		"height":       input.Meta.Height,
		"fps":          fps,
		"asset_count":  len(input.Assets),
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err := s.repo.CreateSandboxJob(ctx, db.CreateSandboxJobParams{
		WorkspaceID:   input.WorkspaceID,
		TargetNodeID:  input.TargetNodeID,
		JobType:       "internal_media",
		OperationType: "template_to_video",
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

	jobKey := uuidString(job.ID)
	projectDir := path.Join(DefaultWorkdir, "template-video", jobKey)
	assetDir := path.Join(projectDir, "assets")
	assetCheckPaths := make([]string, 0, len(input.Assets))
	for _, asset := range input.Assets {
		sourceKey, err := storage.KeyFromStorageURL(input.WorkspaceID, asset.StorageURL)
		if err != nil {
			failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_input_invalid", "invalid storage URL: "+err.Error(), nil)
			if failErr != nil {
				return SandboxJobResult{}, failErr
			}
			return SandboxJobResult{Job: failed}, fmt.Errorf("invalid storage URL: %w", err)
		}
		getURL, err := s.storage.PresignedSandboxGetURL(ctx, input.WorkspaceID, sourceKey, time.Hour)
		if err != nil {
			failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_input_presign_failed", err.Error(), nil)
			if failErr != nil {
				return SandboxJobResult{}, failErr
			}
			return SandboxJobResult{Job: failed}, err
		}
		destPath := strings.TrimSpace(asset.WorkspacePath)
		if destPath == "" {
			destPath = path.Join(assetDir, safeTemplateAssetFileName(asset))
		} else if !path.IsAbs(destPath) {
			cleanPath := path.Clean(destPath)
			if cleanPath == "." || strings.HasPrefix(cleanPath, "../") || cleanPath == ".." {
				failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_input_invalid", "invalid template asset path: "+asset.WorkspacePath, nil)
				if failErr != nil {
					return SandboxJobResult{}, failErr
				}
				return SandboxJobResult{Job: failed}, fmt.Errorf("invalid template asset path: %s", asset.WorkspacePath)
			}
			destPath = path.Join(projectDir, cleanPath)
		}
		assetCheckPaths = append(assetCheckPaths, destPath)
		downloadResult, err := DownloadFromMinIO(ctx, s.client, workspaceSandbox.SandboxID, getURL, destPath)
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
	}

	if err := uploadTemplateProject(ctx, s.client, workspaceSandbox.SandboxID, projectDir, input.HTML, input.Meta, input.Variables); err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_template_upload_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed}, err
	}

	outputPath := path.Join(OutputDir, "template-"+jobKey+".mp4")
	command := templateRenderCommand(outputPath, fps, assetCheckPaths)
	job, err = s.repo.MarkSandboxJobRunning(ctx, db.MarkSandboxJobRunningParams{
		ID:        job.ID,
		SandboxID: pgtype.Text{String: workspaceSandbox.SandboxID, Valid: true},
		Command:   command,
		Cwd:       projectDir,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	execResult, err := RunExec(ctx, s.client, workspaceSandbox.SandboxID, ExecInput{
		Command:        command,
		Cwd:            projectDir,
		TimeoutSeconds: MaxExecTimeoutSeconds,
	})
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_hyperframes_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	if execResult.ExitCode != 0 {
		message := strings.TrimSpace(execResult.Stderr)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_hyperframes_failed", message, nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, fmt.Errorf("sandbox hyperframes failed: %s", message)
	}
	info, err := InspectArtifact(ctx, s.client, workspaceSandbox.SandboxID, outputPath)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_inspect_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	if err := ValidateArtifactSize(info.SizeBytes); err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_too_large", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	if !strings.HasPrefix(info.Mime, "video/") {
		err := fmt.Errorf("unexpected template video MIME %q", info.Mime)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_invalid", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	objectKey := "production/" + uuidString(input.TargetNodeID) + "/" + jobKey + extensionForMIME(info.Mime)
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
		"template_key": strings.TrimSpace(input.TemplateKey),
		"output_path":  outputPath,
		"storage_url":  asset.StorageURL,
		"mime":         info.Mime,
		"size_bytes":   info.SizeBytes,
		"width":        input.Meta.Width,
		"height":       input.Meta.Height,
		"fps":          fps,
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
	return SandboxJobResult{Job: job, Exec: uploadResult, Asset: asset, MIME: info.Mime, Size: info.SizeBytes}, nil
}

func uploadTemplateProject(ctx context.Context, client Client, sandboxID string, projectDir string, html string, meta TemplateVideoMeta, variables map[string]any) error {
	if err := validateTransferPath(projectDir); err != nil {
		return err
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	variablesJSON, err := json.MarshalIndent(variables, "", "  ")
	if err != nil {
		return err
	}
	files := map[string]string{
		"index.html":     html,
		"meta.json":      string(metaJSON),
		"variables.json": string(variablesJSON),
	}
	for name, content := range files {
		if err := client.Upload(ctx, sandboxID, path.Join(projectDir, name), strings.NewReader(content)); err != nil {
			return err
		}
	}
	return nil
}

func templateRenderCommand(outputPath string, fps int, assetCheckPaths []string) string {
	parts := make([]string, 0, len(assetCheckPaths)+1)
	for _, assetPath := range assetCheckPaths {
		parts = append(parts, "test -s "+shellQuote(assetPath))
	}
	parts = append(parts, "hyperframes render . --output "+shellQuote(outputPath)+" --fps "+fmt.Sprintf("%d", fps)+" --quality draft")
	return strings.Join(parts, " && ")
}

func safeTemplateAssetFileName(asset RenderTemplateAssetInput) string {
	fileName := strings.TrimSpace(asset.FileName)
	if fileName != "" {
		base := SafeAssetName(strings.TrimSuffix(fileName, path.Ext(fileName)))
		if base == "" {
			base = safePathComponent(asset.AssetID)
		}
		ext := path.Ext(fileName)
		if ext == "" {
			ext = extensionForMIME(asset.Mime)
		}
		return base + ext
	}
	return safeInputName(asset.AssetID, asset.Mime)
}
