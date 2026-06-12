ALTER TABLE identity_challenges
    ADD COLUMN IF NOT EXISTS delivery_failure_class TEXT NOT NULL DEFAULT '';

ALTER TABLE identity_challenge_delivery_outbox
    ADD COLUMN IF NOT EXISTS failure_class TEXT NOT NULL DEFAULT '';

ALTER TABLE identity_challenge_delivery_repair_audit
    ADD COLUMN IF NOT EXISTS previous_failure_class TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS new_failure_class TEXT NOT NULL DEFAULT '';

ALTER TABLE identity_challenges
    DROP CONSTRAINT IF EXISTS ck_identity_challenges_delivery_failure_class,
    ADD CONSTRAINT ck_identity_challenges_delivery_failure_class
        CHECK (delivery_failure_class IN ('', 'configuration', 'provider_non_success', 'timeout', 'network', 'serialization', 'token_crypto', 'delivery_failed', 'canceled', 'inactive', 'unknown')) NOT VALID;

ALTER TABLE identity_challenge_delivery_outbox
    DROP CONSTRAINT IF EXISTS ck_identity_challenge_delivery_outbox_failure_class,
    ADD CONSTRAINT ck_identity_challenge_delivery_outbox_failure_class
        CHECK (failure_class IN ('', 'configuration', 'provider_non_success', 'timeout', 'network', 'serialization', 'token_crypto', 'delivery_failed', 'canceled', 'inactive', 'unknown')) NOT VALID;

ALTER TABLE identity_challenge_delivery_repair_audit
    DROP CONSTRAINT IF EXISTS ck_identity_challenge_delivery_repair_audit_previous_failure_class,
    ADD CONSTRAINT ck_identity_challenge_delivery_repair_audit_previous_failure_class
        CHECK (previous_failure_class IN ('', 'configuration', 'provider_non_success', 'timeout', 'network', 'serialization', 'token_crypto', 'delivery_failed', 'canceled', 'inactive', 'unknown')) NOT VALID,
    DROP CONSTRAINT IF EXISTS ck_identity_challenge_delivery_repair_audit_new_failure_class,
    ADD CONSTRAINT ck_identity_challenge_delivery_repair_audit_new_failure_class
        CHECK (new_failure_class IN ('', 'configuration', 'provider_non_success', 'timeout', 'network', 'serialization', 'token_crypto', 'delivery_failed', 'canceled', 'inactive', 'unknown')) NOT VALID;

CREATE INDEX IF NOT EXISTS idx_identity_challenges_delivery_failure_class
    ON identity_challenges (tenant_id, delivery_failure_class, delivery_failed_at DESC, challenge_id)
    WHERE delivery_failure_class <> '';

CREATE INDEX IF NOT EXISTS idx_identity_challenge_delivery_outbox_failure_class
    ON identity_challenge_delivery_outbox (status, failure_class, updated_at DESC)
    WHERE failure_class <> '';
