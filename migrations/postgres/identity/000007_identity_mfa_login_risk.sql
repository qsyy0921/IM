ALTER TABLE identity_mfa_factors
    ADD COLUMN IF NOT EXISTS login_failed_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS login_failed_last_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS login_locked_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_identity_mfa_factors_login_locked_until
    ON identity_mfa_factors (tenant_id, login_locked_until)
    WHERE login_locked_until IS NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_identity_mfa_factors_login_failed_count_nonnegative'
    ) THEN
        ALTER TABLE identity_mfa_factors
            ADD CONSTRAINT chk_identity_mfa_factors_login_failed_count_nonnegative
            CHECK (login_failed_count >= 0);
    END IF;
END $$;
