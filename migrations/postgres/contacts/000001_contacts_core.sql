BEGIN;

CREATE TABLE IF NOT EXISTS contact_requests (
    request_id        TEXT        PRIMARY KEY,
    tenant_id         TEXT        NOT NULL,
    sender_user_id    TEXT        NOT NULL,
    receiver_user_id  TEXT        NOT NULL,
    status            TEXT        NOT NULL,
    idempotency_key   TEXT        NOT NULL,
    command_hash      TEXT        NOT NULL,
    message           TEXT        NOT NULL DEFAULT '',
    decided_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, sender_user_id, idempotency_key),
    CHECK (sender_user_id <> receiver_user_id),
    CHECK (status IN ('PENDING', 'ACCEPTED', 'DECLINED', 'CANCELED', 'EXPIRED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_contact_requests_pending_pair
    ON contact_requests (
        tenant_id,
        LEAST(sender_user_id, receiver_user_id),
        GREATEST(sender_user_id, receiver_user_id)
    )
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_contact_requests_receiver_status
    ON contact_requests (tenant_id, receiver_user_id, status, created_at);

CREATE TABLE IF NOT EXISTS contact_edges (
    tenant_id          TEXT        NOT NULL,
    owner_user_id      TEXT        NOT NULL,
    contact_user_id    TEXT        NOT NULL,
    status             TEXT        NOT NULL,
    source_request_id  TEXT        NOT NULL,
    version            BIGINT      NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, owner_user_id, contact_user_id),
    CHECK (owner_user_id <> contact_user_id),
    CHECK (version > 0),
    CHECK (status IN ('ACTIVE', 'DELETED', 'BLOCKED'))
);

CREATE INDEX IF NOT EXISTS idx_contact_edges_owner_status
    ON contact_edges (tenant_id, owner_user_id, status, contact_user_id);

CREATE TABLE IF NOT EXISTS contact_command_idempotency (
    tenant_id        TEXT        NOT NULL,
    user_id          TEXT        NOT NULL,
    idempotency_key  TEXT        NOT NULL,
    command_type     TEXT        NOT NULL,
    command_hash     TEXT        NOT NULL,
    result_id        TEXT        NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, idempotency_key),
    CHECK (command_type IN ('SEND_CONTACT_REQUEST', 'RESPOND_CONTACT_REQUEST'))
);

CREATE TABLE IF NOT EXISTS contacts_outbox (
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

CREATE INDEX IF NOT EXISTS idx_contacts_outbox_ready
    ON contacts_outbox (status, COALESCE(next_retry_at, available_at), id)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_contacts_outbox_dlq
    ON contacts_outbox (tenant_id, status, dead_lettered_at)
    WHERE status = 'DLQ';

COMMIT;
