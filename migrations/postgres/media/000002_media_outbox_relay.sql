ALTER TABLE media_outbox
    ADD COLUMN IF NOT EXISTS id BIGSERIAL;

ALTER TABLE media_outbox
    ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_media_outbox_id
    ON media_outbox (id);
