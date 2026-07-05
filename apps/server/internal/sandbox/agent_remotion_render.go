package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type RenderAgentRemotionCodeInput struct {
	WorkspaceID         pgtype.UUID
	TargetNodeID        pgtype.UUID
	TimelinePlanID      pgtype.UUID
	RendererArtifactID  pgtype.UUID
	RendererAttemptID   pgtype.UUID
	AttemptWorkspaceDir string
	OutputPath          string
}

type AgentRemotionRenderMetadata struct {
	OutputPath         string  `json:"output_path"`
	MIME               string  `json:"mime"`
	SizeBytes          int64   `json:"size_bytes"`
	DurationSec        float64 `json:"duration_sec,omitempty"`
	Width              int     `json:"width,omitempty"`
	Height             int     `json:"height,omitempty"`
	VideoStream        bool    `json:"video_stream"`
	AudioStream        bool    `json:"audio_stream"`
	RendererEngine     string  `json:"renderer_engine"`
	RendererArtifactID string  `json:"renderer_artifact_id"`
	RendererAttemptID  string  `json:"renderer_attempt_id"`
}

func (s *JobService) RenderAgentRemotionCode(ctx context.Context, input RenderAgentRemotionCodeInput) (SandboxJobResult, error) {
	if s.manager == nil || s.client == nil || s.repo == nil {
		return SandboxJobResult{}, fmt.Errorf("sandbox agent remotion renderer is not configured")
	}
	outputPath, err := ValidateOutputPath(input.OutputPath)
	if err != nil {
		return SandboxJobResult{}, err
	}
	workdir, err := validateAgentRemotionWorkdir(input.AttemptWorkspaceDir)
	if err != nil {
		return SandboxJobResult{}, err
	}
	inputJSON, err := json.Marshal(map[string]any{
		"timeline_plan_id":      uuidString(input.TimelinePlanID),
		"renderer_artifact_id":  uuidString(input.RendererArtifactID),
		"renderer_attempt_id":   uuidString(input.RendererAttemptID),
		"attempt_workspace_dir": workdir,
		"output_path":           outputPath,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err := s.repo.CreateSandboxJob(ctx, db.CreateSandboxJobParams{
		WorkspaceID:   input.WorkspaceID,
		TargetNodeID:  input.TargetNodeID,
		JobType:       "internal_media",
		OperationType: "render_agent_remotion_code",
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
	command := agentRemotionRenderCommand(workdir, outputPath)
	job, err = s.repo.MarkSandboxJobRunning(ctx, db.MarkSandboxJobRunningParams{
		ID:        job.ID,
		SandboxID: pgtype.Text{String: workspaceSandbox.SandboxID, Valid: true},
		Command:   command,
		Cwd:       workdir,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	execResult, err := RunExec(ctx, s.client, workspaceSandbox.SandboxID, ExecInput{
		Command:        command,
		Cwd:            workdir,
		TimeoutSeconds: MaxExecTimeoutSeconds,
	})
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_agent_remotion_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	if execResult.ExitCode != 0 {
		message := strings.TrimSpace(execResult.Stderr)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_agent_remotion_failed", message, nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, fmt.Errorf("sandbox agent remotion failed: %s", message)
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
		err := fmt.Errorf("unexpected agent remotion MIME %q", info.Mime)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_invalid", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	metadata, err := ProbeVideoMetadata(ctx, s.client, workspaceSandbox.SandboxID, outputPath)
	if err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_probe_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	metadata.OutputPath = outputPath
	metadata.MIME = info.Mime
	metadata.SizeBytes = info.SizeBytes
	metadata.RendererEngine = "remotion"
	metadata.RendererArtifactID = uuidString(input.RendererArtifactID)
	metadata.RendererAttemptID = uuidString(input.RendererAttemptID)
	outputJSON, _ := json.Marshal(metadata)
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
	return SandboxJobResult{Job: job, Exec: execResult, MIME: info.Mime, Size: info.SizeBytes}, nil
}

func agentRemotionRenderCommand(workdir string, outputPath string) string {
	return "node /opt/clipanvil/remotion-agent-runtime/src/render.mjs --workdir " + shellQuote(workdir) +
		" --out " + shellQuote(outputPath) +
		" --browser-executable " + shellQuote("/usr/bin/chromium-headless-shell") +
		" --public-dir " + shellQuote(DefaultWorkdir)
}

func validateAgentRemotionWorkdir(workdir string) (string, error) {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return "", fmt.Errorf("attempt workspace dir is required")
	}
	if workdir != AgentRemotionDir && !strings.HasPrefix(workdir, AgentRemotionDir+"/") {
		return "", fmt.Errorf("attempt workspace dir must be inside %s", AgentRemotionDir)
	}
	return workdir, nil
}

func ProbeVideoMetadata(ctx context.Context, client Client, sandboxID string, outputPath string) (AgentRemotionRenderMetadata, error) {
	outputPath, err := ValidateOutputPath(outputPath)
	if err != nil {
		return AgentRemotionRenderMetadata{}, err
	}
	command := "ffprobe -v error -print_format json -show_streams -show_format " + shellQuote(outputPath)
	result, err := RunExec(ctx, client, sandboxID, ExecInput{Command: command, TimeoutSeconds: 30})
	if err != nil {
		return AgentRemotionRenderMetadata{}, err
	}
	if result.ExitCode != 0 {
		return AgentRemotionRenderMetadata{}, fmt.Errorf("ffprobe failed with exit code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	var payload struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		return AgentRemotionRenderMetadata{}, err
	}
	metadata := AgentRemotionRenderMetadata{}
	if payload.Format.Duration != "" {
		if duration, err := strconv.ParseFloat(payload.Format.Duration, 64); err == nil {
			metadata.DurationSec = duration
		}
	}
	for _, stream := range payload.Streams {
		switch stream.CodecType {
		case "video":
			metadata.VideoStream = true
			if metadata.Width == 0 {
				metadata.Width = stream.Width
			}
			if metadata.Height == 0 {
				metadata.Height = stream.Height
			}
		case "audio":
			metadata.AudioStream = true
		}
	}
	if !metadata.VideoStream {
		return AgentRemotionRenderMetadata{}, fmt.Errorf("ffprobe did not report a video stream")
	}
	return metadata, nil
}
