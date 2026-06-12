CREATE TABLE IF NOT EXISTS identity_challenge_request_limits (
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    challenge_type TEXT NOT NULL,
    channel TEXT NOT NULL,
    target_key TEXT NOT NULL,
    request_count INT NOT NULL DEFAULT 0,
    window_start TIMESTAMPTZ NOT NULL,
    last_request_at TIMESTAMPTZ NOT NULL,
    locked_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, challenge_type, channel, target_key),
    CHECK (challenge_type IN ('PASSWORD_RESET')),
    CHECK (channel IN ('EMAIL', 'PHONE')),
    CHECK (target_key <> ''),
    CHECK (request_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_identity_challenge_request_limits_locked
    ON identity_challenge_request_limits (tenant_id, locked_until)
    WHERE locked_until IS NOT NULL;
