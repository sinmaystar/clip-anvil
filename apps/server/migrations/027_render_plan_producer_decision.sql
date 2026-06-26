-- +goose Up
ALTER TABLE render_plan DROP CONSTRAINT render_plan_status_check;
ALTER TABLE render_plan
    ADD CONSTRAINT render_plan_status_check CHECK (status IN (
        'draft',
        'blocked',
        'compiled',
        'waiting_for_approval',
        'submitted',
        'running',
        'succeeded',
        'failed',
        'rejected',
        'archived'
    ));

-- +goose Down
UPDATE render_plan
SET status = 'blocked'
WHERE status = 'rejected';

ALTER TABLE render_plan DROP CONSTRAINT render_plan_status_check;
ALTER TABLE render_plan
    ADD CONSTRAINT render_plan_status_check CHECK (status IN (
        'draft',
        'blocked',
        'compiled',
        'waiting_for_approval',
        'submitted',
        'running',
        'succeeded',
        'failed',
        'archived'
    ));
