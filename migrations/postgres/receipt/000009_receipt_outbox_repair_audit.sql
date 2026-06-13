CREATE TABLE IF NOT EXISTS receipt_outbox_repair_audit (
    id                          BIGSERIAL   PRIMARY KEY,
    event_id                    TEXT        NOT NULL,
    tenant_id                   TEXT        NOT NULL,
    previous_status             TEXT        NOT NULL,
    previous_retry_count        INT         NOT NULL,
    previous_last_error         TEXT        NOT NULL DEFAULT '',
    previous_dead_lettered_at   TIMESTAMPTZ,
    repair_reason               TEXT        NOT NULL DEFAULT '',
    repaired_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_receipt_outbox_repair_audit_event
    ON receipt_outbox_repair_audit (event_id, repaired_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_receipt_outbox_repair_audit_tenant
    ON receipt_outbox_repair_audit (tenant_id, repaired_at DESC, id DESC);
