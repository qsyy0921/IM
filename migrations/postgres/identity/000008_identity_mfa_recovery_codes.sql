CREATE TABLE IF NOT EXISTS identity_mfa_recovery_codes (
    tenant_id text NOT NULL,
    user_id text NOT NULL,
    code_id text NOT NULL,
    code_hash text NOT NULL,
    status text NOT NULL,
    created_at timestamptz NOT NULL,
    used_at timestamptz NULL,
    disabled_at timestamptz NULL,
    trace_id text NOT NULL DEFAULT '',
    request_id text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, user_id, code_id),
    CONSTRAINT chk_identity_mfa_recovery_codes_status
        CHECK (status IN ('ACTIVE', 'USED', 'DISABLED')),
    CONSTRAINT chk_identity_mfa_recovery_codes_hash_not_empty
        CHECK (length(code_hash) > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_identity_mfa_recovery_codes_active_hash
    ON identity_mfa_recovery_codes (tenant_id, user_id, code_hash)
    WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_identity_mfa_recovery_codes_active_user
    ON identity_mfa_recovery_codes (tenant_id, user_id, created_at DESC)
    WHERE status = 'ACTIVE';
