ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS pinned BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS pinned_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_user_conversation_summaries_list_pinned
    ON user_conversation_summaries (tenant_id, user_id, archived, pinned DESC, sort_updated_at DESC, conversation_id);
