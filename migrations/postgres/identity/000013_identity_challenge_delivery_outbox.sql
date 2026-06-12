CREATE TABLE IF NOT EXISTS identity_challenge_delivery_outbox (
    id                 BIGSERIAL   PRIMARY KEY,
    tenant_id          TEXT        NOT NULL,
    user_id            TEXT        NOT NULL,
    challenge_id       TEXT        NOT NULL,
    challenge_type     TEXT        NOT NULL,
    channel            TEXT        NOT NULL,
    destination        TEXT        NOT NULL,
    token_ciphertext   TEXT        NOT NULL,
    token_nonce        TEXT        NOT NULL,
    token_key_version  TEXT        NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    trace_id           TEXT        NOT NULL DEFAULT '',
    request_id         TEXT        NOT NULL DEFAULT '',
    status             TEXT        NOT NULL DEFAULT 'PENDING',
    retry_count        INT         NOT NULL DEFAULT 0,
    available_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at      TIMESTAMPTZ,
    delivered_at       TIMESTAMPTZ,
    dead_lettered_at   TIMESTAMPTZ,
    last_error         TEXT        NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, challenge_id),
    FOREIGN KEY (tenant_id, user_id, challenge_id)
        REFERENCES identity_challenges (tenant_id, user_id, challenge_id)
        ON DELETE CASCADE,
    CHECK (challenge_type IN ('EMAIL_VERIFICATION', 'PHONE_VERIFICATION', 'PASSWORD_RESET')),
    CHECK (channel IN ('EMAIL', 'PHONE')),
    CHECK (status IN ('PENDING', 'DELIVERED', 'DLQ', 'CANCELED')),
    CHECK (retry_count >= 0),
    CHECK (expires_at > created_at),
    CHECK (status <> 'DELIVERED' OR delivered_at IS NOT NULL),
    CHECK (status <> 'DLQ' OR dead_lettered_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_identity_challenge_delivery_ready
    ON identity_challenge_delivery_outbox (status, (COALESCE(next_retry_at, available_at)), id)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_identity_challenge_delivery_dlq
    ON identity_challenge_delivery_outbox (tenant_id, status, dead_lettered_at)
    WHERE status = 'DLQ';
