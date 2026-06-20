CREATE TABLE IF NOT EXISTS action_executor_provider_failures (
    tenant_id TEXT NOT NULL,
    provider_failure_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    result_id TEXT NOT NULL,
    proposal_id TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    prepared_audit_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    skill_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    classification TEXT NOT NULL,
    status TEXT NOT NULL,
    retryable BOOLEAN NOT NULL DEFAULT FALSE,
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    failure_ref TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, provider_failure_id),
    CONSTRAINT fk_action_executor_provider_failures_execution
        FOREIGN KEY (tenant_id, execution_id)
        REFERENCES action_executor_execution_audits (tenant_id, execution_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_action_executor_provider_failures_result
        FOREIGN KEY (tenant_id, result_id)
        REFERENCES action_executor_tool_results (tenant_id, result_id)
        ON DELETE CASCADE,
    CONSTRAINT ck_action_executor_provider_failure_status
        CHECK (status IN ('RETRY_PENDING', 'DLQ')),
    CONSTRAINT ck_action_executor_provider_failure_retry_state
        CHECK (
            (
                status = 'RETRY_PENDING'
                AND retryable = TRUE
                AND next_retry_at IS NOT NULL
                AND dead_lettered_at IS NULL
            )
            OR
            (
                status = 'DLQ'
                AND retryable = FALSE
                AND next_retry_at IS NULL
                AND dead_lettered_at IS NOT NULL
            )
        ),
    CONSTRAINT ck_action_executor_provider_failure_retry_count
        CHECK (retry_count >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_action_executor_provider_failures_execution
    ON action_executor_provider_failures (tenant_id, execution_id);

CREATE INDEX IF NOT EXISTS idx_action_executor_provider_failures_status_created
    ON action_executor_provider_failures (tenant_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_action_executor_provider_failures_tool_created
    ON action_executor_provider_failures (tenant_id, tool_name, created_at DESC);
