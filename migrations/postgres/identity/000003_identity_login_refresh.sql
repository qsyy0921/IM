ALTER TABLE identity_users
    ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS password_updated_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS identity_refresh_tokens (
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    token_id TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    replaced_by_token_id TEXT NOT NULL DEFAULT '',
    trace_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, device_id, session_id, token_id),
    UNIQUE (token_hash),
    CHECK (status IN ('ACTIVE', 'USED', 'REVOKED'))
);

CREATE INDEX IF NOT EXISTS idx_identity_refresh_tokens_session
    ON identity_refresh_tokens (tenant_id, user_id, device_id, session_id, status, expires_at DESC);

CREATE INDEX IF NOT EXISTS idx_identity_refresh_tokens_active_expiry
    ON identity_refresh_tokens (status, expires_at)
    WHERE status = 'ACTIVE';

CREATE UNIQUE INDEX IF NOT EXISTS uq_identity_refresh_tokens_active_session
    ON identity_refresh_tokens (tenant_id, user_id, device_id, session_id)
    WHERE status = 'ACTIVE';
