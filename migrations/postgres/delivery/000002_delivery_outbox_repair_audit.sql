CREATE TABLE IF NOT EXISTS delivery_outbox_repair_audit (
    id                          BIGSERIAL   PRIMARY KEY,
    outbox_id                   BIGINT      NOT NULL,
    event_id                    TEXT        NOT NULL,
    tenant_id                   TEXT        NOT NULL,
    conversation_id             TEXT        NOT NULL,
    aggregate_version           BIGINT      NOT NULL,
    mode                        TEXT        NOT NULL,
    outcome                     TEXT        NOT NULL,
    skip_reason                 TEXT        NOT NULL DEFAULT '',
    operator                    TEXT        NOT NULL,
    reason                      TEXT        NOT NULL,
    dry_run                     BOOLEAN     NOT NULL DEFAULT FALSE,
    before_status               TEXT        NOT NULL,
    before_retry_count          INT         NOT NULL,
    before_last_error           TEXT        NOT NULL DEFAULT '',
    before_next_retry_at        TIMESTAMPTZ,
    before_dead_lettered_at     TIMESTAMPTZ,
    after_status                TEXT        NOT NULL,
    after_retry_count           INT         NOT NULL,
    after_last_error            TEXT        NOT NULL DEFAULT '',
    after_next_retry_at         TIMESTAMPTZ,
    after_dead_lettered_at      TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_delivery_outbox_repair_audit_outbox
    ON delivery_outbox_repair_audit (outbox_id, created_at DESC);
