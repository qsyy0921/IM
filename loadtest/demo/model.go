package main

import "time"

type summary struct {
	Commit                 string                   `json:"commit"`
	CommitFull             string                   `json:"commit_full"`
	GitDirty               bool                     `json:"git_dirty"`
	ResultDir              string                   `json:"result_dir"`
	TenantID               string                   `json:"tenant_id"`
	ConversationID         string                   `json:"conversation_id"`
	SenderUserID           string                   `json:"sender_user_id"`
	ReceiverUserID         string                   `json:"receiver_user_id"`
	ReceiverDeviceID       string                   `json:"receiver_device_id"`
	ConversationTLSEnabled bool                     `json:"conversation_tls_enabled"`
	MessageTLSEnabled      bool                     `json:"message_tls_enabled"`
	DeliveryTLSEnabled     bool                     `json:"delivery_tls_enabled"`
	ReceiptTLSEnabled      bool                     `json:"receipt_tls_enabled"`
	PushTLSEnabled         bool                     `json:"push_tls_enabled"`
	VerifiedAuthMetadata   bool                     `json:"verified_auth_metadata"`
	GatewayFacade          bool                     `json:"gateway_facade"`
	GatewayAuthMode        string                   `json:"gateway_auth_mode,omitempty"`
	GatewayAuthAudience    string                   `json:"gateway_auth_audience,omitempty"`
	StartedAt              time.Time                `json:"started_at"`
	FinishedAt             time.Time                `json:"finished_at"`
	Success                bool                     `json:"success"`
	Error                  string                   `json:"error,omitempty"`
	ServerHello            serverFrame              `json:"server_hello"`
	MemberJoin             memberJoinSummary        `json:"member_join"`
	SendMessage            sendSummary              `json:"send_message"`
	Notify                 serverFrame              `json:"delivery_notify"`
	PullInbox              pullSummary              `json:"pull_inbox"`
	WebSocketAck           serverFrame              `json:"websocket_ack"`
	MarkRead               markReadSummary          `json:"mark_read"`
	ListBeforeRead         conversationListSummary  `json:"list_conversations_before_read"`
	ListAfterRead          conversationListSummary  `json:"list_conversations_after_read"`
	Postgres               postgresSummary          `json:"postgres"`
	PolicyAuditKafka       *policyAuditKafkaSummary `json:"policy_audit_kafka,omitempty"`
	Capacity               *capacitySummary         `json:"capacity_summary,omitempty"`
}

type memberJoinSummary struct {
	ChangeID    string `json:"change_id"`
	BoundarySeq int64  `json:"boundary_seq"`
	Status      string `json:"status"`
}

type sendSummary struct {
	MessageID       string `json:"message_id"`
	ConversationSeq int64  `json:"conversation_seq"`
}

type pullSummary struct {
	ItemCount int         `json:"item_count"`
	MaxSeq    int64       `json:"max_seq"`
	Items     []inboxItem `json:"items"`
}

type inboxItem struct {
	ConversationSeq int64  `json:"conversation_seq"`
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
}

type markReadSummary struct {
	LastReadSeq int64 `json:"last_read_seq"`
}

type conversationListSummary struct {
	ItemCount int                       `json:"item_count"`
	Items     []conversationSummaryItem `json:"items"`
}

type conversationSummaryItem struct {
	ConversationID string `json:"conversation_id"`
	LastVisibleSeq int64  `json:"last_visible_seq"`
	LastMessageID  string `json:"last_message_id"`
	UnreadCount    int64  `json:"unread_count"`
	LastReadSeq    int64  `json:"last_read_seq"`
}

type postgresSummary struct {
	UserInboxCount            int64 `json:"user_inbox_count"`
	DeviceDeliveryCursorSeq   int64 `json:"device_delivery_cursor_seq"`
	UserReadCursorSeq         int64 `json:"user_read_cursor_seq"`
	UserConversationSummaries int64 `json:"user_conversation_summaries"`
}

type policyAuditKafkaSummary struct {
	Topic             string `json:"topic"`
	EventCount        int64  `json:"event_count"`
	EventID           string `json:"event_id"`
	EventType         string `json:"event_type"`
	Producer          string `json:"producer"`
	Allowed           bool   `json:"allowed"`
	PermissionVersion int64  `json:"permission_version"`
	Classification    string `json:"classification"`
}

type capacitySummary struct {
	DurationSeconds          float64 `json:"duration_seconds"`
	GatewayFacade            bool    `json:"gateway_facade"`
	GatewayAuthMode          string  `json:"gateway_auth_mode,omitempty"`
	UserFacingOperationCount int     `json:"user_facing_operation_count"`
	WebSocketFrameCount      int     `json:"websocket_frame_count"`
	ItemsPulled              int     `json:"items_pulled"`
	MaxConversationSeq       int64   `json:"max_conversation_seq"`
	UnreadBeforeRead         int64   `json:"unread_before_read"`
	UnreadAfterRead          int64   `json:"unread_after_read"`
	PostgresUserInboxCount   int64   `json:"postgres_user_inbox_count"`
	PostgresSummaryCount     int64   `json:"postgres_summary_count"`
	PolicyAuditKafkaEvents   int64   `json:"policy_audit_kafka_events,omitempty"`
	OperationsPerSecond      float64 `json:"operations_per_second"`
}

type clientFrame struct {
	Op             string               `json:"op"`
	RequestID      string               `json:"request_id,omitempty"`
	DeviceID       string               `json:"device_id,omitempty"`
	ResumeToken    string               `json:"resume_token,omitempty"`
	LastReceived   []conversationCursor `json:"last_received,omitempty"`
	ConversationID string               `json:"conversation_id,omitempty"`
	ReceivedSeq    int64                `json:"received_seq,omitempty"`
}

type serverFrame struct {
	Op                string               `json:"op"`
	RequestID         string               `json:"request_id,omitempty"`
	SessionID         string               `json:"session_id,omitempty"`
	ResumeToken       string               `json:"resume_token,omitempty"`
	HeartbeatInterval int64                `json:"heartbeat_interval_ms,omitempty"`
	EventID           string               `json:"event_id,omitempty"`
	TenantID          string               `json:"tenant_id,omitempty"`
	ConversationID    string               `json:"conversation_id,omitempty"`
	ConversationSeq   int64                `json:"conversation_seq,omitempty"`
	SourceEventID     string               `json:"source_event_id,omitempty"`
	SourceEventType   string               `json:"source_event_type,omitempty"`
	MessageID         string               `json:"message_id,omitempty"`
	PullRequired      bool                 `json:"pull_required,omitempty"`
	LastReceivedSeq   int64                `json:"last_received_seq,omitempty"`
	Reason            string               `json:"reason,omitempty"`
	Conversations     []conversationCursor `json:"conversations,omitempty"`
	Code              string               `json:"code,omitempty"`
	Message           string               `json:"message,omitempty"`
	Retryable         bool                 `json:"retryable"`
}

type conversationCursor struct {
	ConversationID string `json:"conversation_id"`
	Seq            int64  `json:"seq"`
}
