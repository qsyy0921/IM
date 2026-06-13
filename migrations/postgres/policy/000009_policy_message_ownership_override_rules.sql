CREATE TABLE IF NOT EXISTS policy_message_ownership_override_rules (
    tenant_id      TEXT        NOT NULL,
    action         TEXT        NOT NULL,
    min_role       TEXT        NOT NULL,
    classification TEXT        NOT NULL,
    reason         TEXT        NOT NULL DEFAULT '',
    source         TEXT        NOT NULL DEFAULT 'manual',
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, action),
    CHECK (action IN ('EDIT', 'REVOKE', 'DELETE')),
    CHECK (min_role IN ('OWNER', 'ADMIN')),
    CHECK (classification <> ''),
    CHECK (source <> '')
);

CREATE INDEX IF NOT EXISTS idx_policy_message_ownership_override_rules_updated
    ON policy_message_ownership_override_rules (tenant_id, updated_at DESC);
