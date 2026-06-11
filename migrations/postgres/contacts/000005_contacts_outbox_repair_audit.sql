CREATE TABLE IF NOT EXISTS contacts_outbox_repair_audit (
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

CREATE INDEX IF NOT EXISTS idx_contacts_outbox_repair_audit_event
    ON contacts_outbox_repair_audit (tenant_id, event_id, repaired_at DESC);
