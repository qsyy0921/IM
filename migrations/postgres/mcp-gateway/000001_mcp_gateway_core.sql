CREATE TABLE IF NOT EXISTS mcp_gateway_tool_call_audits (
    tenant_id TEXT NOT NULL,
    audit_id TEXT NOT NULL,
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
    requires_approval BOOLEAN NOT NULL DEFAULT FALSE,
    permission_version BIGINT NOT NULL DEFAULT 0,
    classification TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    decision_source TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, audit_id),
    CONSTRAINT ck_mcp_gateway_tool_call_audits_status
        CHECK (status IN ('ALLOWED', 'BLOCKED', 'FAILED'))
);

CREATE INDEX IF NOT EXISTS idx_mcp_gateway_tool_call_audits_tenant_tool_created
    ON mcp_gateway_tool_call_audits (tenant_id, tool_name, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_mcp_gateway_tool_call_audits_tenant_user_created
    ON mcp_gateway_tool_call_audits (tenant_id, user_id, created_at DESC);
