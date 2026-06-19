CREATE TABLE IF NOT EXISTS action_executor_execution_audits (
    tenant_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    proposal_id TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    prepared_audit_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    skill_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    tool_action TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    risk_level TEXT NOT NULL DEFAULT '',
    intent TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL DEFAULT '',
    input_sha256 TEXT NOT NULL DEFAULT '',
    allowed BOOLEAN NOT NULL DEFAULT FALSE,
    requires_approval BOOLEAN NOT NULL DEFAULT TRUE,
    permission_version BIGINT NOT NULL DEFAULT 0,
    classification TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    decision_source TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    executed BOOLEAN NOT NULL DEFAULT FALSE,
    output_sha256 TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, execution_id),
    CONSTRAINT ck_action_executor_tool_action CHECK (tool_action IN ('CALL', 'APPROVE', 'EXECUTE')),
    CONSTRAINT ck_action_executor_status CHECK (status IN ('RECORDED', 'BLOCKED', 'FAILED')),
    CONSTRAINT ck_action_executor_no_unapproved_execution CHECK (approval_id <> '' AND proposal_id <> '' AND prepared_audit_id <> ''),
    CONSTRAINT ck_action_executor_no_raw_output CHECK (output_sha256 <> '' OR executed = FALSE)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_action_executor_idempotency
    ON action_executor_execution_audits (tenant_id, user_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_action_executor_tool_created
    ON action_executor_execution_audits (tenant_id, tool_name, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_action_executor_approval_created
    ON action_executor_execution_audits (tenant_id, approval_id, created_at DESC);
