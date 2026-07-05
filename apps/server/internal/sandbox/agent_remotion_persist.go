package sandbox

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type AgentRemotionAttemptRepository interface {
	UpdateRemotionRendererAttemptSnapshot(ctx context.Context, arg db.UpdateRemotionRendererAttemptSnapshotParams) (db.RemotionRendererAttempt, error)
}

func PersistAgentRemotionValidation(ctx context.Context, repo AgentRemotionAttemptRepository, attemptID pgtype.UUID, snapshot AgentRemotionSnapshot, result AgentRemotionValidationResult) (db.RemotionRendererAttempt, error) {
	if repo == nil {
		return db.RemotionRendererAttempt{}, errors.New("remotion renderer attempt repository is required")
	}
	sourceSnapshot, err := json.Marshal(snapshot.SourceSnapshot)
	if err != nil {
		return db.RemotionRendererAttempt{}, err
	}
	validationResult, err := json.Marshal(result)
	if err != nil {
		return db.RemotionRendererAttempt{}, err
	}
	compileResult, err := json.Marshal(map[string]string{
		"status": "not_run",
		"phase":  "m14_2_static_validation",
	})
	if err != nil {
		return db.RemotionRendererAttempt{}, err
	}
	status := "validated"
	if !result.Passed {
		status = "validation_failed"
	}
	return repo.UpdateRemotionRendererAttemptSnapshot(ctx, db.UpdateRemotionRendererAttemptSnapshotParams{
		Status:           status,
		SourceSnapshot:   sourceSnapshot,
		PropsJson:        append([]byte(nil), snapshot.PropsJSON...),
		SourceHash:       snapshot.SourceHash,
		PropsHash:        snapshot.PropsHash,
		WorkspaceDir:     snapshot.WorkspaceDir,
		ValidationResult: validationResult,
		CompileResult:    compileResult,
		ID:               attemptID,
	})
}
