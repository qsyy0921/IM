CREATE TABLE IF NOT EXISTS identity_challenge_delivery_repair_audit (
    id                                      BIGSERIAL   PRIMARY KEY,
    delivery_id                             BIGINT      NOT NULL,
    tenant_id                               TEXT        NOT NULL,
    user_id                                 TEXT        NOT NULL,
    challenge_id                            TEXT        NOT NULL,
    previous_delivery_status                TEXT        NOT NULL,
    previous_challenge_status               TEXT        NOT NULL,
    previous_challenge_delivery_status      TEXT        NOT NULL,
    previous_retry_count                    INT         NOT NULL,
    previous_last_error                     TEXT        NOT NULL DEFAULT '',
    previous_dead_lettered_at               TIMESTAMPTZ,
    new_delivery_status                     TEXT        NOT NULL,
    new_challenge_status                    TEXT        NOT NULL,
    new_challenge_delivery_status           TEXT        NOT NULL,
    repair_mode                             TEXT        NOT NULL DEFAULT '',
    repair_outcome                          TEXT        NOT NULL DEFAULT '',
    skip_reason                             TEXT        NOT NULL DEFAULT '',
    dry_run                                 BOOLEAN     NOT NULL DEFAULT false,
    repair_operator                         TEXT        NOT NULL DEFAULT '',
    repair_reason                           TEXT        NOT NULL DEFAULT '',
    repaired_at                             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (repair_mode IN ('audit', 'redrive-active-pending', 'cancel-inactive')),
    CHECK (repair_outcome IN ('AUDITED', 'MUTATED', 'SKIPPED'))
);

CREATE INDEX IF NOT EXISTS idx_identity_challenge_delivery_repair_audit_delivery
    ON identity_challenge_delivery_repair_audit (delivery_id, repaired_at DESC);

CREATE INDEX IF NOT EXISTS idx_identity_challenge_delivery_repair_audit_challenge
    ON identity_challenge_delivery_repair_audit (tenant_id, user_id, challenge_id, repaired_at DESC);
