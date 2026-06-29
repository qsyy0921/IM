package types

const (
	OpClientHello               = "client.hello"
	OpClientPing                = "client.ping"
	OpConversationSubscribe     = "conversation.subscribe"
	OpConversationUnsubscribe   = "conversation.unsubscribe"
	OpConversationSubscribeOK   = "conversation.subscribe.ok"
	OpConversationUnsubscribeOK = "conversation.unsubscribe.ok"
	OpDeliveryAck               = "delivery.ack"
	OpServerHello               = "server.hello"
	OpServerPong                = "server.pong"
	OpDeliveryNotify            = "delivery.notify"
	OpDeliveryHide              = "delivery.hide"
	OpDeliveryAckOK             = "delivery.ack.ok"
	OpResumeHint                = "server.resume_hint"
	OpError                     = "error"
)

type ConversationCursor struct {
	ConversationID string `json:"conversation_id"`
	Seq            int64  `json:"seq"`
}

type ClientFrame struct {
	Op             string               `json:"op"`
	RequestID      string               `json:"request_id,omitempty"`
	DeviceID       string               `json:"device_id,omitempty"`
	ResumeToken    string               `json:"resume_token,omitempty"`
	LastReceived   []ConversationCursor `json:"last_received,omitempty"`
	ConversationID string               `json:"conversation_id,omitempty"`
	ReceivedSeq    int64                `json:"received_seq,omitempty"`
}

type ServerFrame struct {
	Op                string               `json:"op"`
	RequestID         string               `json:"request_id,omitempty"`
	SessionID         string               `json:"session_id,omitempty"`
	ResumeToken       string               `json:"resume_token,omitempty"`
	HeartbeatInterval int64                `json:"heartbeat_interval_ms,omitempty"`
	ServerTimeMS      int64                `json:"server_time_ms,omitempty"`
	EventID           string               `json:"event_id,omitempty"`
	TenantID          string               `json:"tenant_id,omitempty"`
	ConversationID    string               `json:"conversation_id,omitempty"`
	ConversationSeq   int64                `json:"conversation_seq,omitempty"`
	SourceEventID     string               `json:"source_event_id,omitempty"`
	SourceEventType   string               `json:"source_event_type,omitempty"`
	MessageID         string               `json:"message_id,omitempty"`
	CorrelationID     string               `json:"correlation_id,omitempty"`
	PullRequired      bool                 `json:"pull_required,omitempty"`
	LastReceivedSeq   int64                `json:"last_received_seq,omitempty"`
	Reason            string               `json:"reason,omitempty"`
	Conversations     []ConversationCursor `json:"conversations,omitempty"`
	Code              string               `json:"code,omitempty"`
	Message           string               `json:"message,omitempty"`
	Retryable         bool                 `json:"retryable"`
	RetryAfterMS      int64                `json:"retry_after_ms,omitempty"`
}
