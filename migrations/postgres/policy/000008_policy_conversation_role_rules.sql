CREATE TABLE IF NOT EXISTS policy_conversation_members_projection (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    role                TEXT        NOT NULL,
    status              TEXT        NOT NULL,
    member_version      BIGINT      NOT NULL,
    permission_version  BIGINT      NOT NULL,
    updated_by_event_id TEXT        NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, user_id),
    CHECK (role IN ('OWNER', 'ADMIN', 'MEMBER')),
    CHECK (status IN ('ACTIVE', 'LEFT', 'BANNED')),
    CHECK (member_version > 0),
    CHECK (permission_version > 0),
    CHECK (updated_by_event_id <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_conversation_members_active_role
    ON policy_conversation_members_projection (tenant_id, conversation_id, status, role);

CREATE TABLE IF NOT EXISTS policy_conversation_role_action_rules (
    tenant_id          TEXT        NOT NULL,
    action             TEXT        NOT NULL,
    min_role           TEXT        NOT NULL,
    classification     TEXT        NOT NULL,
    reason             TEXT        NOT NULL DEFAULT '',
    source             TEXT        NOT NULL DEFAULT 'manual',
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, action),
    CHECK (action IN ('SEND', 'EDIT', 'REVOKE', 'DELETE')),
    CHECK (min_role IN ('OWNER', 'ADMIN', 'MEMBER')),
    CHECK (classification <> ''),
    CHECK (source <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_conversation_role_action_rules_updated
    ON policy_conversation_role_action_rules (tenant_id, updated_at DESC);
