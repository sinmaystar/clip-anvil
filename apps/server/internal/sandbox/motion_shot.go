package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/motionshot"
	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type RenderMotionShotInput struct {
	WorkspaceID  pgtype.UUID
	TargetNodeID pgtype.UUID
	Plan         motionshot.Plan
	Meta         MotionShotMeta
	Assets       []RenderMotionAssetInput
}

type RenderMotionAssetInput struct {
	AssetID       string
	StorageURL    string
	Mime          string
	FileName      string
	WorkspacePath string
}

type MotionShotMeta struct {
	DurationSec int `json:"duration_sec"`
	Width       int `json:"width"`
	Height      int `json:"height"`
	FPS         int `json:"fps"`
}

func (s *JobService) RenderMotionShot(ctx context.Context, input RenderMotionShotInput) (SandboxJobResult, error) {
	if s.manager == nil || s.client == nil || s.repo == nil || s.storage == nil {
		return SandboxJobResult{}, fmt.Errorf("sandbox motion shot renderer is not configured")
	}
	inputJSON, err := json.Marshal(map[string]any{
		"duration_sec": input.Meta.DurationSec,
		"width":        input.Meta.Width,
		"height":       input.Meta.Height,
		"fps":          input.Meta.FPS,
		"asset_count":  len(input.Assets),
		"motion_style": strings.TrimSpace(input.Plan.MotionStyle),
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err := s.repo.CreateSandboxJob(ctx, db.CreateSandboxJobParams{
		WorkspaceID:   input.WorkspaceID,
		TargetNodeID:  input.TargetNodeID,
		JobType:       "internal_media",
		OperationType: "image_to_motion_video",
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
	projectDir := path.Join(DefaultWorkdir, "motion-shot", jobKey)
	assetDir := path.Join(projectDir, "assets")
	assetPaths := map[string]string{}
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
		assetFileName := safeMotionAssetFileName(asset)
		if destPath == "" {
			destPath = path.Join(assetDir, assetFileName)
		} else if !path.IsAbs(destPath) {
			cleanPath := path.Clean(destPath)
			if cleanPath == "." || strings.HasPrefix(cleanPath, "../") || cleanPath == ".." {
				failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_input_invalid", "invalid motion asset path: "+asset.WorkspacePath, nil)
				if failErr != nil {
					return SandboxJobResult{}, failErr
				}
				return SandboxJobResult{Job: failed}, fmt.Errorf("invalid motion asset path: %s", asset.WorkspacePath)
			}
			destPath = path.Join(projectDir, cleanPath)
		}
		indexMotionAssetPath(assetPaths, asset, assetFileName, motionProjectRelativePath(projectDir, destPath))
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

	plan := input.Plan
	normalizeMotionPlanAssetRefs(&plan, projectDir, assetPaths)
	if err := uploadMotionPlan(ctx, s.client, workspaceSandbox.SandboxID, projectDir, plan); err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_motion_plan_upload_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed}, err
	}

	outputPath := path.Join(OutputDir, "motion-"+jobKey+".mp4")
	command := motionRenderCommand(projectDir, outputPath)
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
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_remotion_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	if execResult.ExitCode != 0 {
		message := strings.TrimSpace(execResult.Stderr)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_remotion_failed", message, nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, fmt.Errorf("sandbox remotion failed: %s", message)
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
		err := fmt.Errorf("unexpected motion shot MIME %q", info.Mime)
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
	storageURL := s.storage.StorageURL(input.WorkspaceID, objectKey)
	outputJSON, _ := json.Marshal(map[string]any{
		"storage_url":     storageURL,
		"mime":            info.Mime,
		"size_bytes":      info.SizeBytes,
		"renderer_engine": "remotion",
	})
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
	return SandboxJobResult{
		Job:   job,
		Exec:  execResult,
		Asset: ArtifactObject{StorageURL: storageURL},
		MIME:  info.Mime,
		Size:  info.SizeBytes,
	}, nil
}

func uploadMotionPlan(ctx context.Context, client Client, sandboxID string, projectDir string, plan motionshot.Plan) error {
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return client.Upload(ctx, sandboxID, path.Join(projectDir, "motion-plan.json"), strings.NewReader(string(raw)))
}

func indexMotionAssetPath(paths map[string]string, asset RenderMotionAssetInput, fileName string, publicRef string) {
	for _, ref := range []string{
		strings.TrimSpace(asset.WorkspacePath),
		fileName,
		path.Join("assets", fileName),
		strings.TrimSpace(asset.AssetID),
	} {
		ref = strings.TrimSpace(ref)
		if ref == "" || path.IsAbs(ref) || strings.HasPrefix(ref, "file://") {
			continue
		}
		clean := path.Clean(ref)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			continue
		}
		paths[clean] = publicRef
	}
}

func normalizeMotionPlanAssetRefs(plan *motionshot.Plan, projectDir string, assetPaths map[string]string) {
	for i := range plan.VisualLayers {
		ref := strings.TrimSpace(plan.VisualLayers[i].InputRef)
		if ref == "" || strings.HasPrefix(ref, "file://") || strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "data:") {
			continue
		}
		if path.IsAbs(ref) && strings.HasPrefix(ref, projectDir+"/") {
			plan.VisualLayers[i].InputRef = strings.TrimPrefix(ref, projectDir+"/")
			continue
		}
		if path.IsAbs(ref) {
			continue
		}
		clean := path.Clean(ref)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			continue
		}
		if destPath := assetPaths[clean]; destPath != "" {
			plan.VisualLayers[i].InputRef = destPath
			continue
		}
		plan.VisualLayers[i].InputRef = clean
	}
}

func motionProjectRelativePath(projectDir string, destPath string) string {
	prefix := strings.TrimRight(projectDir, "/") + "/"
	if strings.HasPrefix(destPath, prefix) {
		return strings.TrimPrefix(destPath, prefix)
	}
	return path.Base(destPath)
}

func motionRenderCommand(projectDir string, outputPath string) string {
	return "/opt/clipanvil/remotion-motion-shot/node_modules/.bin/remotion render /opt/clipanvil/remotion-motion-shot/src/index.tsx MotionShot " + shellQuote(outputPath) + " --props=motion-plan.json --codec=h264 --browser-executable=/usr/bin/chromium-headless-shell --public-dir=" + shellQuote(projectDir) + " --overwrite"
}

func safeMotionAssetFileName(asset RenderMotionAssetInput) string {
	name := SafeAssetName(strings.TrimSpace(asset.FileName))
	if name == "" || name == "asset.bin" {
		name = SafeAssetName(strings.TrimSpace(asset.AssetID))
	}
	if name == "" || name == "asset.bin" {
		name = "product" + extensionForMIME(asset.Mime)
	}
	if path.Ext(name) == "" {
		name += extensionForMIME(asset.Mime)
	}
	return name
}
