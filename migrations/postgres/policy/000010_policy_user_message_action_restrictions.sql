CREATE TABLE IF NOT EXISTS policy_user_message_action_restrictions (
    tenant_id          TEXT        NOT NULL,
    user_id            TEXT        NOT NULL,
    action             TEXT        NOT NULL,
    permission_version BIGINT      NOT NULL,
    classification     TEXT        NOT NULL,
    reason             TEXT        NOT NULL DEFAULT '',
    source             TEXT        NOT NULL DEFAULT 'manual',
    expires_at         TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, action),
    CHECK (action IN ('SEND', 'EDIT', 'REVOKE', 'DELETE')),
    CHECK (permission_version > 0),
    CHECK (classification <> ''),
    CHECK (source <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_user_message_action_restrictions_active
    ON policy_user_message_action_restrictions (tenant_id, user_id, action, expires_at);

CREATE INDEX IF NOT EXISTS idx_policy_user_message_action_restrictions_updated
    ON policy_user_message_action_restrictions (tenant_id, updated_at DESC);
