CREATE TABLE IF NOT EXISTS user_inbox (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    event_id            TEXT        NOT NULL,
    event_type          TEXT        NOT NULL,
    message_id          TEXT        NOT NULL DEFAULT '',
    sender_id           TEXT        NOT NULL DEFAULT '',
    payload_json        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    fanout_mode         TEXT        NOT NULL,
    permission_version  BIGINT      NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id, conversation_seq),
    UNIQUE (tenant_id, user_id, event_id)
);

CREATE INDEX IF NOT EXISTS idx_user_inbox_pull
    ON user_inbox (tenant_id, user_id, conversation_id, conversation_seq);

CREATE TABLE IF NOT EXISTS delivery_membership_projection (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    role                TEXT        NOT NULL,
    status              TEXT        NOT NULL,
    join_seq            BIGINT      NOT NULL,
    leave_seq           BIGINT,
    member_version      BIGINT      NOT NULL,
    permission_version  BIGINT      NOT NULL,
    updated_by_event_id TEXT        NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_delivery_membership_visible
    ON delivery_membership_projection (tenant_id, conversation_id, status, join_seq, leave_seq);

CREATE TABLE IF NOT EXISTS device_delivery_cursors (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    device_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    last_received_seq   BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, device_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS delivery_kafka_checkpoints (
    consumer_group      TEXT        NOT NULL,
    topic               TEXT        NOT NULL,
    partition_id        INT         NOT NULL,
    offset_value        BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_group, topic, partition_id)
);

CREATE TABLE IF NOT EXISTS delivery_outbox (
    id                  BIGSERIAL   PRIMARY KEY,
    event_id            TEXT        NOT NULL UNIQUE,
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    aggregate_version   BIGINT      NOT NULL,
    event_type          TEXT        NOT NULL,
    event_version       TEXT        NOT NULL,
    partition_key       TEXT        NOT NULL,
    mapping_version     BIGINT      NOT NULL,
    correlation_id      TEXT        NOT NULL DEFAULT '',
    causation_id        TEXT        NOT NULL DEFAULT '',
    producer            TEXT        NOT NULL,
    trace_id            TEXT        NOT NULL DEFAULT '',
    payload_json        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    status              TEXT        NOT NULL DEFAULT 'PENDING',
    retry_count         INT         NOT NULL DEFAULT 0,
    last_error          TEXT        NOT NULL DEFAULT '',
    available_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at       TIMESTAMPTZ,
    dead_lettered_at    TIMESTAMPTZ,
    published_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_delivery_outbox_ready
    ON delivery_outbox (status, available_at, id);

CREATE INDEX IF NOT EXISTS idx_delivery_outbox_aggregate
    ON delivery_outbox (tenant_id, conversation_id, aggregate_version);

CREATE INDEX IF NOT EXISTS idx_delivery_outbox_pending_ready_expr
    ON delivery_outbox ((COALESCE(next_retry_at, available_at)), id)
    WHERE status = 'PENDING' AND published_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_delivery_outbox_blocking_aggregate
    ON delivery_outbox (tenant_id, conversation_id, aggregate_version)
    WHERE status IN ('PENDING', 'DLQ');

CREATE INDEX IF NOT EXISTS idx_delivery_outbox_pending_conversation_version
    ON delivery_outbox (tenant_id, conversation_id, aggregate_version, id)
    WHERE status = 'PENDING' AND published_at IS NULL;
