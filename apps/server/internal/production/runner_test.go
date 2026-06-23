package production

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestQueuedJobFailureEventIncludesCanvasRoutingFields(t *testing.T) {
	runErr := errors.New("persist output asset failed")
	job := db.GenerationJob{
		ID:           pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true},
		WorkspaceID:  pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true},
		TargetNodeID: pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true},
	}

	event := queuedJobFailureEvent(job, runErr)

	if event.Type != ProductionEventJobFailed {
		t.Fatalf("event type = %q, want %q", event.Type, ProductionEventJobFailed)
	}
	if event.JobID != job.ID || event.WorkspaceID != job.WorkspaceID || event.TargetNodeID != job.TargetNodeID {
		t.Fatalf("event routing fields = %#v, want job/workspace/node ids", event)
	}
	if event.Progress != 100 {
		t.Fatalf("progress = %d, want 100", event.Progress)
	}
	if !errors.Is(event.Err, runErr) {
		t.Fatalf("event error = %v, want %v", event.Err, runErr)
	}
	if event.Payload["error"] != runErr.Error() {
		t.Fatalf("payload error = %#v, want %q", event.Payload["error"], runErr.Error())
	}
}
