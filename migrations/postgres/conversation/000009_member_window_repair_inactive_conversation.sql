ALTER TABLE conversation_member_window_repair_audit
    ADD COLUMN IF NOT EXISTS conversation_status TEXT,
    ADD COLUMN IF NOT EXISTS previous_member_status TEXT,
    ADD COLUMN IF NOT EXISTS new_member_status TEXT;

ALTER TABLE conversation_member_window_repair_audit
    DROP CONSTRAINT IF EXISTS ck_conversation_member_window_repair_issue;

ALTER TABLE conversation_member_window_repair_audit
    ADD CONSTRAINT ck_conversation_member_window_repair_issue
    CHECK (
        issue_class IN (
            'ACTIVE_WITH_LEAVE_SEQ',
            'INACTIVE_WITHOUT_LEAVE_SEQ',
            'LEAVE_BEFORE_JOIN',
            'MEMBER_VERSION_AHEAD_CONVERSATION',
            'PERMISSION_VERSION_AHEAD_CONVERSATION',
            'ACTIVE_MEMBER_IN_INACTIVE_CONVERSATION'
        )
    ) NOT VALID;

ALTER TABLE conversation_member_window_repair_audit
    DROP CONSTRAINT IF EXISTS ck_conversation_member_window_repair_action;

ALTER TABLE conversation_member_window_repair_audit
    ADD CONSTRAINT ck_conversation_member_window_repair_action
    CHECK (
        repair_action IN (
            'clear_active_leave_seq',
            'set_inactive_leave_seq',
            'clamp_leave_to_join_seq',
            'raise_conversation_member_version',
            'raise_conversation_permission_version',
            'mark_active_member_left_in_inactive_conversation'
        )
    ) NOT VALID;
