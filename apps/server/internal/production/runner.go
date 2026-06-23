package production

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ProductionRunner struct {
	service     *Service
	runtime     EinoProductionRuntime
	publisher   ProductionEventPublisher
	jobs        chan db.GenerationJob
	concurrency int
}

func NewProductionRunner(service *Service, runtime EinoProductionRuntime, concurrency int, publisher ProductionEventPublisher) *ProductionRunner {
	if concurrency < 1 {
		concurrency = 1
	}
	return &ProductionRunner{
		service:     service,
		runtime:     runtime,
		publisher:   publisher,
		jobs:        make(chan db.GenerationJob, concurrency*16),
		concurrency: concurrency,
	}
}

func (r *ProductionRunner) Start(ctx context.Context) {
	for i := 0; i < r.concurrency; i++ {
		go r.worker(ctx)
	}
}

func (r *ProductionRunner) Enqueue(job db.GenerationJob) {
	if r == nil {
		return
	}
	r.jobs <- job
}

func (r *ProductionRunner) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-r.jobs:
			if err := r.runJob(ctx, job); err != nil {
				slog.Warn("production job failed", "job_id", uuidToString(job.ID), "error", err)
			}
		}
	}
}

func (r *ProductionRunner) runJob(ctx context.Context, job db.GenerationJob) error {
	if r == nil || r.service == nil || r.runtime == nil {
		return fmt.Errorf("%w: production runner is not configured", ErrProviderConfig)
	}
	var intent GenerationIntent
	if err := json.Unmarshal(job.Intent, &intent); err != nil {
		return r.failQueuedJob(ctx, job, fmt.Errorf("%w: invalid generation intent", ErrProviderExecution))
	}
	started, err := r.service.markQueuedJobRunning(ctx, job, 1, map[string]any{"event": ProductionEventJobStarted})
	if err != nil {
		return err
	}
	r.publish(ProductionEvent{
		JobID:        started.ID,
		WorkspaceID:  started.WorkspaceID,
		TargetNodeID: started.TargetNodeID,
		Type:         ProductionEventJobStarted,
		Progress:     started.Progress,
	})

	events, err := r.runtime.Start(ctx, ProductionJob{ID: job.ID, WorkspaceID: job.WorkspaceID, TargetNodeID: job.TargetNodeID}, intent)
	if err != nil {
		return r.failQueuedJob(ctx, job, err)
	}
	for event := range events {
		event.JobID = job.ID
		event.WorkspaceID = job.WorkspaceID
		event.TargetNodeID = job.TargetNodeID
		switch event.Type {
		case ProductionEventJobSucceeded:
			if _, err := r.service.persistQueuedJobSuccess(ctx, job.ID, outputToProviderResult(event.Output)); err != nil {
				return r.failQueuedJob(ctx, job, err)
			}
			r.publish(event)
			return nil
		case ProductionEventJobFailed:
			if event.Err == nil {
				event.Err = fmt.Errorf("%w: provider failed", ErrProviderExecution)
			}
			return r.failQueuedJob(ctx, job, event.Err)
		case ProductionEventJobCancelled:
			event.Err = fmt.Errorf("%w: production job cancelled", ErrProviderExecution)
			if err := r.service.markQueuedJobFailed(ctx, job, event.Err); err != nil {
				return err
			}
			r.publish(event)
			return nil
		default:
			r.publish(event)
			if event.Progress > 0 {
				if _, err := r.service.markQueuedJobProgress(ctx, job, event.Progress, eventPayload(event)); err != nil {
					return err
				}
			}
		}
	}
	return r.failQueuedJob(ctx, job, fmt.Errorf("%w: runtime completed without output", ErrProviderExecution))
}

func (r *ProductionRunner) publish(event ProductionEvent) {
	if r.publisher != nil {
		r.publisher.PublishProductionEvent(event)
	}
}

func (r *ProductionRunner) failQueuedJob(ctx context.Context, job db.GenerationJob, runErr error) error {
	if runErr == nil {
		runErr = fmt.Errorf("%w: provider failed", ErrProviderExecution)
	}
	err := r.service.markQueuedJobFailed(ctx, job, runErr)
	r.publish(queuedJobFailureEvent(job, runErr))
	return err
}

func queuedJobFailureEvent(job db.GenerationJob, runErr error) ProductionEvent {
	if runErr == nil {
		runErr = fmt.Errorf("%w: provider failed", ErrProviderExecution)
	}
	return ProductionEvent{
		JobID:        job.ID,
		WorkspaceID:  job.WorkspaceID,
		TargetNodeID: job.TargetNodeID,
		Type:         ProductionEventJobFailed,
		Progress:     100,
		Err:          runErr,
		Payload: map[string]any{
			"error": runErr.Error(),
		},
	}
}

func eventPayload(event ProductionEvent) map[string]any {
	payload := map[string]any{
		"event": event.Type,
	}
	for key, value := range event.Payload {
		payload[key] = value
	}
	return payload
}
