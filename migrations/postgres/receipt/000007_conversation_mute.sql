ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS muted BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS muted_at TIMESTAMPTZ;
