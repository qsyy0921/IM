DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ck_identity_sessions_mfa_empty_proof'
          AND conrelid = 'identity_sessions'::regclass
    ) THEN
        ALTER TABLE identity_sessions
            ADD CONSTRAINT ck_identity_sessions_mfa_empty_proof
            CHECK (
                mfa_method <> ''
                OR (mfa_verified_at IS NULL AND mfa_factor_id = '')
            ) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ck_identity_sessions_mfa_totp_proof'
          AND conrelid = 'identity_sessions'::regclass
    ) THEN
        ALTER TABLE identity_sessions
            ADD CONSTRAINT ck_identity_sessions_mfa_totp_proof
            CHECK (
                mfa_method <> 'TOTP'
                OR (mfa_verified_at IS NOT NULL AND mfa_factor_id <> '')
            ) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ck_identity_sessions_mfa_recovery_proof'
          AND conrelid = 'identity_sessions'::regclass
    ) THEN
        ALTER TABLE identity_sessions
            ADD CONSTRAINT ck_identity_sessions_mfa_recovery_proof
            CHECK (
                mfa_method <> 'RECOVERY_CODE'
                OR (mfa_verified_at IS NOT NULL AND mfa_factor_id = '')
            ) NOT VALID;
    END IF;
END $$;
