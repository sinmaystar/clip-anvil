package audio

import (
	"os"
	"strings"
	"testing"
)

func TestAudioPlanPersistenceContract(t *testing.T) {
	migration, err := os.ReadFile("../../../migrations/032_m7_1_audio_plan.sql")
	if err != nil {
		t.Fatal(err)
	}
	query, err := os.ReadFile("../../../sqlc/queries/audio_plan.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrationText := string(migration)
	queryText := string(query)
	for _, want := range []string{
		"CREATE TABLE audio_plan",
		"workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE",
		"voiceover_script TEXT NOT NULL DEFAULT ''",
		"voice_profile JSONB NOT NULL DEFAULT '{}'",
		"bgm_plan JSONB NOT NULL DEFAULT '{}'",
		"cue_plan JSONB NOT NULL DEFAULT '[]'",
		"generation_params JSONB NOT NULL DEFAULT '{}'",
		"idx_audio_plan_workspace_active",
		"status IN ('draft', 'waiting_for_user', 'approved', 'generating', 'voiceover_ready', 'composing', 'completed', 'blocked', 'failed', 'archived')",
	} {
		if !strings.Contains(migrationText, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	for _, want := range []string{
		"-- name: CreateAudioPlan :one",
		"-- name: GetAudioPlan :one",
		"-- name: GetActiveAudioPlanByWorkspace :one",
		"-- name: ListAudioPlansByWorkspace :many",
		"-- name: UpdateAudioPlan :one",
		"-- name: ArchiveActiveAudioPlansByWorkspace :exec",
		"-- name: UpdateAudioPlanStatus :one",
	} {
		if !strings.Contains(queryText, want) {
			t.Fatalf("query missing %q", want)
		}
	}
}
