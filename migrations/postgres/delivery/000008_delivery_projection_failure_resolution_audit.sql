CREATE TABLE IF NOT EXISTS delivery_projection_failure_resolution_audit (
    id                         BIGSERIAL PRIMARY KEY,
    consumer_group             TEXT        NOT NULL,
    topic                      TEXT        NOT NULL,
    partition_id               INTEGER     NOT NULL,
    offset_value               BIGINT      NOT NULL,
    event_id                   TEXT        NOT NULL DEFAULT '',
    failure_class              TEXT        NOT NULL DEFAULT '',
    operator                   TEXT        NOT NULL,
    reason                     TEXT        NOT NULL,
    dry_run                    BOOLEAN     NOT NULL DEFAULT FALSE,
    outcome                    TEXT        NOT NULL,
    checkpoint_offset_value    BIGINT,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT delivery_projection_failure_resolution_audit_outcome_check
        CHECK (outcome IN ('AUDITED', 'RESOLVED'))
);

CREATE INDEX IF NOT EXISTS idx_delivery_projection_failure_resolution_audit_failure
    ON delivery_projection_failure_resolution_audit (consumer_group, topic, partition_id, offset_value, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_delivery_projection_failure_resolution_audit_operator
    ON delivery_projection_failure_resolution_audit (operator, created_at DESC);
