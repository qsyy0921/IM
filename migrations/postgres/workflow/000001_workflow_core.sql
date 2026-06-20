CREATE TABLE IF NOT EXISTS workflow_requests (
    tenant_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    workflow_type TEXT NOT NULL,
    risk_level TEXT NOT NULL,
    requester_ref TEXT NOT NULL,
    requester_service TEXT NOT NULL,
    target_service TEXT NOT NULL,
    target_operation TEXT NOT NULL,
    target_ref_hash TEXT NOT NULL,
    payload_schema_version TEXT NOT NULL,
    payload_ref_hash TEXT NOT NULL,
    approval_policy_ref TEXT NOT NULL DEFAULT '',
    timeout_policy_ref TEXT NOT NULL DEFAULT '',
    compensation_policy_ref TEXT NOT NULL DEFAULT '',
    reason_ref TEXT NOT NULL DEFAULT '',
    evidence_refs_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL,
    current_step_id TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, workflow_id),
    CONSTRAINT ck_workflow_requests_type CHECK (workflow_type IN ('ACTION_APPROVAL', 'REPAIR_APPROVAL')),
    CONSTRAINT ck_workflow_requests_risk CHECK (risk_level IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CONSTRAINT ck_workflow_requests_status CHECK (status IN ('WAITING_DECISION', 'APPROVED', 'REJECTED', 'CANCELED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_workflow_requests_idempotency
    ON workflow_requests (tenant_id, requester_service, idempotency_key);

CREATE TABLE IF NOT EXISTS workflow_steps (
    tenant_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    step_id TEXT NOT NULL,
    step_index INTEGER NOT NULL,
    step_type TEXT NOT NULL,
    target_service TEXT NOT NULL,
    target_operation TEXT NOT NULL,
    status TEXT NOT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    failure_class TEXT NOT NULL DEFAULT '',
    public_error TEXT NOT NULL DEFAULT '',
    due_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, workflow_id, step_id),
    CONSTRAINT fk_workflow_steps_request FOREIGN KEY (tenant_id, workflow_id)
        REFERENCES workflow_requests (tenant_id, workflow_id) ON DELETE CASCADE,
    CONSTRAINT ck_workflow_steps_status CHECK (status IN ('READY', 'SUCCEEDED', 'FAILED', 'SKIPPED'))
);

CREATE TABLE IF NOT EXISTS workflow_decisions (
    tenant_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    decision_id TEXT NOT NULL,
    step_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    command_hash TEXT NOT NULL,
    decider_ref TEXT NOT NULL,
    decision_type TEXT NOT NULL,
    decision_policy_ref TEXT NOT NULL DEFAULT '',
    reason_ref TEXT NOT NULL DEFAULT '',
    evidence_refs_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, workflow_id, decision_id),
    CONSTRAINT fk_workflow_decisions_request FOREIGN KEY (tenant_id, workflow_id)
        REFERENCES workflow_requests (tenant_id, workflow_id) ON DELETE CASCADE,
    CONSTRAINT ck_workflow_decisions_type CHECK (decision_type IN ('APPROVE', 'REJECT', 'REQUEST_CHANGES', 'CANCEL'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_workflow_decisions_idempotency
    ON workflow_decisions (tenant_id, workflow_id, decider_ref, idempotency_key);

CREATE TABLE IF NOT EXISTS workflow_timers (
    tenant_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    timer_id TEXT NOT NULL,
    step_id TEXT NOT NULL DEFAULT '',
    timer_type TEXT NOT NULL,
    due_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    fired_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, workflow_id, timer_id),
    CONSTRAINT ck_workflow_timers_status CHECK (status IN ('PENDING', 'FIRED', 'CANCELED'))
);

CREATE TABLE IF NOT EXISTS workflow_compensations (
    tenant_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    compensation_id TEXT NOT NULL,
    source_step_id TEXT NOT NULL DEFAULT '',
    target_service TEXT NOT NULL,
    target_operation TEXT NOT NULL,
    target_ref_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    failure_class TEXT NOT NULL DEFAULT '',
    public_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, workflow_id, compensation_id),
    CONSTRAINT ck_workflow_compensations_status CHECK (status IN ('PENDING', 'REQUESTED', 'SUCCEEDED', 'FAILED', 'CANCELED'))
);

CREATE TABLE IF NOT EXISTS workflow_outbox (
    event_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_version INTEGER NOT NULL DEFAULT 1,
    partition_key TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retry_count INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_workflow_outbox_status CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);

CREATE INDEX IF NOT EXISTS idx_workflow_requests_status
    ON workflow_requests (tenant_id, status, created_at);

CREATE INDEX IF NOT EXISTS idx_workflow_outbox_ready
    ON workflow_outbox (status, available_at, created_at)
    WHERE status = 'PENDING';
