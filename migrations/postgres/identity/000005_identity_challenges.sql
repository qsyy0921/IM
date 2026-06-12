ALTER TABLE identity_users
    ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS phone_verified_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS identity_challenges (
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    challenge_id TEXT NOT NULL,
    challenge_type TEXT NOT NULL,
    status TEXT NOT NULL,
    channel TEXT NOT NULL,
    destination TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    attempt_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    trace_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, challenge_id),
    UNIQUE (token_hash),
    CHECK (challenge_type IN ('EMAIL_VERIFICATION', 'PHONE_VERIFICATION', 'PASSWORD_RESET')),
    CHECK (status IN ('ACTIVE', 'CONSUMED', 'EXPIRED')),
    CHECK (channel IN ('EMAIL', 'PHONE')),
    CHECK (attempt_count >= 0),
    CHECK (max_attempts > 0),
    CHECK (expires_at > issued_at)
);

CREATE INDEX IF NOT EXISTS idx_identity_challenges_active
    ON identity_challenges (tenant_id, user_id, challenge_type, status, expires_at DESC)
    WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_identity_users_email
    ON identity_users (tenant_id, email)
    WHERE email <> '';

CREATE INDEX IF NOT EXISTS idx_identity_users_phone
    ON identity_users (tenant_id, phone)
    WHERE phone <> '';
