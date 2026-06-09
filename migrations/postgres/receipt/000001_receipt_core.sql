CREATE TABLE IF NOT EXISTS receipt_inbox_projection (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    source_event_id     TEXT        NOT NULL,
    delivery_event_id   TEXT        NOT NULL,
    message_id          TEXT        NOT NULL,
    sender_id           TEXT        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id, conversation_seq),
    UNIQUE (tenant_id, user_id, delivery_event_id)
);

CREATE INDEX IF NOT EXISTS idx_receipt_inbox_message
    ON receipt_inbox_projection (tenant_id, conversation_id, message_id);

CREATE TABLE IF NOT EXISTS device_received_cursors (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    device_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    last_received_seq   BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, device_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS user_received_cursors (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    last_received_seq   BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS user_read_cursors (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    last_read_seq       BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS message_receipt_states (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    message_id          TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    received_at         TIMESTAMPTZ,
    read_at             TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, conversation_seq, user_id)
);

CREATE INDEX IF NOT EXISTS idx_message_receipt_states_message
    ON message_receipt_states (tenant_id, conversation_id, message_id);

CREATE TABLE IF NOT EXISTS receipt_kafka_checkpoints (
    consumer_group      TEXT        NOT NULL,
    topic               TEXT        NOT NULL,
    partition_id        INT         NOT NULL,
    offset_value        BIGINT      NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_group, topic, partition_id)
);

CREATE TABLE IF NOT EXISTS receipt_outbox (
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

CREATE INDEX IF NOT EXISTS idx_receipt_outbox_ready
    ON receipt_outbox (status, available_at, id);

CREATE INDEX IF NOT EXISTS idx_receipt_outbox_aggregate
    ON receipt_outbox (tenant_id, conversation_id, aggregate_version);
