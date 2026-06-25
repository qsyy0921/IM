ALTER TABLE workflow_requests
    DROP CONSTRAINT IF EXISTS ck_workflow_requests_status;

ALTER TABLE workflow_requests
    ADD CONSTRAINT ck_workflow_requests_status
        CHECK (status IN (
            'WAITING_DECISION',
            'APPROVED',
            'REJECTED',
            'CANCELED',
            'TIMED_OUT',
            'COMPENSATION_PENDING',
            'COMPENSATED'
        ));

CREATE INDEX IF NOT EXISTS idx_workflow_timers_due
    ON workflow_timers (timer_type, status, due_at, created_at)
    WHERE status = 'PENDING';
