BEGIN;

ALTER TABLE contact_privacy_settings
    ADD COLUMN IF NOT EXISTS allow_search_contact_requests BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE contact_tenant_privacy_defaults
    ADD COLUMN IF NOT EXISTS allow_search_contact_requests BOOLEAN NOT NULL DEFAULT true;

COMMIT;
