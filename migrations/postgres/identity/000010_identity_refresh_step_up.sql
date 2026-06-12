ALTER TABLE identity_sessions
    ADD COLUMN IF NOT EXISTS mfa_verified_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS mfa_method TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS mfa_factor_id TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ck_identity_sessions_mfa_method'
          AND conrelid = 'identity_sessions'::regclass
    ) THEN
        ALTER TABLE identity_sessions
            ADD CONSTRAINT ck_identity_sessions_mfa_method
            CHECK (mfa_method IN ('', 'TOTP', 'RECOVERY_CODE'));
    END IF;
END $$;
