ALTER TABLE contact_privacy_settings
    ADD COLUMN IF NOT EXISTS allow_profile_visibility BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE contact_tenant_privacy_defaults
    ADD COLUMN IF NOT EXISTS allow_profile_visibility BOOLEAN NOT NULL DEFAULT true;
