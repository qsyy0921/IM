package main

import "time"

type summary struct {
	Commit              string          `json:"commit"`
	CommitFull          string          `json:"commit_full"`
	GitDirty            bool            `json:"git_dirty"`
	ResultDir           string          `json:"result_dir"`
	TenantID            string          `json:"tenant_id"`
	GroupConversationID string          `json:"group_conversation_id"`
	SenderUserID        string          `json:"sender_user_id"`
	ReceiverUserID      string          `json:"receiver_user_id"`
	ReceiverDevice      string          `json:"receiver_device_id"`
	BFFBaseURL          string          `json:"bff_base_url"`
	PushURL             string          `json:"push_url"`
	StartedAt           time.Time       `json:"started_at"`
	FinishedAt          time.Time       `json:"finished_at"`
	Success             bool            `json:"success"`
	Error               string          `json:"error,omitempty"`
	Setup               setupSummary    `json:"setup"`
	Contact             contactSummary  `json:"contact"`
	SenderLogin         loginSummary    `json:"sender_login"`
	ReceiverLogin       loginSummary    `json:"receiver_login"`
	ServerHello         serverFrame     `json:"server_hello"`
	DirectChat          scenarioSummary `json:"direct_chat"`
	GroupChat           scenarioSummary `json:"group_chat"`
}

type setupSummary struct {
	SenderRegistered   bool `json:"sender_registered"`
	ReceiverRegistered bool `json:"receiver_registered"`
}

type contactSummary struct {
	RequestID      string `json:"request_id"`
	SenderActive   bool   `json:"sender_active"`
	ReceiverActive bool   `json:"receiver_active"`
}

type scenarioSummary struct {
	ConversationID    string              `json:"conversation_id"`
	ConversationType  string              `json:"conversation_type"`
	MemberChangeID    string              `json:"member_change_id,omitempty"`
	MemberBoundarySeq int64               `json:"member_boundary_seq,omitempty"`
	SendMessage       sendSummary         `json:"send_message"`
	Notify            serverFrame         `json:"delivery_notify"`
	PullInbox         pullSummary         `json:"pull_inbox"`
	ListConversations conversationSummary `json:"list_conversations"`
	AckDelivery       ackSummary          `json:"ack_delivery"`
	Postgres          postgresSummary     `json:"postgres"`
}

type loginSummary struct {
	SessionID       string `json:"session_id"`
	TokenType       string `json:"token_type"`
	GatewayTokenSet bool   `json:"gateway_token_set"`
	PushTokenSet    bool   `json:"push_gateway_token_set"`
	RefreshTokenSet bool   `json:"refresh_token_set"`
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

type conversationSummary struct {
	ItemCount int                       `json:"item_count"`
	Items     []conversationSummaryItem `json:"items"`
}

type conversationSummaryItem struct {
	ConversationID string `json:"conversation_id"`
	LastVisibleSeq int64  `json:"last_visible_seq"`
	LastMessageID  string `json:"last_message_id"`
	UnreadCount    int64  `json:"unread_count"`
}

type ackSummary struct {
	ConversationID  string `json:"conversation_id"`
	LastReceivedSeq int64  `json:"last_received_seq"`
}

type postgresSummary struct {
	UserInboxCount          int64 `json:"user_inbox_count"`
	DeviceDeliveryCursorSeq int64 `json:"device_delivery_cursor_seq"`
}

type authSession struct {
	TenantID       string
	UserID         string
	DeviceID       string
	SessionID      string
	TokenType      string
	GatewayToken   string
	PushToken      string
	RefreshToken   string
	GatewayExpires int64
	PushExpires    int64
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

type clientFrame struct {
	Op             string               `json:"op"`
	RequestID      string               `json:"request_id,omitempty"`
	DeviceID       string               `json:"device_id,omitempty"`
	ResumeToken    string               `json:"resume_token,omitempty"`
	LastReceived   []conversationCursor `json:"last_received,omitempty"`
	ConversationID string               `json:"conversation_id,omitempty"`
	ReceivedSeq    int64                `json:"received_seq,omitempty"`
}

type conversationCursor struct {
	ConversationID string `json:"conversation_id"`
	Seq            int64  `json:"seq"`
}
