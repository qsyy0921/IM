ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_user_conversation_summaries_tags
    ON user_conversation_summaries USING GIN (tags);
