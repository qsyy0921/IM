BEGIN;

ALTER TABLE contact_requests
    ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'DIRECT',
    ADD COLUMN IF NOT EXISTS source_ref  TEXT NOT NULL DEFAULT '';

ALTER TABLE contact_requests
    DROP CONSTRAINT IF EXISTS contact_requests_source_type_check;

ALTER TABLE contact_requests
    ADD CONSTRAINT contact_requests_source_type_check
    CHECK (source_type IN (
        'DIRECT',
        'SEARCH',
        'GROUP',
        'INVITE_LINK',
        'QR_CODE',
        'IMPORT'
    ));

ALTER TABLE contact_requests
    DROP CONSTRAINT IF EXISTS contact_requests_source_ref_length_check;

ALTER TABLE contact_requests
    ADD CONSTRAINT contact_requests_source_ref_length_check
    CHECK (char_length(source_ref) <= 256);

COMMIT;
