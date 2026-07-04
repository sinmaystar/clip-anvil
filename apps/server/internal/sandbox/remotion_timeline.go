package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func (s *JobService) RenderRemotionTimeline(ctx context.Context, input RenderRemotionTimelineInput) (SandboxJobResult, error) {
	if s.manager == nil || s.client == nil || s.repo == nil {
		return SandboxJobResult{}, fmt.Errorf("sandbox remotion timeline renderer is not configured")
	}
	outputPath, err := ValidateOutputPath(input.OutputPath)
	if err != nil {
		return SandboxJobResult{}, err
	}
	inputJSON, err := json.Marshal(map[string]any{
		"timeline_plan_id": uuidString(input.TimelinePlanID),
		"schema":           input.Plan.Schema,
		"composition":      input.Plan.Composition,
		"duration_sec":     input.Plan.Output.DurationSec,
		"width":            input.Plan.Output.Width,
		"height":           input.Plan.Output.Height,
		"fps":              input.Plan.Output.FPS,
		"segment_count":    len(input.Plan.Segments),
		"audio_count":      len(input.Plan.AudioTracks),
		"output_path":      outputPath,
	})
	if err != nil {
		return SandboxJobResult{}, err
	}
	job, err := s.repo.CreateSandboxJob(ctx, db.CreateSandboxJobParams{
		WorkspaceID:   input.WorkspaceID,
		TargetNodeID:  input.TargetNodeID,
		JobType:       "internal_media",
		OperationType: "render_remotion_timeline",
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
	projectDir := path.Join(DefaultWorkdir, "remotion-timeline", jobKey)
	planPath := path.Join(projectDir, "timeline-plan.json")
	if err := uploadRemotionTimelinePlan(ctx, s.client, workspaceSandbox.SandboxID, planPath, input.Plan); err != nil {
		failed, failErr := s.markFailed(ctx, job.ID, ExecResult{}, "sandbox_timeline_plan_upload_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed}, err
	}

	command := remotionTimelineRenderCommand(planPath, outputPath)
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
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_remotion_timeline_failed", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	if execResult.ExitCode != 0 {
		message := strings.TrimSpace(execResult.Stderr)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_remotion_timeline_failed", message, nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, fmt.Errorf("sandbox remotion timeline failed: %s", message)
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
		err := fmt.Errorf("unexpected remotion timeline MIME %q", info.Mime)
		failed, failErr := s.markFailed(ctx, job.ID, execResult, "sandbox_output_invalid", err.Error(), nil)
		if failErr != nil {
			return SandboxJobResult{}, failErr
		}
		return SandboxJobResult{Job: failed, Exec: execResult}, err
	}
	outputJSON, _ := json.Marshal(map[string]any{
		"output_path":     outputPath,
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
	return SandboxJobResult{Job: job, Exec: execResult, MIME: info.Mime, Size: info.SizeBytes}, nil
}

func uploadRemotionTimelinePlan(ctx context.Context, client Client, sandboxID string, planPath string, plan any) error {
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return client.Upload(ctx, sandboxID, planPath, strings.NewReader(string(raw)))
}

func remotionTimelineRenderCommand(planPath string, outputPath string) string {
	return "node /opt/clipanvil/remotion-timeline/src/render.mjs --props " + shellQuote(planPath) + " --out " + shellQuote(outputPath) + " --browser-executable /usr/bin/chromium-headless-shell"
}
