ALTER TABLE receipt_inbox_projection
    ADD COLUMN IF NOT EXISTS source_event_type TEXT NOT NULL DEFAULT 'message.persisted.v1';
