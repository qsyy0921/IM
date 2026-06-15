ALTER TABLE user_inbox
    ADD COLUMN IF NOT EXISTS hidden_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS hidden_by_device_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS hide_reason TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_user_inbox_pull_visible
    ON user_inbox (tenant_id, user_id, conversation_id, conversation_seq)
    WHERE hidden_at IS NULL;
