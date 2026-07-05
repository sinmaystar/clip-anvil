package composer

import (
	"os"
	"strings"
	"testing"
)

func TestTimelinePlanMigrationDocumentsComposerDurableOutput(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/031_composer_timeline_plan.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, want := range []string{
		"CREATE TABLE timeline_plan",
		"workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE",
		"source_storyboard_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL",
		"status TEXT NOT NULL CHECK (status IN ('draft', 'rendering', 'completed', 'blocked', 'failed'))",
		"plan_json JSONB NOT NULL",
		"idx_timeline_plan_workspace_created",
		"idx_timeline_plan_workspace_status",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}

func TestTimelinePlanQueriesExposeComposerPlanLifecycle(t *testing.T) {
	raw, err := os.ReadFile("../../../sqlc/queries/timeline_plan.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, want := range []string{
		"-- name: CreateTimelinePlan :one",
		"-- name: GetTimelinePlan :one",
		"-- name: ListTimelinePlansByWorkspace :many",
		"-- name: GetLatestCompletedTimelinePlanByWorkspace :one",
		"-- name: UpdateTimelinePlanStatus :one",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("query missing %q", want)
		}
	}
}

func TestRemotionRendererMigrationDocumentsArtifactAttemptHistory(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/042_m14_1_remotion_renderer_artifact_attempt.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, want := range []string{
		"CREATE TABLE remotion_renderer_artifact",
		"workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE",
		"timeline_plan_id UUID NOT NULL REFERENCES timeline_plan(id) ON DELETE CASCADE",
		"current_attempt_id UUID",
		"route_policy JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CONSTRAINT remotion_renderer_artifact_timeline_unique UNIQUE (timeline_plan_id)",
		"CREATE TABLE remotion_renderer_attempt",
		"renderer_artifact_id UUID NOT NULL REFERENCES remotion_renderer_artifact(id) ON DELETE CASCADE",
		"attempt_no INT NOT NULL",
		"source_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb",
		"props_json JSONB NOT NULL DEFAULT '{}'::jsonb",
		"source_hash TEXT NOT NULL DEFAULT ''",
		"props_hash TEXT NOT NULL DEFAULT ''",
		"workspace_dir TEXT NOT NULL DEFAULT ''",
		"validation_result JSONB NOT NULL DEFAULT '{}'::jsonb",
		"compile_result JSONB NOT NULL DEFAULT '{}'::jsonb",
		"render_result JSONB NOT NULL DEFAULT '{}'::jsonb",
		"qa_result JSONB NOT NULL DEFAULT '{}'::jsonb",
		"sandbox_job_id UUID REFERENCES sandbox_job(id) ON DELETE SET NULL",
		"repair_from_attempt_id UUID REFERENCES remotion_renderer_attempt(id) ON DELETE SET NULL",
		"CONSTRAINT remotion_renderer_attempt_artifact_no_unique UNIQUE (renderer_artifact_id, attempt_no)",
		"workspace_dir = '' OR workspace_dir LIKE '/workspace/agent-remotion/%'",
		"idx_remotion_renderer_artifact_workspace_created",
		"idx_remotion_renderer_attempt_artifact",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}

func TestRemotionRendererQueriesExposeArtifactAttemptLifecycle(t *testing.T) {
	raw, err := os.ReadFile("../../../sqlc/queries/remotion_renderer.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, want := range []string{
		"-- name: CreateRemotionRendererArtifact :one",
		"-- name: GetRemotionRendererArtifact :one",
		"-- name: GetRemotionRendererArtifactByTimelinePlan :one",
		"-- name: ListRemotionRendererArtifactsByWorkspace :many",
		"-- name: UpdateRemotionRendererArtifactStatus :one",
		"-- name: SetCurrentRemotionRendererAttempt :one",
		"-- name: CreateRemotionRendererAttempt :one",
		"-- name: GetRemotionRendererAttempt :one",
		"-- name: GetCurrentRemotionRendererAttempt :one",
		"-- name: ListRemotionRendererAttemptsByArtifact :many",
		"-- name: GetLatestRemotionRendererAttemptByArtifact :one",
		"-- name: UpdateRemotionRendererAttemptSnapshot :one",
		"-- name: UpdateRemotionRendererAttemptRenderResult :one",
		"-- name: UpdateRemotionRendererAttemptQAResult :one",
		"AND remotion_renderer_attempt.renderer_artifact_id = remotion_renderer_artifact.id",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("query missing %q", want)
		}
	}
}
