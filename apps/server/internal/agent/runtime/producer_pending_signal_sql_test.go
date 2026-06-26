package runtime

import (
	"os"
	"strings"
	"testing"
)

func TestProducerPendingSignalMigrationDocumentsQueueSemantics(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/029_producer_pending_signal.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, want := range []string{
		"CREATE TABLE producer_pending_signal",
		"producer_pending_signal_workspace_dedupe_idx",
		"ON producer_pending_signal(workspace_id, dedupe_key)",
		"status IN ('pending', 'claimed', 'processed', 'ignored', 'failed')",
		"signal_type <> 'craftsman_render_plan_ready' OR render_plan_id IS NOT NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}

func TestProducerPendingSignalQueriesUseDurableQueueSemantics(t *testing.T) {
	raw, err := os.ReadFile("../../../sqlc/queries/producer_pending_signal.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, want := range []string{
		"ON CONFLICT (workspace_id, dedupe_key) DO UPDATE",
		"FOR UPDATE SKIP LOCKED",
		"status = 'claimed'",
		"ListClaimedProducerSignalsByTask",
		"MarkProducerPendingSignalsProcessedByRenderPlan",
		"ReleaseProducerPendingSignalsForTask",
		"status = 'pending'",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("query missing %q", want)
		}
	}
}
