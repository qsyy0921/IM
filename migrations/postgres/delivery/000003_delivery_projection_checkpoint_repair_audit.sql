CREATE TABLE IF NOT EXISTS delivery_projection_checkpoint_repair_audit (
    id                  BIGSERIAL   PRIMARY KEY,
    consumer_group      TEXT        NOT NULL,
    topic               TEXT        NOT NULL,
    partition_id        INT         NOT NULL,
    mode                TEXT        NOT NULL,
    outcome             TEXT        NOT NULL,
    skip_reason         TEXT        NOT NULL DEFAULT '',
    operator            TEXT        NOT NULL,
    reason              TEXT        NOT NULL,
    dry_run             BOOLEAN     NOT NULL DEFAULT FALSE,
    before_offset_value BIGINT      NOT NULL,
    after_offset_value  BIGINT      NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_delivery_projection_checkpoint_repair_audit_checkpoint
    ON delivery_projection_checkpoint_repair_audit (consumer_group, topic, partition_id, created_at DESC);
