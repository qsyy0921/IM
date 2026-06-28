CREATE TABLE IF NOT EXISTS delivery_timeline_items (
    tenant_id           TEXT        NOT NULL,
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
    PRIMARY KEY (tenant_id, conversation_id, conversation_seq),
    UNIQUE (tenant_id, event_id),
    CHECK (fanout_mode IN ('WRITE_FANOUT', 'HYBRID_FANOUT', 'READ_FANOUT', 'BROADCAST_SIGNAL'))
);

CREATE INDEX IF NOT EXISTS idx_delivery_timeline_pull
    ON delivery_timeline_items (tenant_id, conversation_id, conversation_seq);

CREATE INDEX IF NOT EXISTS idx_delivery_timeline_fanout_mode
    ON delivery_timeline_items (tenant_id, conversation_id, fanout_mode, conversation_seq);

CREATE TABLE IF NOT EXISTS delivery_user_hidden_timeline_items (
    tenant_id           TEXT        NOT NULL,
    user_id             TEXT        NOT NULL,
    device_id           TEXT        NOT NULL DEFAULT '',
    conversation_id     TEXT        NOT NULL,
    conversation_seq    BIGINT      NOT NULL,
    message_id          TEXT        NOT NULL DEFAULT '',
    reason              TEXT        NOT NULL DEFAULT '',
    hidden_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, conversation_id, conversation_seq)
);

CREATE INDEX IF NOT EXISTS idx_delivery_hidden_timeline_items_pull
    ON delivery_user_hidden_timeline_items (tenant_id, user_id, conversation_id, conversation_seq);
