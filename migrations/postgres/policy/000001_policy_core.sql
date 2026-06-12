CREATE TABLE IF NOT EXISTS policy_message_action_rules (
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    action TEXT NOT NULL,
    allowed BOOLEAN NOT NULL,
    permission_version BIGINT NOT NULL,
    classification TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'manual',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id, action),
    CHECK (action IN ('SEND', 'EDIT', 'REVOKE', 'DELETE')),
    CHECK (permission_version > 0),
    CHECK (classification <> ''),
    CHECK (source <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_message_action_rules_updated
    ON policy_message_action_rules (tenant_id, updated_at DESC);
