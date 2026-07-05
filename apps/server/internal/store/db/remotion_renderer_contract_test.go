package db

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type remotionRendererQueriesContract interface {
	CreateRemotionRendererArtifact(context.Context, CreateRemotionRendererArtifactParams) (RemotionRendererArtifact, error)
	GetRemotionRendererArtifact(context.Context, pgtype.UUID) (RemotionRendererArtifact, error)
	GetRemotionRendererArtifactByTimelinePlan(context.Context, pgtype.UUID) (RemotionRendererArtifact, error)
	ListRemotionRendererArtifactsByWorkspace(context.Context, ListRemotionRendererArtifactsByWorkspaceParams) ([]RemotionRendererArtifact, error)
	UpdateRemotionRendererArtifactStatus(context.Context, UpdateRemotionRendererArtifactStatusParams) (RemotionRendererArtifact, error)
	SetCurrentRemotionRendererAttempt(context.Context, SetCurrentRemotionRendererAttemptParams) (RemotionRendererArtifact, error)
	CreateRemotionRendererAttempt(context.Context, CreateRemotionRendererAttemptParams) (RemotionRendererAttempt, error)
	GetRemotionRendererAttempt(context.Context, pgtype.UUID) (RemotionRendererAttempt, error)
	GetCurrentRemotionRendererAttempt(context.Context, pgtype.UUID) (RemotionRendererAttempt, error)
	ListRemotionRendererAttemptsByArtifact(context.Context, pgtype.UUID) ([]RemotionRendererAttempt, error)
	GetLatestRemotionRendererAttemptByArtifact(context.Context, pgtype.UUID) (RemotionRendererAttempt, error)
	UpdateRemotionRendererAttemptSnapshot(context.Context, UpdateRemotionRendererAttemptSnapshotParams) (RemotionRendererAttempt, error)
	UpdateRemotionRendererAttemptRenderResult(context.Context, UpdateRemotionRendererAttemptRenderResultParams) (RemotionRendererAttempt, error)
	UpdateRemotionRendererAttemptQAResult(context.Context, UpdateRemotionRendererAttemptQAResultParams) (RemotionRendererAttempt, error)
}

func TestRemotionRendererGeneratedQueriesExposeDurableAttemptContracts(t *testing.T) {
	var _ remotionRendererQueriesContract = (*Queries)(nil)

	artifactParams := CreateRemotionRendererArtifactParams{
		WorkspaceID:    pgtype.UUID{Valid: true},
		TimelinePlanID: pgtype.UUID{Valid: true},
		Status:         "draft",
		RoutePolicy:    []byte(`{"route":"agent_remotion_code_v1"}`),
		Summary:        "dynamic renderer route selected",
		CreatedByRole:  "composer",
	}
	attemptParams := CreateRemotionRendererAttemptParams{
		WorkspaceID:         pgtype.UUID{Valid: true},
		TimelinePlanID:      pgtype.UUID{Valid: true},
		RendererArtifactID:  pgtype.UUID{Valid: true},
		AttemptNo:           1,
		Status:              "draft",
		SourceSnapshot:      []byte(`{"files":{"GeneratedComposition.tsx":"export default function Video(){return null}"}}`),
		PropsJson:           []byte(`{"output":{"fps":30}}`),
		SourceHash:          "sha256:source",
		PropsHash:           "sha256:props",
		WorkspaceDir:        "/workspace/agent-remotion/artifact-1/1",
		ValidationResult:    []byte(`{"status":"pending"}`),
		CompileResult:       []byte(`{"status":"pending"}`),
		RenderResult:        []byte(`{}`),
		QaResult:            []byte(`{}`),
		RepairFromAttemptID: pgtype.UUID{},
		RepairNotes:         "",
	}
	if artifactParams.Status != "draft" || attemptParams.AttemptNo != 1 {
		t.Fatalf("contract params not initialized: %#v %#v", artifactParams, attemptParams)
	}
}

func TestRemotionRendererGeneratedSQLPreservesAttemptHistoryInvariants(t *testing.T) {
	for _, want := range []string{
		"renderer_artifact_id",
		"attempt_no",
		"source_snapshot",
		"props_json",
		"source_hash",
		"props_hash",
		"validation_result",
		"compile_result",
		"render_result",
		"qa_result",
		"repair_from_attempt_id",
		"repair_notes",
	} {
		if !strings.Contains(createRemotionRendererAttempt, want) {
			t.Fatalf("create attempt SQL missing %q", want)
		}
	}
	for _, want := range []string{
		"current_attempt_id = $1",
		"WHERE remotion_renderer_artifact.id = $3",
		"AND remotion_renderer_attempt.renderer_artifact_id = remotion_renderer_artifact.id",
	} {
		if !strings.Contains(setCurrentRemotionRendererAttempt, want) {
			t.Fatalf("set current attempt SQL missing %q", want)
		}
	}
	for _, want := range []string{
		"source_snapshot = $2",
		"props_json = $3",
		"source_hash = $4",
		"props_hash = $5",
		"workspace_dir = $6",
		"validation_result = $7",
		"compile_result = $8",
	} {
		if !strings.Contains(updateRemotionRendererAttemptSnapshot, want) {
			t.Fatalf("snapshot update SQL missing %q", want)
		}
	}
	if !strings.Contains(updateRemotionRendererAttemptRenderResult, "sandbox_job_id = $3") {
		t.Fatalf("render update SQL does not persist sandbox job: %s", updateRemotionRendererAttemptRenderResult)
	}
	if !strings.Contains(updateRemotionRendererAttemptQAResult, "qa_result = $2") {
		t.Fatalf("QA update SQL does not persist QA result: %s", updateRemotionRendererAttemptQAResult)
	}
}
