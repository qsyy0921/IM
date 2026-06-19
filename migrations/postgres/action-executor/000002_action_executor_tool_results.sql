CREATE TABLE IF NOT EXISTS action_executor_tool_results (
    tenant_id TEXT NOT NULL,
    result_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    proposal_id TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    prepared_audit_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    skill_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    executed BOOLEAN NOT NULL DEFAULT FALSE,
    result_ref TEXT NOT NULL DEFAULT '',
    output_sha256 TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, result_id),
    CONSTRAINT fk_action_executor_tool_results_execution
        FOREIGN KEY (tenant_id, execution_id)
        REFERENCES action_executor_execution_audits (tenant_id, execution_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_action_executor_tool_result_status CHECK (status IN ('NOT_EXECUTED', 'BLOCKED', 'SUCCEEDED', 'FAILED')),
    CONSTRAINT ck_action_executor_tool_result_output_hash CHECK (executed = FALSE OR output_sha256 <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_action_executor_tool_results_execution
    ON action_executor_tool_results (tenant_id, execution_id);

CREATE INDEX IF NOT EXISTS idx_action_executor_tool_results_status_created
    ON action_executor_tool_results (tenant_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_action_executor_tool_results_approval_created
    ON action_executor_tool_results (tenant_id, approval_id, created_at DESC);
