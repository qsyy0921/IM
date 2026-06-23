BEGIN;

ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS announcement TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'conversations'::regclass
          AND conname = 'ck_conversations_announcement_length'
    ) THEN
        ALTER TABLE conversations
            ADD CONSTRAINT ck_conversations_announcement_length CHECK (length(announcement) <= 1024) NOT VALID;
    END IF;
END;
$$;

COMMIT;
