CREATE TABLE IF NOT EXISTS admin_operations (
    tenant_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    operation_type TEXT NOT NULL,
    target_ref_hash TEXT NOT NULL,
    risk_level TEXT NOT NULL,
    payload_schema_version TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    payload_hash TEXT NOT NULL,
    reason_ref TEXT NOT NULL DEFAULT '',
    evidence_refs_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    approved_by TEXT NOT NULL DEFAULT '',
    approved_at TIMESTAMPTZ,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, operation_id),
    CONSTRAINT ck_admin_operations_status CHECK (status IN ('SUBMITTED', 'APPROVED', 'REJECTED', 'EXECUTING', 'SUCCEEDED', 'FAILED', 'CANCELED', 'COMPENSATION_REQUESTED')),
    CONSTRAINT ck_admin_operations_risk CHECK (risk_level IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CONSTRAINT ck_admin_operations_payload_object CHECK (jsonb_typeof(payload_json) = 'object'),
    CONSTRAINT ck_admin_operations_evidence_array CHECK (jsonb_typeof(evidence_refs_json) = 'array')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_admin_operations_idempotency
    ON admin_operations (tenant_id, requested_by, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_admin_operations_tenant_status
    ON admin_operations (tenant_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS admin_approvals (
    tenant_id TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    approver_ref TEXT NOT NULL,
    decision TEXT NOT NULL,
    approval_policy_ref TEXT NOT NULL DEFAULT '',
    reason_ref TEXT NOT NULL DEFAULT '',
    evidence_refs_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, approval_id),
    CONSTRAINT fk_admin_approvals_operation FOREIGN KEY (tenant_id, operation_id)
        REFERENCES admin_operations (tenant_id, operation_id) ON DELETE CASCADE,
    CONSTRAINT ck_admin_approvals_decision CHECK (decision IN ('APPROVE', 'REJECT')),
    CONSTRAINT ck_admin_approvals_evidence_array CHECK (jsonb_typeof(evidence_refs_json) = 'array')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_admin_approvals_idempotency
    ON admin_approvals (tenant_id, operation_id, approver_ref, idempotency_key);

CREATE TABLE IF NOT EXISTS admin_operation_results (
    tenant_id TEXT NOT NULL,
    result_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    downstream_service TEXT NOT NULL,
    downstream_request_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    failure_class TEXT NOT NULL DEFAULT '',
    public_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, result_id),
    CONSTRAINT fk_admin_results_operation FOREIGN KEY (tenant_id, operation_id)
        REFERENCES admin_operations (tenant_id, operation_id) ON DELETE CASCADE,
    CONSTRAINT ck_admin_results_status CHECK (status IN ('PENDING', 'SUCCEEDED', 'FAILED'))
);

CREATE TABLE IF NOT EXISTS admin_outbox (
    event_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_version INT NOT NULL,
    partition_key TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_admin_outbox_status CHECK (status IN ('PENDING', 'PUBLISHED', 'FAILED', 'DLQ')),
    CONSTRAINT ck_admin_outbox_payload_object CHECK (jsonb_typeof(payload_json) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_admin_outbox_ready
    ON admin_outbox (status, available_at, next_retry_at, created_at);

CREATE INDEX IF NOT EXISTS idx_admin_outbox_operation
    ON admin_outbox (tenant_id, operation_id, event_type);
