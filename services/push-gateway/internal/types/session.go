package types

import "time"

const (
	DefaultHeartbeatInterval = 30 * time.Second
	DefaultSessionQueueSize  = 256
)

type SessionRegistration struct {
	AuthContext AuthContext
	SessionID   string
	ResumeToken string
	Outbound    chan<- ServerFrame
}

type ConnectSessionCommand struct {
	AuthContext       AuthContext
	QueueSize         int
	HeartbeatInterval time.Duration
}

type ConnectSessionResult struct {
	SessionID           string
	ResumeToken         string
	HeartbeatIntervalMS int64
}

type DeliveryNotification struct {
	EventID         string
	TenantID        string
	UserID          string
	ConversationID  string
	ConversationSeq int64
	SourceEventID   string
	MessageID       string
	CorrelationID   string
}

type NotifyDeliveryCommand struct {
	Notification DeliveryNotification
}

type NotifyDeliveryResult struct {
	MatchedSessions int
	Enqueued        int
	Dropped         int
}

type AckDeliveryCommand struct {
	AuthContext    AuthContext
	ConversationID string
	ReceivedSeq    int64
}

type AckDeliveryResult struct {
	TenantID        string
	UserID          string
	DeviceID        string
	ConversationID  string
	LastReceivedSeq int64
}

type DeliveryEventMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Value     []byte
}
