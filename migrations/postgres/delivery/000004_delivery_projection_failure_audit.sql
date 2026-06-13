CREATE TABLE IF NOT EXISTS delivery_projection_failures (
    consumer_group      TEXT        NOT NULL,
    topic               TEXT        NOT NULL,
    partition_id        INT         NOT NULL,
    offset_value        BIGINT      NOT NULL,
    event_id            TEXT        NOT NULL DEFAULT '',
    event_type          TEXT        NOT NULL DEFAULT '',
    tenant_id           TEXT        NOT NULL DEFAULT '',
    conversation_id     TEXT        NOT NULL DEFAULT '',
    aggregate_version   BIGINT      NOT NULL DEFAULT 0,
    trace_id            TEXT        NOT NULL DEFAULT '',
    failure_class       TEXT        NOT NULL,
    last_error          TEXT        NOT NULL DEFAULT '',
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    failure_count       BIGINT      NOT NULL DEFAULT 1,
    PRIMARY KEY (consumer_group, topic, partition_id, offset_value)
);

CREATE INDEX IF NOT EXISTS idx_delivery_projection_failures_last_seen
    ON delivery_projection_failures (last_seen_at DESC);
