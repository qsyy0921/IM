package main

import "time"

type summary struct {
	Commit                  string        `json:"commit"`
	CommitFull              string        `json:"commit_full"`
	GitDirty                bool          `json:"git_dirty"`
	GitStatusShort          string        `json:"git_status_short,omitempty"`
	Target                  string        `json:"target"`
	MessageTLSEnabled       bool          `json:"message_tls_enabled"`
	VerifiedAuthMetadata    bool          `json:"verified_auth_metadata"`
	ResultDir               string        `json:"result_dir"`
	Scenario                string        `json:"scenario"`
	Action                  string        `json:"action"`
	TenantID                string        `json:"tenant_id"`
	UserID                  string        `json:"user_id"`
	ChangeUserID            string        `json:"change_user_id,omitempty"`
	ConversationID          string        `json:"conversation_id"`
	StartedAt               time.Time     `json:"started_at"`
	FinishedAt              time.Time     `json:"finished_at"`
	Success                 bool          `json:"success"`
	Error                   string        `json:"error,omitempty"`
	ExpectedPermissionVer   int64         `json:"expected_permission_version"`
	ExpectedClassification  string        `json:"expected_classification"`
	ExpectedReason          string        `json:"expected_reason,omitempty"`
	SendMessage             sendSummary   `json:"send_message"`
	ChangeMessage           changeSummary `json:"change_message,omitempty"`
	MessageError            errorSummary  `json:"message_error,omitempty"`
	PolicyRuleSeeded        bool          `json:"policy_rule_seeded"`
	TenantPolicyRuleSeeded  bool          `json:"tenant_policy_rule_seeded"`
	ConversationRoleSeeded  bool          `json:"conversation_role_gate_seeded"`
	OwnershipOverrideSeeded bool          `json:"ownership_override_rule_seeded"`
	PolicyAuditExpected     bool          `json:"policy_audit_expected"`
	ExpectedAuditRows       int64         `json:"expected_policy_audit_rows,omitempty"`
	PolicyRule              policyRule    `json:"policy_rule,omitempty"`
	PolicyRules             []policyRule  `json:"policy_rules,omitempty"`
	ConversationRoleRule    roleRule      `json:"conversation_role_rule,omitempty"`
	OwnershipOverrideRule   roleRule      `json:"ownership_override_rule,omitempty"`
	ConversationMember      memberRow     `json:"conversation_member_projection,omitempty"`
	DBBefore                dbStats       `json:"db_before"`
	DBBeforeAction          dbStats       `json:"db_before_action,omitempty"`
	DBAfter                 dbStats       `json:"db_after"`
	MessageRow              messageRow    `json:"message_row,omitempty"`
	ChangeRow               changeRow     `json:"change_row,omitempty"`
	PolicyAudit             policyAudit   `json:"policy_decision_audit,omitempty"`
	LatencyMS               float64       `json:"latency_ms"`
}

type sendSummary struct {
	MessageID        string `json:"message_id,omitempty"`
	ConversationSeq  int64  `json:"conversation_seq,omitempty"`
	IdempotentReplay bool   `json:"idempotent_replay,omitempty"`
	GRPCCode         string `json:"grpc_code"`
}

type changeSummary struct {
	MessageID        string `json:"message_id,omitempty"`
	ConversationSeq  int64  `json:"conversation_seq,omitempty"`
	ChangeVersion    int32  `json:"change_version,omitempty"`
	IdempotentReplay bool   `json:"idempotent_replay,omitempty"`
	GRPCCode         string `json:"grpc_code"`
}

type errorSummary struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type policyRule struct {
	TenantID          string `json:"tenant_id,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	ConversationID    string `json:"conversation_id,omitempty"`
	Action            string `json:"action,omitempty"`
	Allowed           bool   `json:"allowed"`
	PermissionVersion int64  `json:"permission_version,omitempty"`
	Classification    string `json:"classification,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

type roleRule struct {
	TenantID       string `json:"tenant_id,omitempty"`
	Action         string `json:"action,omitempty"`
	MinRole        string `json:"min_role,omitempty"`
	Classification string `json:"classification,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

type memberRow struct {
	TenantID          string `json:"tenant_id,omitempty"`
	ConversationID    string `json:"conversation_id,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	Role              string `json:"role,omitempty"`
	Status            string `json:"status,omitempty"`
	MemberVersion     int64  `json:"member_version,omitempty"`
	PermissionVersion int64  `json:"permission_version,omitempty"`
	UpdatedByEventID  string `json:"updated_by_event_id,omitempty"`
}

type policyAudit struct {
	RowCount          int64  `json:"row_count"`
	EventID           string `json:"event_id,omitempty"`
	Action            string `json:"action,omitempty"`
	MessageIDPresent  bool   `json:"message_id_present,omitempty"`
	MessageKeyPresent bool   `json:"message_key_present,omitempty"`
	Allowed           bool   `json:"allowed"`
	PermissionVersion int64  `json:"permission_version,omitempty"`
	Classification    string `json:"classification,omitempty"`
	ReasonCode        string `json:"reason_code,omitempty"`
	Status            string `json:"status,omitempty"`
}

type dbStats struct {
	MessageLog           int64 `json:"message_log"`
	TimelineEvents       int64 `json:"conversation_timeline_events"`
	MessageOutbox        int64 `json:"message_outbox"`
	MessageChangeHistory int64 `json:"message_change_history"`
	CommandIdempotency   int64 `json:"message_command_idempotency"`
	ConversationSeq      int64 `json:"conversation_seq"`
}

type messageRow struct {
	MessageID                 string `json:"message_id"`
	ConversationSeq           int64  `json:"conversation_seq"`
	MessageStatus             string `json:"message_status"`
	MessagePayload            string `json:"message_payload,omitempty"`
	MessagePermissionVersion  int64  `json:"message_permission_version"`
	MessageClassification     string `json:"message_classification"`
	TimelinePermissionVersion int64  `json:"timeline_permission_version"`
	TimelineClassification    string `json:"timeline_classification"`
	FanoutPolicyVersion       int64  `json:"fanout_policy_version"`
	OutboxStatus              string `json:"outbox_status"`
}

type changeRow struct {
	MessageID                 string `json:"message_id"`
	MessageStatus             string `json:"message_status"`
	MessagePayload            string `json:"message_payload,omitempty"`
	TimelineEventType         string `json:"timeline_event_type"`
	TimelinePermissionVersion int64  `json:"timeline_permission_version"`
	TimelineClassification    string `json:"timeline_classification"`
	OutboxStatus              string `json:"outbox_status"`
	ChangeHistoryRows         int64  `json:"message_change_history_rows"`
	ChangeHistoryType         string `json:"message_change_history_type,omitempty"`
	ChangeHistoryBeforeStatus string `json:"message_change_history_before_status,omitempty"`
	ChangeHistoryAfterStatus  string `json:"message_change_history_after_status,omitempty"`
	EditedAtSet               bool   `json:"edited_at_set,omitempty"`
	RevokedAtSet              bool   `json:"revoked_at_set,omitempty"`
	DeletedAtSet              bool   `json:"deleted_at_set,omitempty"`
}
