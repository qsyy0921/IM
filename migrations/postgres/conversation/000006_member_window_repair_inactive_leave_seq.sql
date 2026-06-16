ALTER TABLE conversation_member_window_repair_audit
    DROP CONSTRAINT IF EXISTS ck_conversation_member_window_repair_issue;

ALTER TABLE conversation_member_window_repair_audit
    ADD CONSTRAINT ck_conversation_member_window_repair_issue
    CHECK (issue_class IN ('ACTIVE_WITH_LEAVE_SEQ', 'INACTIVE_WITHOUT_LEAVE_SEQ')) NOT VALID;

ALTER TABLE conversation_member_window_repair_audit
    DROP CONSTRAINT IF EXISTS ck_conversation_member_window_repair_action;

ALTER TABLE conversation_member_window_repair_audit
    ADD CONSTRAINT ck_conversation_member_window_repair_action
    CHECK (repair_action IN ('clear_active_leave_seq', 'set_inactive_leave_seq')) NOT VALID;
