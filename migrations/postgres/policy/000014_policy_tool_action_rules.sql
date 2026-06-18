CREATE TABLE IF NOT EXISTS policy_tool_action_rules (
    tenant_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '*',
    risk_level TEXT NOT NULL DEFAULT 'ANY',
    allowed BOOLEAN NOT NULL DEFAULT false,
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    permission_version BIGINT NOT NULL DEFAULT 1,
    classification TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    priority INTEGER NOT NULL DEFAULT 100,
    enabled BOOLEAN NOT NULL DEFAULT true,
    source TEXT NOT NULL DEFAULT 'manual',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, tool_name, action, resource_type, risk_level),
    CHECK (tenant_id <> ''),
    CHECK (tool_name <> ''),
    CHECK (action IN ('CALL', 'APPROVE', 'EXECUTE')),
    CHECK (resource_type <> ''),
    CHECK (risk_level IN ('ANY', 'LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CHECK (permission_version > 0),
    CHECK (classification <> ''),
    CHECK (priority >= 0),
    CHECK (source <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_tool_action_rules_tenant_lookup
    ON policy_tool_action_rules (tenant_id, action, enabled, priority, updated_at);

CREATE TABLE IF NOT EXISTS policy_tool_decision_audit (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    actor_user_key TEXT NOT NULL,
    device_key TEXT NOT NULL DEFAULT '',
    tool_name TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id_present BOOLEAN NOT NULL DEFAULT false,
    risk_level TEXT NOT NULL,
    allowed BOOLEAN NOT NULL,
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    permission_version BIGINT NOT NULL,
    classification TEXT NOT NULL,
    reason_code TEXT NOT NULL DEFAULT '',
    decision_source TEXT NOT NULL,
    trace_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (tenant_id <> ''),
    CHECK (actor_user_key <> ''),
    CHECK (tool_name <> ''),
    CHECK (action IN ('CALL', 'APPROVE', 'EXECUTE')),
    CHECK (resource_type <> ''),
    CHECK (risk_level IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CHECK (permission_version > 0),
    CHECK (classification <> ''),
    CHECK (decision_source <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_tool_decision_audit_tenant_created
    ON policy_tool_decision_audit (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_policy_tool_decision_audit_tool_created
    ON policy_tool_decision_audit (tenant_id, tool_name, action, created_at DESC);
