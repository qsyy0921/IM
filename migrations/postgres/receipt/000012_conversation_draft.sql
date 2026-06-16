ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS draft_text TEXT NOT NULL DEFAULT '';

ALTER TABLE user_conversation_summaries
    ADD COLUMN IF NOT EXISTS draft_updated_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ck_user_conversation_summaries_draft_text_size'
    ) THEN
        ALTER TABLE user_conversation_summaries
            ADD CONSTRAINT ck_user_conversation_summaries_draft_text_size
                CHECK (length(draft_text) <= 4096) NOT VALID;
    END IF;
END $$;
