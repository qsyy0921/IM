ALTER TABLE conversation_timeline_events
    ADD COLUMN IF NOT EXISTS partition_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS correlation_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS causation_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS producer TEXT NOT NULL DEFAULT 'message-service';
