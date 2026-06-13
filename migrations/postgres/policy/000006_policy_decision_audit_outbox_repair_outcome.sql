ALTER TABLE policy_decision_audit_outbox_repair_audit
    ADD COLUMN IF NOT EXISTS repair_outcome TEXT NOT NULL DEFAULT 'REPAIRED',
    ADD COLUMN IF NOT EXISTS skip_reason TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_policy_decision_audit_outbox_repair_outcome'
    ) THEN
        ALTER TABLE policy_decision_audit_outbox_repair_audit
            ADD CONSTRAINT ck_policy_decision_audit_outbox_repair_outcome
            CHECK (repair_outcome IN ('REPAIRED', 'SKIPPED')) NOT VALID;
    END IF;
END $$;
