CREATE TABLE IF NOT EXISTS workflow_compensation_instructions (
    tenant_id TEXT NOT NULL,
    instruction_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL DEFAULT '',
    payload_ref_hash TEXT NOT NULL,
    target_service TEXT NOT NULL,
    target_operation TEXT NOT NULL,
    instruction_type TEXT NOT NULL,
    environment TEXT NOT NULL DEFAULT '',
    config_kind TEXT NOT NULL DEFAULT '',
    bundle_key TEXT NOT NULL DEFAULT '',
    target_version TEXT NOT NULL DEFAULT '',
    operator_ref TEXT NOT NULL,
    reason_ref TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, instruction_id)
);

ALTER TABLE workflow_compensation_instructions
    DROP CONSTRAINT IF EXISTS ck_workflow_compensation_instructions_status;

ALTER TABLE workflow_compensation_instructions
    ADD CONSTRAINT ck_workflow_compensation_instructions_status
        CHECK (status IN ('ACTIVE', 'DISABLED')) NOT VALID;

ALTER TABLE workflow_compensation_instructions
    DROP CONSTRAINT IF EXISTS ck_workflow_compensation_instructions_type;

ALTER TABLE workflow_compensation_instructions
    ADD CONSTRAINT ck_workflow_compensation_instructions_type
        CHECK (instruction_type IN ('CONTROL_PLANE_ROLLBACK')) NOT VALID;

CREATE UNIQUE INDEX IF NOT EXISTS uq_workflow_compensation_instructions_active_target
    ON workflow_compensation_instructions (tenant_id, payload_ref_hash, target_service, target_operation)
    WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_workflow_compensation_instructions_workflow
    ON workflow_compensation_instructions (tenant_id, workflow_id)
    WHERE workflow_id <> '';
