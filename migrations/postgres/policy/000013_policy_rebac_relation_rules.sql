CREATE TABLE IF NOT EXISTS policy_rebac_message_action_rules (
    tenant_id            TEXT        NOT NULL,
    action               TEXT        NOT NULL,
    relation_type        TEXT        NOT NULL,
    conversation_scope   TEXT        NOT NULL DEFAULT 'ANY',
    permission_version   BIGINT      NOT NULL,
    classification       TEXT        NOT NULL,
    reason               TEXT        NOT NULL DEFAULT '',
    priority             INTEGER     NOT NULL DEFAULT 100,
    enabled              BOOLEAN     NOT NULL DEFAULT true,
    source               TEXT        NOT NULL DEFAULT 'manual',
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, action, relation_type, conversation_scope),
    CHECK (action IN ('SEND', 'EDIT', 'REVOKE', 'DELETE')),
    CHECK (relation_type IN ('DIRECT_CONTACT_ACTIVE', 'CONVERSATION_MEMBER_ACTIVE')),
    CHECK (conversation_scope IN ('ANY', 'DIRECT', 'GROUP')),
    CHECK (permission_version > 0),
    CHECK (classification <> ''),
    CHECK (priority >= 0),
    CHECK (source <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_rebac_message_action_rules_enabled
    ON policy_rebac_message_action_rules (tenant_id, action, conversation_scope, enabled, priority);

CREATE INDEX IF NOT EXISTS idx_policy_rebac_message_action_rules_updated
    ON policy_rebac_message_action_rules (tenant_id, updated_at DESC);
