ALTER TABLE delivery_projection_failures
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS resolved_checkpoint_offset BIGINT;

CREATE INDEX IF NOT EXISTS idx_delivery_projection_failures_unresolved_last_seen
    ON delivery_projection_failures (last_seen_at DESC)
    WHERE resolved_at IS NULL;
