package production

import "github.com/jackc/pgx/v5/pgtype"

const (
	ProductionEventJobStarted          = "job.started"
	ProductionEventModelStreamDelta    = "model.stream_delta"
	ProductionEventProviderTaskCreated = "provider.task_created"
	ProductionEventProviderProgress    = "provider.progress"
	ProductionEventAssetDownloading    = "asset.downloading"
	ProductionEventAssetUploading      = "asset.uploading"
	ProductionEventJobSucceeded        = "job.succeeded"
	ProductionEventJobFailed           = "job.failed"
	ProductionEventJobCancelled        = "job.cancelled"
)

type ProductionEvent struct {
	JobID        pgtype.UUID
	WorkspaceID  pgtype.UUID
	TargetNodeID pgtype.UUID
	Type         string
	Progress     int32
	Payload      map[string]any
	Output       ProductionOutput
	Err          error
}

type ProductionEventPublisher interface {
	PublishProductionEvent(event ProductionEvent)
}
