BEGIN;

ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS avatar_uri TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS profile_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS profile_updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'conversations'::regclass
          AND conname = 'ck_conversations_title_length'
    ) THEN
        ALTER TABLE conversations
            ADD CONSTRAINT ck_conversations_title_length CHECK (length(title) <= 128) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'conversations'::regclass
          AND conname = 'ck_conversations_avatar_uri_length'
    ) THEN
        ALTER TABLE conversations
            ADD CONSTRAINT ck_conversations_avatar_uri_length CHECK (length(avatar_uri) <= 512) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'conversations'::regclass
          AND conname = 'ck_conversations_profile_version_positive'
    ) THEN
        ALTER TABLE conversations
            ADD CONSTRAINT ck_conversations_profile_version_positive CHECK (profile_version >= 1) NOT VALID;
    END IF;
END;
$$;

COMMIT;
