ALTER TABLE delivery_projection_checkpoint_repair_audit
    ADD COLUMN IF NOT EXISTS failure_offset_value BIGINT,
    ADD COLUMN IF NOT EXISTS failure_event_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS failure_class TEXT NOT NULL DEFAULT '';
