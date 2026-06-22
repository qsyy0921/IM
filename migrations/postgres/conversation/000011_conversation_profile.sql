BEGIN;

ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS avatar_uri TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS profile_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS profile_updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE conversations
    ADD CONSTRAINT ck_conversations_title_length CHECK (length(title) <= 128) NOT VALID,
    ADD CONSTRAINT ck_conversations_avatar_uri_length CHECK (length(avatar_uri) <= 512) NOT VALID,
    ADD CONSTRAINT ck_conversations_profile_version_positive CHECK (profile_version >= 1) NOT VALID;

COMMIT;
