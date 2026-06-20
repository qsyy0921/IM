CREATE TABLE IF NOT EXISTS presence_user_states (
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    actual_state TEXT NOT NULL DEFAULT 'OFFLINE',
    visible_state TEXT NOT NULL DEFAULT 'OFFLINE',
    manual_status TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    device_count INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id),
    CONSTRAINT ck_presence_user_states_actual
        CHECK (actual_state IN ('OFFLINE', 'ONLINE', 'AWAY', 'DO_NOT_DISTURB', 'INVISIBLE')),
    CONSTRAINT ck_presence_user_states_visible
        CHECK (visible_state IN ('OFFLINE', 'ONLINE', 'AWAY', 'DO_NOT_DISTURB', 'UNKNOWN')),
    CONSTRAINT ck_presence_user_states_device_count
        CHECK (device_count >= 0)
);

CREATE TABLE IF NOT EXISTS presence_sessions (
    tenant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    presence_state TEXT NOT NULL,
    device_state TEXT NOT NULL,
    source TEXT NOT NULL,
    manual_status TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_idempotency_key TEXT NOT NULL DEFAULT '',
    last_command_hash TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, session_id),
    CONSTRAINT ck_presence_sessions_presence_state
        CHECK (presence_state IN ('OFFLINE', 'ONLINE', 'AWAY', 'DO_NOT_DISTURB', 'INVISIBLE')),
    CONSTRAINT ck_presence_sessions_device_state
        CHECK (device_state IN ('CONNECTED', 'HEARTBEAT_ACTIVE', 'STALE', 'DISCONNECTED', 'REVOKED')),
    CONSTRAINT ck_presence_sessions_source
        CHECK (source IN ('PUSH_GATEWAY', 'CLIENT', 'OPERATOR'))
);

CREATE INDEX IF NOT EXISTS idx_presence_sessions_user
    ON presence_sessions (tenant_id, user_id, expires_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uq_presence_sessions_idempotency
    ON presence_sessions (tenant_id, user_id, last_idempotency_key)
    WHERE last_idempotency_key <> '';

CREATE TABLE IF NOT EXISTS presence_subscriptions (
    tenant_id TEXT NOT NULL,
    subscriber_user_id TEXT NOT NULL,
    target_user_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, subscriber_user_id, target_user_id),
    CONSTRAINT ck_presence_subscriptions_status
        CHECK (status IN ('ACTIVE', 'DISABLED'))
);

CREATE TABLE IF NOT EXISTS presence_typing_indicators (
    tenant_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    typing_state TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, user_id, device_id),
    CONSTRAINT ck_presence_typing_indicators_state
        CHECK (typing_state IN ('STARTED', 'STOPPED'))
);

CREATE INDEX IF NOT EXISTS idx_presence_typing_expires
    ON presence_typing_indicators (tenant_id, expires_at);

CREATE TABLE IF NOT EXISTS presence_outbox (
    event_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_version INTEGER NOT NULL DEFAULT 1,
    partition_key TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    retry_count INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at TIMESTAMPTZ NULL,
    published_at TIMESTAMPTZ NULL,
    dead_lettered_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_presence_outbox_status
        CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);

CREATE INDEX IF NOT EXISTS idx_presence_outbox_ready
    ON presence_outbox (status, available_at, next_retry_at, partition_key);
