CREATE TABLE IF NOT EXISTS identity_mfa_factors (
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    factor_id TEXT NOT NULL,
    factor_type TEXT NOT NULL,
    status TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    secret_ciphertext TEXT NOT NULL,
    secret_nonce TEXT NOT NULL,
    secret_key_version TEXT NOT NULL DEFAULT 'local-v1',
    created_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    trace_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, user_id, factor_id),
    CHECK (factor_type IN ('TOTP')),
    CHECK (status IN ('PENDING', 'ACTIVE', 'DISABLED')),
    CHECK (secret_ciphertext <> ''),
    CHECK (secret_nonce <> '')
);

CREATE INDEX IF NOT EXISTS idx_identity_mfa_factors_active
    ON identity_mfa_factors (tenant_id, user_id, factor_type, status, updated_at DESC)
    WHERE status IN ('PENDING', 'ACTIVE');
