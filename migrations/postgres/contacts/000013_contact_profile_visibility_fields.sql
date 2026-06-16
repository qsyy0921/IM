ALTER TABLE contact_privacy_settings
    ADD COLUMN IF NOT EXISTS profile_visibility_fields TEXT[] NOT NULL DEFAULT ARRAY['DISPLAY_NAME', 'AVATAR', 'ORGANIZATION', 'TITLE']::TEXT[];

ALTER TABLE contact_tenant_privacy_defaults
    ADD COLUMN IF NOT EXISTS profile_visibility_fields TEXT[] NOT NULL DEFAULT ARRAY['DISPLAY_NAME', 'AVATAR', 'ORGANIZATION', 'TITLE']::TEXT[];

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ck_contact_privacy_settings_profile_visibility_fields'
          AND conrelid = 'contact_privacy_settings'::regclass
    ) THEN
        ALTER TABLE contact_privacy_settings
            ADD CONSTRAINT ck_contact_privacy_settings_profile_visibility_fields
            CHECK (profile_visibility_fields <@ ARRAY['DISPLAY_NAME', 'AVATAR', 'ORGANIZATION', 'TITLE', 'STATUS_MESSAGE']::TEXT[])
            NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ck_contact_tenant_privacy_defaults_profile_visibility_fields'
          AND conrelid = 'contact_tenant_privacy_defaults'::regclass
    ) THEN
        ALTER TABLE contact_tenant_privacy_defaults
            ADD CONSTRAINT ck_contact_tenant_privacy_defaults_profile_visibility_fields
            CHECK (profile_visibility_fields <@ ARRAY['DISPLAY_NAME', 'AVATAR', 'ORGANIZATION', 'TITLE', 'STATUS_MESSAGE']::TEXT[])
            NOT VALID;
    END IF;
END $$;
