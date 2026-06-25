ALTER TABLE workflow_external_callback_deliveries
    ADD COLUMN IF NOT EXISTS redrive_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_redrive_plan_sha256 text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_redrive_reason_ref text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_redriven_at timestamptz;

ALTER TABLE workflow_external_callback_deliveries
    ADD CONSTRAINT ck_workflow_external_callback_deliveries_redrive_count
    CHECK (redrive_count >= 0) NOT VALID;
