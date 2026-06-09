ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS archived BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_user_conversation_summaries_list_archived
    ON user_conversation_summaries (tenant_id, user_id, archived, sort_updated_at DESC, conversation_id);
