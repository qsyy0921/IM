ALTER TABLE action_executor_execution_audits
    ADD COLUMN IF NOT EXISTS redrive_provider_failure_id TEXT,
    ADD COLUMN IF NOT EXISTS redrive_reason_sha256 TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'fk_action_executor_redrive_provider_failure'
    ) THEN
        ALTER TABLE action_executor_execution_audits
            ADD CONSTRAINT fk_action_executor_redrive_provider_failure
            FOREIGN KEY (tenant_id, redrive_provider_failure_id)
            REFERENCES action_executor_provider_failures (tenant_id, provider_failure_id)
            ON DELETE RESTRICT;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ck_action_executor_redrive_metadata'
    ) THEN
        ALTER TABLE action_executor_execution_audits
            ADD CONSTRAINT ck_action_executor_redrive_metadata
            CHECK (
                (
                    redrive_provider_failure_id IS NULL
                    AND redrive_reason_sha256 = ''
                )
                OR
                (
                    redrive_provider_failure_id IS NOT NULL
                    AND redrive_reason_sha256 ~ '^[0-9a-f]{64}$'
                )
            )
            NOT VALID;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_action_executor_execution_redrive_source
    ON action_executor_execution_audits (tenant_id, redrive_provider_failure_id, created_at DESC)
    WHERE redrive_provider_failure_id IS NOT NULL;
