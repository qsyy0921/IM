ALTER TABLE policy_decision_audit_outbox
    ADD COLUMN IF NOT EXISTS aggregate_type TEXT NOT NULL DEFAULT 'policy_decision',
    ADD COLUMN IF NOT EXISTS aggregate_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS aggregate_version BIGINT GENERATED ALWAYS AS (id) STORED,
    ADD COLUMN IF NOT EXISTS mapping_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS causation_id TEXT NOT NULL DEFAULT '';

UPDATE policy_decision_audit_outbox
SET aggregate_id = partition_key
WHERE aggregate_id = '';

UPDATE policy_decision_audit_outbox
SET causation_id = correlation_id
WHERE causation_id = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_policy_decision_audit_outbox_aggregate_type'
    ) THEN
        ALTER TABLE policy_decision_audit_outbox
            ADD CONSTRAINT ck_policy_decision_audit_outbox_aggregate_type
            CHECK (aggregate_type <> '') NOT VALID;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_policy_decision_audit_outbox_aggregate_id'
    ) THEN
        ALTER TABLE policy_decision_audit_outbox
            ADD CONSTRAINT ck_policy_decision_audit_outbox_aggregate_id
            CHECK (aggregate_id <> '') NOT VALID;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_policy_decision_audit_outbox_aggregate_version'
    ) THEN
        ALTER TABLE policy_decision_audit_outbox
            ADD CONSTRAINT ck_policy_decision_audit_outbox_aggregate_version
            CHECK (aggregate_version > 0) NOT VALID;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_policy_decision_audit_outbox_mapping_version'
    ) THEN
        ALTER TABLE policy_decision_audit_outbox
            ADD CONSTRAINT ck_policy_decision_audit_outbox_mapping_version
            CHECK (mapping_version > 0) NOT VALID;
    END IF;
END $$;
