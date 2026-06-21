ALTER TABLE workflow_compensation_instructions
    ALTER COLUMN workflow_id DROP DEFAULT;

ALTER TABLE workflow_compensation_instructions
    DROP CONSTRAINT IF EXISTS ck_workflow_compensation_instructions_workflow_bound;

ALTER TABLE workflow_compensation_instructions
    ADD CONSTRAINT ck_workflow_compensation_instructions_workflow_bound
        CHECK (workflow_id <> '') NOT VALID;

DROP INDEX IF EXISTS uq_workflow_compensation_instructions_active_target;

CREATE UNIQUE INDEX IF NOT EXISTS uq_workflow_compensation_instructions_active_target
    ON workflow_compensation_instructions (tenant_id, workflow_id, payload_ref_hash, target_service, target_operation)
    WHERE status = 'ACTIVE';
