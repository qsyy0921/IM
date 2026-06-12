CREATE TABLE IF NOT EXISTS identity_outbox (
    id                BIGSERIAL   PRIMARY KEY,
    event_id          TEXT        NOT NULL UNIQUE,
    tenant_id         TEXT        NOT NULL,
    aggregate_type    TEXT        NOT NULL,
    aggregate_id      TEXT        NOT NULL,
    aggregate_version BIGINT      NOT NULL,
    event_type        TEXT        NOT NULL,
    event_version     TEXT        NOT NULL,
    mapping_version   INT         NOT NULL,
    partition_key     TEXT        NOT NULL,
    producer          TEXT        NOT NULL,
    correlation_id    TEXT        NOT NULL DEFAULT '',
    causation_id      TEXT        NOT NULL DEFAULT '',
    trace_id          TEXT        NOT NULL DEFAULT '',
    payload_json      JSONB       NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'PENDING',
    retry_count       INT         NOT NULL DEFAULT 0,
    available_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_retry_at     TIMESTAMPTZ,
    published_at      TIMESTAMPTZ,
    dead_lettered_at  TIMESTAMPTZ,
    last_error        TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (aggregate_version > 0),
    CHECK (mapping_version > 0),
    CHECK (retry_count >= 0),
    CHECK (jsonb_typeof(payload_json) = 'object'),
    CHECK (status IN ('PENDING', 'PUBLISHED', 'DLQ'))
);

CREATE INDEX IF NOT EXISTS idx_identity_outbox_ready
    ON identity_outbox (status, COALESCE(next_retry_at, available_at), id)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_identity_outbox_dlq
    ON identity_outbox (tenant_id, status, dead_lettered_at)
    WHERE status = 'DLQ';

CREATE INDEX IF NOT EXISTS idx_identity_outbox_partition_blockers
    ON identity_outbox (tenant_id, partition_key, aggregate_version, status)
    WHERE status IN ('PENDING', 'DLQ');
