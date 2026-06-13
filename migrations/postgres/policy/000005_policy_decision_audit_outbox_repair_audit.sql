CREATE TABLE IF NOT EXISTS policy_decision_audit_outbox_repair_audit (
    id                          BIGSERIAL   PRIMARY KEY,
    event_id                    TEXT        NOT NULL,
    tenant_id                   TEXT        NOT NULL,
    previous_status             TEXT        NOT NULL,
    previous_retry_count        INT         NOT NULL,
    previous_last_error         TEXT        NOT NULL DEFAULT '',
    previous_dead_lettered_at   TIMESTAMPTZ,
    repair_operator             TEXT        NOT NULL DEFAULT '',
    repair_reason               TEXT        NOT NULL DEFAULT '',
    repaired_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE policy_decision_audit_outbox_repair_audit
    ADD COLUMN IF NOT EXISTS repair_operator TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_policy_decision_audit_outbox_repair_audit_event
    ON policy_decision_audit_outbox_repair_audit (tenant_id, event_id, repaired_at DESC);

CREATE INDEX IF NOT EXISTS idx_policy_decision_audit_outbox_dlq
    ON policy_decision_audit_outbox (status, dead_lettered_at, id)
    WHERE status = 'DLQ';

CREATE INDEX IF NOT EXISTS idx_policy_decision_audit_outbox_partition_order
    ON policy_decision_audit_outbox (tenant_id, partition_key, aggregate_version, status);
