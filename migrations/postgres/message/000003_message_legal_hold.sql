CREATE TABLE IF NOT EXISTS message_legal_holds (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    hold_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    reason TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_by TEXT NOT NULL DEFAULT '',
    released_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_message_legal_holds_tenant_hold UNIQUE (tenant_id, hold_id),
    CONSTRAINT ck_message_legal_holds_status CHECK (status IN ('ACTIVE', 'RELEASED')),
    CONSTRAINT ck_message_legal_holds_release_fields CHECK (
        (status = 'ACTIVE' AND released_at IS NULL)
        OR (status = 'RELEASED' AND released_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_message_legal_holds_active_message
    ON message_legal_holds (tenant_id, conversation_id, message_id)
    WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_message_legal_holds_status_updated
    ON message_legal_holds (tenant_id, status, updated_at DESC);
