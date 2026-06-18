ALTER TABLE policy_decision_audit_outbox
    ADD COLUMN IF NOT EXISTS decision_source TEXT NOT NULL DEFAULT 'UNSPECIFIED';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_policy_decision_audit_outbox_decision_source'
    ) THEN
        ALTER TABLE policy_decision_audit_outbox
            ADD CONSTRAINT ck_policy_decision_audit_outbox_decision_source
            CHECK (decision_source <> '') NOT VALID;
    END IF;
END $$;

