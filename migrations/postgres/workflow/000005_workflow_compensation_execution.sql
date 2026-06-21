ALTER TABLE workflow_requests
    DROP CONSTRAINT IF EXISTS ck_workflow_requests_status;

ALTER TABLE workflow_requests
    ADD CONSTRAINT ck_workflow_requests_status
        CHECK (status IN (
            'WAITING_DECISION',
            'APPROVED',
            'REJECTED',
            'CANCELED',
            'COMPENSATION_PENDING',
            'COMPENSATED'
        ));

ALTER TABLE workflow_compensations
    ADD COLUMN IF NOT EXISTS payload_schema_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payload_ref_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS compensation_policy_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reason_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS downstream_service TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS downstream_request_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE workflow_compensations
    DROP CONSTRAINT IF EXISTS ck_workflow_compensations_status;

ALTER TABLE workflow_compensations
    ADD CONSTRAINT ck_workflow_compensations_status
        CHECK (status IN ('PENDING', 'REQUESTED', 'EXECUTING', 'SUCCEEDED', 'FAILED', 'CANCELED'));

CREATE INDEX IF NOT EXISTS idx_workflow_compensations_ready
    ON workflow_compensations (tenant_id, status, updated_at, created_at)
    WHERE status IN ('REQUESTED', 'EXECUTING');
