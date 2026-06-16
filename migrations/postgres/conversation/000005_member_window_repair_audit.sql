CREATE TABLE IF NOT EXISTS conversation_member_window_repair_audit (
    id                 BIGSERIAL PRIMARY KEY,
    tenant_id          TEXT        NOT NULL,
    conversation_id    TEXT        NOT NULL,
    user_id            TEXT        NOT NULL,
    issue_class        TEXT        NOT NULL,
    repair_action      TEXT        NOT NULL,
    repair_outcome     TEXT        NOT NULL,
    previous_join_seq  BIGINT,
    previous_leave_seq BIGINT,
    new_leave_seq      BIGINT,
    operator_id        TEXT        NOT NULL DEFAULT 'manual',
    repair_reason      TEXT        NOT NULL DEFAULT '',
    dry_run            BOOLEAN     NOT NULL DEFAULT true,
    repaired_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_conversation_member_window_repair_audit_member
    ON conversation_member_window_repair_audit (tenant_id, conversation_id, user_id, repaired_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_conversation_member_window_repair_audit_issue
    ON conversation_member_window_repair_audit (issue_class, repair_outcome, repaired_at DESC, id DESC);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_conversation_member_window_repair_issue'
    ) THEN
        ALTER TABLE conversation_member_window_repair_audit
            ADD CONSTRAINT ck_conversation_member_window_repair_issue
            CHECK (issue_class IN ('ACTIVE_WITH_LEAVE_SEQ')) NOT VALID;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_conversation_member_window_repair_action'
    ) THEN
        ALTER TABLE conversation_member_window_repair_audit
            ADD CONSTRAINT ck_conversation_member_window_repair_action
            CHECK (repair_action IN ('clear_active_leave_seq')) NOT VALID;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'ck_conversation_member_window_repair_outcome'
    ) THEN
        ALTER TABLE conversation_member_window_repair_audit
            ADD CONSTRAINT ck_conversation_member_window_repair_outcome
            CHECK (repair_outcome IN ('AUDITED', 'MUTATED', 'SKIPPED')) NOT VALID;
    END IF;
END $$;
