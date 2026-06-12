ALTER TABLE identity_users
    ADD COLUMN IF NOT EXISTS mfa_recovery_failed_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS mfa_recovery_failed_last_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS mfa_recovery_locked_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_identity_users_mfa_recovery_locked_until
    ON identity_users (tenant_id, mfa_recovery_locked_until)
    WHERE mfa_recovery_locked_until IS NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_identity_users_mfa_recovery_failed_count_nonnegative'
    ) THEN
        ALTER TABLE identity_users
            ADD CONSTRAINT chk_identity_users_mfa_recovery_failed_count_nonnegative
            CHECK (mfa_recovery_failed_count >= 0);
    END IF;
END $$;
