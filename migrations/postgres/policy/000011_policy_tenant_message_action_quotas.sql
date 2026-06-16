CREATE TABLE IF NOT EXISTS policy_tenant_message_action_quotas (
    tenant_id TEXT NOT NULL,
    action TEXT NOT NULL,
    max_decisions INTEGER NOT NULL,
    window_seconds INTEGER NOT NULL,
    permission_version BIGINT NOT NULL,
    classification TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    source TEXT NOT NULL DEFAULT 'manual',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, action),
    CHECK (action IN ('SEND', 'EDIT', 'REVOKE', 'DELETE')),
    CHECK (max_decisions > 0),
    CHECK (window_seconds > 0),
    CHECK (permission_version > 0),
    CHECK (classification <> ''),
    CHECK (source <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_tenant_message_action_quotas_updated
    ON policy_tenant_message_action_quotas (tenant_id, updated_at DESC);
