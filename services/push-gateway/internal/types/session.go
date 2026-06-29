package types

import "time"

const (
	DefaultHeartbeatInterval = 30 * time.Second
	DefaultSessionQueueSize  = 256
	DefaultResumeBufferSize  = 256
	DefaultResumeBufferTTL   = 10 * time.Minute
)

const (
	DeliveryNotificationKindInboxItemCreated   = "inbox_item_created"
	DeliveryNotificationKindInboxItemHidden    = "inbox_item_hidden"
	DeliveryNotificationKindConversationSignal = "conversation_signal"
)

type SessionRegistration struct {
	AuthContext     AuthContext
	SessionID       string
	ResumeToken     string
	ResumeRequested bool
	LastReceived    []ConversationCursor
	Outbound        chan<- ServerFrame
	Evicted         chan<- SessionEviction
}

type SessionRegistrationResult struct {
	ResumeToken string
}

type SessionEviction struct {
	Reason        string
	Conversations []ConversationCursor
}

type ConnectSessionCommand struct {
	AuthContext       AuthContext
	ResumeToken       string
	LastReceived      []ConversationCursor
	QueueSize         int
	HeartbeatInterval time.Duration
}

type ConnectSessionResult struct {
	SessionID           string
	ResumeToken         string
	HeartbeatIntervalMS int64
}

type DeliveryNotification struct {
	Kind            string
	EventID         string
	TenantID        string
	UserID          string
	ConversationID  string
	ConversationSeq int64
	SourceEventID   string
	SourceEventType string
	MessageID       string
	CorrelationID   string
}

func (notification DeliveryNotification) Validate() error {
	kind := notification.Kind
	if kind == "" {
		kind = DeliveryNotificationKindInboxItemCreated
	}
	if kind != DeliveryNotificationKindInboxItemCreated &&
		kind != DeliveryNotificationKindInboxItemHidden &&
		kind != DeliveryNotificationKindConversationSignal {
		return NewInvalidFrame("delivery notification kind is unsupported")
	}
	if notification.TenantID == "" ||
		notification.ConversationID == "" ||
		notification.ConversationSeq <= 0 ||
		notification.EventID == "" {
		return NewInvalidFrame("delivery notification is incomplete")
	}
	if kind != DeliveryNotificationKindConversationSignal && notification.UserID == "" {
		return NewInvalidFrame("delivery notification is incomplete")
	}
	if (kind == DeliveryNotificationKindInboxItemCreated || kind == DeliveryNotificationKindConversationSignal) && notification.SourceEventType == "" {
		return NewInvalidFrame("delivery notification is incomplete")
	}
	return nil
}

type NotifyDeliveryCommand struct {
	Notification DeliveryNotification
}

type NotifyDeliveryResult struct {
	MatchedSessions int
	Enqueued        int
	Dropped         int
	Evicted         int
}

type ConversationSubscriptionCommand struct {
	AuthContext    AuthContext
	ConversationID string
}

type ConversationSubscriptionResult struct {
	ConversationID string
	Subscribed     bool
}

type SessionEvictionResult struct {
	MatchedSessions int
	Evicted         int
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
