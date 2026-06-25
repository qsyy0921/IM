CREATE TABLE IF NOT EXISTS workflow_external_callback_deliveries (
    tenant_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    delivery_id TEXT NOT NULL,
    delivery_plan_sha256 TEXT NOT NULL,
    source_decision_manifest_sha256 TEXT NOT NULL DEFAULT '',
    step_id TEXT NOT NULL,
    workflow_type TEXT NOT NULL,
    target_service TEXT NOT NULL,
    target_operation TEXT NOT NULL,
    target_ref_hash TEXT NOT NULL,
    payload_schema_version TEXT NOT NULL,
    payload_ref_hash TEXT NOT NULL,
    approval_policy_ref TEXT NOT NULL,
    decision_policy_ref TEXT NOT NULL,
    callback_provider_ref TEXT NOT NULL,
    callback_endpoint_ref TEXT NOT NULL,
    delivery_queue_ref TEXT NOT NULL,
    retry_policy_ref TEXT NOT NULL,
    backoff_policy_ref TEXT NOT NULL,
    callback_timeout_policy_ref TEXT NOT NULL,
    callback_payload_schema_version TEXT NOT NULL,
    callback_payload_ref_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    leased_until TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    last_failure_class TEXT NOT NULL DEFAULT '',
    last_delivery_result_ref TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, delivery_id),
    CONSTRAINT fk_workflow_external_callback_deliveries_workflow FOREIGN KEY (tenant_id, workflow_id)
        REFERENCES workflow_requests (tenant_id, workflow_id) ON DELETE CASCADE,
    CONSTRAINT ck_workflow_external_callback_deliveries_status
        CHECK (status IN ('PENDING', 'IN_FLIGHT', 'DELIVERED', 'RETRY_PENDING', 'DLQ')) NOT VALID,
    CONSTRAINT ck_workflow_external_callback_deliveries_attempts
        CHECK (attempt_count >= 0 AND max_attempts BETWEEN 1 AND 10) NOT VALID
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_workflow_external_callback_deliveries_plan
    ON workflow_external_callback_deliveries (tenant_id, workflow_id, delivery_plan_sha256);

CREATE INDEX IF NOT EXISTS idx_workflow_external_callback_deliveries_ready
    ON workflow_external_callback_deliveries (status, available_at, created_at)
    WHERE status IN ('PENDING', 'RETRY_PENDING', 'IN_FLIGHT');
