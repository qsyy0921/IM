ALTER TABLE identity_challenges
    ADD COLUMN IF NOT EXISTS delivery_status TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS delivery_attempt_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_failed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE identity_challenges
    DROP CONSTRAINT IF EXISTS ck_identity_challenges_delivery_status,
    ADD CONSTRAINT ck_identity_challenges_delivery_status
        CHECK (delivery_status IN ('PENDING', 'DELIVERED', 'FAILED')) NOT VALID,
    DROP CONSTRAINT IF EXISTS ck_identity_challenges_delivery_attempt_count,
    ADD CONSTRAINT ck_identity_challenges_delivery_attempt_count
        CHECK (delivery_attempt_count >= 0) NOT VALID,
    DROP CONSTRAINT IF EXISTS ck_identity_challenges_delivery_delivered_at,
    ADD CONSTRAINT ck_identity_challenges_delivery_delivered_at
        CHECK (delivery_status <> 'DELIVERED' OR delivered_at IS NOT NULL) NOT VALID,
    DROP CONSTRAINT IF EXISTS ck_identity_challenges_delivery_failed_at,
    ADD CONSTRAINT ck_identity_challenges_delivery_failed_at
        CHECK (delivery_status <> 'FAILED' OR delivery_failed_at IS NOT NULL) NOT VALID;

CREATE INDEX IF NOT EXISTS idx_identity_challenges_delivery_status
    ON identity_challenges (tenant_id, delivery_status, delivery_failed_at DESC, challenge_id)
    WHERE delivery_status IN ('PENDING', 'FAILED');
