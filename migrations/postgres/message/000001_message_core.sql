BEGIN;

CREATE TABLE IF NOT EXISTS conversation_seq (
    tenant_id        TEXT        NOT NULL,
    conversation_id  TEXT        NOT NULL,
    current_seq      BIGINT      NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id)
);

CREATE TABLE IF NOT EXISTS message_log (
    tenant_id           TEXT        NOT NULL,
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    message_id          TEXT        NOT NULL,
    sender_id           TEXT        NOT NULL,
    device_id           TEXT        NOT NULL,
    client_msg_id       TEXT        NOT NULL,
    command_hash        TEXT        NOT NULL,
    message_type        TEXT        NOT NULL,
    payload_json        JSONB       NOT NULL,
    status              TEXT        NOT NULL,
    permission_version  BIGINT      NOT NULL,
    classification      TEXT        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at           TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    deleted_at          TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, conversation_id, conversation_seq),
    UNIQUE (tenant_id, message_id),
    UNIQUE (tenant_id, sender_id, device_id, client_msg_id),
    CHECK (status IN ('NORMAL', 'EDITED', 'REVOKED', 'DELETED'))
);

CREATE INDEX IF NOT EXISTS idx_message_log_message
    ON message_log (tenant_id, message_id);

CREATE INDEX IF NOT EXISTS idx_message_log_sender_client
    ON message_log (tenant_id, sender_id, device_id, client_msg_id);

CREATE TABLE IF NOT EXISTS conversation_timeline_events (
    tenant_id                 TEXT        NOT NULL,
    conversation_id           TEXT        NOT NULL,
    seq                       BIGINT      NOT NULL,
    event_id                  TEXT        NOT NULL,
    event_type                TEXT        NOT NULL,
    event_version             TEXT        NOT NULL,
    message_id                TEXT,
    actor_id                  TEXT        NOT NULL,
    fanout_mode               TEXT        NOT NULL,
    fanout_policy_version     BIGINT      NOT NULL,
    permission_version        BIGINT,
    classification            TEXT,
    mapping_version           TEXT        NOT NULL,
    trace_id                  TEXT        NOT NULL,
    payload_json              JSONB       NOT NULL,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, seq),
    UNIQUE (tenant_id, event_id)
);

CREATE INDEX IF NOT EXISTS idx_conversation_timeline_message
    ON conversation_timeline_events (tenant_id, message_id);

CREATE TABLE IF NOT EXISTS message_outbox (
    id                 BIGSERIAL   PRIMARY KEY,
    event_id           TEXT        NOT NULL UNIQUE,
    tenant_id          TEXT        NOT NULL,
    conversation_id    TEXT        NOT NULL,
    aggregate_version  BIGINT      NOT NULL,
    event_type         TEXT        NOT NULL,
    event_version      TEXT        NOT NULL,
    partition_key      TEXT        NOT NULL,
    mapping_version    TEXT        NOT NULL,
    correlation_id     TEXT        NOT NULL,
    causation_id       TEXT        NOT NULL,
    producer           TEXT        NOT NULL DEFAULT 'message-service',
    payload_json       JSONB       NOT NULL,
    trace_id           TEXT        NOT NULL,
    status             TEXT        NOT NULL DEFAULT 'PENDING',
    available_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at      TIMESTAMPTZ,
    published_at       TIMESTAMPTZ,
    retry_count        INT         NOT NULL DEFAULT 0,
    last_error         TEXT,
    dead_lettered_at   TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);

CREATE INDEX IF NOT EXISTS idx_message_outbox_ready
    ON message_outbox (COALESCE(next_retry_at, available_at), id)
    WHERE status = 'PENDING' AND published_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_message_outbox_dlq
    ON message_outbox (dead_lettered_at, id)
    WHERE status = 'DLQ';

CREATE INDEX IF NOT EXISTS idx_message_outbox_conversation_order
    ON message_outbox (tenant_id, conversation_id, aggregate_version, status);

CREATE TABLE IF NOT EXISTS message_change_history (
    tenant_id            TEXT        NOT NULL,
    conversation_id      TEXT        NOT NULL,
    message_id           TEXT        NOT NULL,
    change_version       INT         NOT NULL,
    change_type          TEXT        NOT NULL,
    before_payload_json  JSONB,
    after_payload_json   JSONB,
    before_status        TEXT        NOT NULL,
    after_status         TEXT        NOT NULL,
    changed_by           TEXT        NOT NULL,
    reason               TEXT,
    trace_id             TEXT        NOT NULL,
    changed_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, message_id, change_version),
    CHECK (change_type IN ('EDIT', 'REVOKE', 'DELETE'))
);

CREATE TABLE IF NOT EXISTS message_command_idempotency (
    tenant_id        TEXT        NOT NULL,
    conversation_id  TEXT        NOT NULL,
    command_type     TEXT        NOT NULL,
    idempotency_key  TEXT        NOT NULL,
    message_id       TEXT        NOT NULL,
    command_hash     TEXT        NOT NULL,
    result_json      JSONB       NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, command_type, message_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS seq_allocation_journal (
    tenant_id          TEXT        NOT NULL,
    conversation_id    TEXT        NOT NULL,
    sequencer_epoch    BIGINT      NOT NULL,
    seq                BIGINT      NOT NULL,
    allocation_id      TEXT        NOT NULL,
    allocated_to       TEXT        NOT NULL,
    status             TEXT        NOT NULL,
    allocated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    committed_at       TIMESTAMPTZ,
    gap_marked_at      TIMESTAMPTZ,
    reason             TEXT,
    PRIMARY KEY (tenant_id, conversation_id, seq),
    CHECK (status IN ('ALLOCATED', 'COMMITTED', 'GAP_MARKED'))
);

CREATE INDEX IF NOT EXISTS idx_seq_allocation_journal_status
    ON seq_allocation_journal (status, allocated_at);

CREATE TABLE IF NOT EXISTS timeline_gap_markers (
    tenant_id        TEXT        NOT NULL,
    conversation_id  TEXT        NOT NULL,
    seq              BIGINT      NOT NULL,
    allocation_id    TEXT        NOT NULL,
    sequencer_epoch  BIGINT      NOT NULL,
    reason           TEXT        NOT NULL,
    detected_by      TEXT        NOT NULL,
    detected_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conversation_id, seq)
);

COMMIT;
