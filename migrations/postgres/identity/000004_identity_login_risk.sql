ALTER TABLE identity_users
    ADD COLUMN IF NOT EXISTS failed_login_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS failed_login_last_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_identity_users_locked_until
    ON identity_users (tenant_id, locked_until)
    WHERE locked_until IS NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_identity_users_failed_login_count_nonnegative'
    ) THEN
        ALTER TABLE identity_users
            ADD CONSTRAINT chk_identity_users_failed_login_count_nonnegative
            CHECK (failed_login_count >= 0);
    END IF;
END $$;
