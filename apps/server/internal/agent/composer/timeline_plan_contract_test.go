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
