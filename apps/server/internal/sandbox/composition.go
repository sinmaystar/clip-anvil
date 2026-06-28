package sandbox

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/storage"
)

type StageMediaInputsInput struct {
	WorkspaceID  pgtype.UUID
	TargetNodeID pgtype.UUID
	Assets       []StageMediaAssetInput
	TargetDir    string
}

type StageMediaAssetInput struct {
	AssetID   pgtype.UUID
	SourceURL string
	FileName  string
	MimeType  string
	SizeBytes int64
}

type StageMediaInputsResult struct {
	Files []StageMediaFile `json:"files"`
}

type StageMediaFile struct {
	AssetID       pgtype.UUID `json:"asset_id"`
	WorkspacePath string      `json:"workspace_path"`
	FileName      string      `json:"file_name"`
	MimeType      string      `json:"mime_type,omitempty"`
	SizeBytes     int64       `json:"size_bytes,omitempty"`
}

type ProbeMediaInput struct {
	WorkspaceID   pgtype.UUID
	TargetNodeID  pgtype.UUID
	WorkspacePath string
}

type ProbeMediaResult struct {
	Format  map[string]any   `json:"format"`
	Streams []map[string]any `json:"streams"`
}

type RunFFmpegCommandInput struct {
	WorkspaceID  pgtype.UUID
	TargetNodeID pgtype.UUID
	Executable   string
	Cwd          string
	Args         []string
	TimeoutSec   int
}

func (s *JobService) StageMediaInputs(ctx context.Context, input StageMediaInputsInput) (StageMediaInputsResult, error) {
	if s.manager == nil || s.client == nil || s.storage == nil {
		return StageMediaInputsResult{}, errors.New("sandbox media staging is not configured")
	}
	if len(input.Assets) == 0 {
		return StageMediaInputsResult{}, errors.New("at least one media asset is required")
	}
	targetDir := strings.TrimSpace(input.TargetDir)
	if targetDir == "" {
		targetDir = InputDir
	}
	if err := validateTransferPath(targetDir); err != nil {
		return StageMediaInputsResult{}, err
	}
	workspaceSandbox, err := s.manager.EnsureSandbox(ctx, input.WorkspaceID)
	if err != nil {
		return StageMediaInputsResult{}, err
	}
	if err := EnsureWorkspaceLayout(ctx, s.client, workspaceSandbox.SandboxID); err != nil {
		return StageMediaInputsResult{}, err
	}
	if _, err := RunExec(ctx, s.client, workspaceSandbox.SandboxID, ExecInput{Command: "mkdir -p " + targetDir, TimeoutSeconds: 30}); err != nil {
		return StageMediaInputsResult{}, err
	}

	used := map[string]struct{}{}
	result := StageMediaInputsResult{Files: make([]StageMediaFile, 0, len(input.Assets))}
	for _, asset := range input.Assets {
		fileName := uniqueStageFileName(asset.FileName, asset.AssetID, used)
		destPath := path.Join(targetDir, fileName)
		sourceURL, err := s.stageSourceURL(ctx, input.WorkspaceID, asset.SourceURL)
		if err != nil {
			return StageMediaInputsResult{}, err
		}
		downloadResult, err := DownloadURLToSandbox(ctx, s.client, workspaceSandbox.SandboxID, sourceURL, destPath)
		if err != nil {
			return StageMediaInputsResult{}, err
		}
		if downloadResult.ExitCode != 0 {
			message := strings.TrimSpace(downloadResult.Stderr)
			if message == "" {
				message = fmt.Sprintf("sandbox download exited with code %d", downloadResult.ExitCode)
			}
			return StageMediaInputsResult{}, errors.New(message)
		}
		result.Files = append(result.Files, StageMediaFile{
			AssetID:       asset.AssetID,
			WorkspacePath: destPath,
			FileName:      fileName,
			MimeType:      asset.MimeType,
			SizeBytes:     asset.SizeBytes,
		})
	}
	return result, nil
}

func (s *JobService) ProbeMedia(ctx context.Context, input ProbeMediaInput) (ProbeMediaResult, error) {
	if err := validateFFmpegWorkspacePath(DefaultWorkdir, input.WorkspacePath); err != nil {
		return ProbeMediaResult{}, err
	}
	result, err := s.RunFFmpegCommand(ctx, RunFFmpegCommandInput{
		WorkspaceID:  input.WorkspaceID,
		TargetNodeID: input.TargetNodeID,
		Executable:   "ffprobe",
		Cwd:          DefaultWorkdir,
		Args: []string{
			"-v", "error",
			"-print_format", "json",
			"-show_format",
			"-show_streams",
			input.WorkspacePath,
		},
		TimeoutSec: DefaultExecTimeoutSeconds,
	})
	if err != nil {
		return ProbeMediaResult{}, err
	}
	var probe ProbeMediaResult
	if err := json.Unmarshal([]byte(result.Exec.Stdout), &probe); err != nil {
		return ProbeMediaResult{}, fmt.Errorf("parse ffprobe json: %w", err)
	}
	return probe, nil
}

func (s *JobService) RunFFmpegCommand(ctx context.Context, input RunFFmpegCommandInput) (SandboxJobResult, error) {
	command, cwd, err := buildFFmpegCommand(input)
	if err != nil {
		return SandboxJobResult{}, err
	}
	return s.RunCommand(ctx, SandboxJobInput{
		WorkspaceID:    input.WorkspaceID,
		TargetNodeID:   input.TargetNodeID,
		JobType:        "internal_media",
		OperationType:  "run_ffmpeg_command",
		Command:        command,
		Cwd:            cwd,
		TimeoutSeconds: normalizeFFmpegTimeout(input.TimeoutSec),
		Input: map[string]any{
			"executable": input.Executable,
			"args":       input.Args,
		},
	})
}

func (s *JobService) stageSourceURL(ctx context.Context, workspaceID pgtype.UUID, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("source_url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return rawURL, nil
	}
	key, err := storage.KeyFromStorageURL(workspaceID, rawURL)
	if err != nil {
		return "", err
	}
	return s.storage.PresignedSandboxGetURL(ctx, workspaceID, key, time.Hour)
}

func buildFFmpegCommand(input RunFFmpegCommandInput) (string, string, error) {
	executable := strings.TrimSpace(input.Executable)
	if executable != "ffmpeg" && executable != "ffprobe" {
		return "", "", errors.New("executable must be ffmpeg or ffprobe")
	}
	cwd := strings.TrimSpace(input.Cwd)
	if cwd == "" {
		cwd = DefaultWorkdir
	}
	if cwd != DefaultWorkdir && !strings.HasPrefix(cwd, DefaultWorkdir+"/") {
		return "", "", errors.New("cwd must be inside /workspace")
	}
	cwd = path.Clean(cwd)
	parts := make([]string, 0, len(input.Args)+1)
	parts = append(parts, executable)
	for _, arg := range input.Args {
		if strings.TrimSpace(arg) == "" {
			return "", "", errors.New("ffmpeg args cannot contain empty values")
		}
		if err := validateFFmpegArgPath(cwd, arg); err != nil {
			return "", "", err
		}
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " "), cwd, nil
}

func validateFFmpegArgPath(cwd string, arg string) error {
	if strings.HasPrefix(arg, "-") {
		return nil
	}
	if strings.Contains(arg, "://") {
		return nil
	}
	if strings.HasPrefix(arg, "file:") {
		return validateFFmpegWorkspacePath(cwd, strings.TrimPrefix(arg, "file:"))
	}
	if strings.Contains(arg, "..") {
		return validateFFmpegWorkspacePath(cwd, arg)
	}
	if strings.HasPrefix(arg, "/") {
		return validateFFmpegWorkspacePath(cwd, arg)
	}
	if strings.Contains(arg, "/") {
		return validateFFmpegWorkspacePath(cwd, arg)
	}
	return nil
}

func validateFFmpegWorkspacePath(cwd string, input string) error {
	input = strings.TrimSpace(input)
	if input == "" {
		return errors.New("path is required")
	}
	var clean string
	if strings.HasPrefix(input, "/") {
		clean = path.Clean(input)
	} else {
		clean = path.Clean(path.Join(cwd, input))
	}
	if clean != DefaultWorkdir && !strings.HasPrefix(clean, DefaultWorkdir+"/") {
		return fmt.Errorf("path %q must stay inside /workspace", input)
	}
	return nil
}

func normalizeFFmpegTimeout(timeout int) int {
	if timeout == 0 {
		return DefaultExecTimeoutSeconds
	}
	if timeout > MaxExecTimeoutSeconds {
		return MaxExecTimeoutSeconds
	}
	return timeout
}

func uniqueStageFileName(fileName string, assetID pgtype.UUID, used map[string]struct{}) string {
	safe := SafeAssetName(fileName)
	if safe == "" || safe == "asset.bin" {
		safe = "media-" + shortUUID(assetID) + extensionForMIME("")
	}
	candidate := safe
	if _, ok := used[candidate]; ok {
		ext := path.Ext(safe)
		base := strings.TrimSuffix(safe, ext)
		if base == "" {
			base = "media"
		}
		candidate = base + "-" + shortUUID(assetID) + ext
	}
	for {
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return candidate
		}
		candidate = strings.TrimSuffix(candidate, path.Ext(candidate)) + "-" + shortUUID(assetID) + path.Ext(candidate)
	}
}

func shortUUID(id pgtype.UUID) string {
	if !id.Valid {
		return "unknown"
	}
	return hex.EncodeToString(id.Bytes[:4])
}
