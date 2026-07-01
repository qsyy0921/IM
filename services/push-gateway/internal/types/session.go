package types

import (
	"sort"
	"time"
)

const (
	DefaultHeartbeatInterval = 30 * time.Second
	DefaultSessionQueueSize  = 256
	DefaultWriterBatchSize   = 16
	DefaultResumeBufferSize  = 256
	DefaultResumeBufferTTL   = 10 * time.Minute
)

const (
	DeliveryNotificationKindInboxItemCreated   = "inbox_item_created"
	DeliveryNotificationKindInboxItemHidden    = "inbox_item_hidden"
	DeliveryNotificationKindConversationSignal = "conversation_signal"
)

const (
	FanoutModeWriteFanout     = "WRITE_FANOUT"
	FanoutModeHybridFanout    = "HYBRID_FANOUT"
	FanoutModeReadFanout      = "READ_FANOUT"
	FanoutModeBroadcastSignal = "BROADCAST_SIGNAL"
)

type ConversationSignalPolicy struct {
	DefaultSampleEvery               int
	ByFanoutMode                     map[string]int
	SubscriberThresholdsByFanoutMode map[string][]ConversationSignalSubscriberThreshold
}

type ConversationSignalSubscriberThreshold struct {
	MinSubscribers int
	SampleEvery    int
}

func NormalizeConversationSignalPolicy(policy ConversationSignalPolicy) ConversationSignalPolicy {
	if policy.DefaultSampleEvery <= 0 {
		policy.DefaultSampleEvery = 1
	}
	if len(policy.ByFanoutMode) > 0 {
		normalized := make(map[string]int, len(policy.ByFanoutMode))
		for mode, sampleEvery := range policy.ByFanoutMode {
			if mode == "" || sampleEvery <= 0 {
				continue
			}
			normalized[mode] = sampleEvery
		}
		policy.ByFanoutMode = normalized
	}
	if len(policy.SubscriberThresholdsByFanoutMode) == 0 {
		return policy
	}
	normalizedThresholds := make(map[string][]ConversationSignalSubscriberThreshold, len(policy.SubscriberThresholdsByFanoutMode))
	for mode, thresholds := range policy.SubscriberThresholdsByFanoutMode {
		if mode == "" {
			continue
		}
		modeThresholds := make([]ConversationSignalSubscriberThreshold, 0, len(thresholds))
		for _, threshold := range thresholds {
			if threshold.MinSubscribers <= 0 || threshold.SampleEvery <= 0 {
				continue
			}
			modeThresholds = append(modeThresholds, threshold)
		}
		if len(modeThresholds) == 0 {
			continue
		}
		sort.Slice(modeThresholds, func(left int, right int) bool {
			return modeThresholds[left].MinSubscribers < modeThresholds[right].MinSubscribers
		})
		normalizedThresholds[mode] = modeThresholds
	}
	policy.SubscriberThresholdsByFanoutMode = normalizedThresholds
	return policy
}

func (policy ConversationSignalPolicy) SampleEveryFor(fanoutMode string) int {
	if policy.DefaultSampleEvery <= 0 {
		policy.DefaultSampleEvery = 1
	}
	if policy.ByFanoutMode != nil {
		if sampleEvery := policy.ByFanoutMode[fanoutMode]; sampleEvery > 0 {
			return sampleEvery
		}
	}
	return policy.DefaultSampleEvery
}

func (policy ConversationSignalPolicy) SampleEveryForSubscribers(fanoutMode string, subscriberCount int) int {
	sampleEvery := policy.SampleEveryFor(fanoutMode)
	if subscriberCount <= 0 || len(policy.SubscriberThresholdsByFanoutMode) == 0 {
		return sampleEvery
	}
	for _, threshold := range policy.SubscriberThresholdsByFanoutMode[fanoutMode] {
		if subscriberCount >= threshold.MinSubscribers && threshold.SampleEvery > sampleEvery {
			sampleEvery = threshold.SampleEvery
		}
	}
	return sampleEvery
}

func (policy ConversationSignalPolicy) RequiresSubscriberCount(fanoutMode string) bool {
	return len(policy.SubscriberThresholdsByFanoutMode[fanoutMode]) > 0
}

func (policy ConversationSignalPolicy) ShouldEmit(conversationSeq int64, fanoutMode string) bool {
	sampleEvery := policy.SampleEveryFor(fanoutMode)
	return shouldEmitConversationSignal(conversationSeq, sampleEvery)
}

func (policy ConversationSignalPolicy) ShouldEmitForSubscribers(conversationSeq int64, fanoutMode string, subscriberCount int) bool {
	sampleEvery := policy.SampleEveryForSubscribers(fanoutMode, subscriberCount)
	return shouldEmitConversationSignal(conversationSeq, sampleEvery)
}

func shouldEmitConversationSignal(conversationSeq int64, sampleEvery int) bool {
	if sampleEvery <= 1 {
		return true
	}
	return conversationSeq%int64(sampleEvery) == 0
}

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
	FanoutMode      string
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
	if kind == DeliveryNotificationKindConversationSignal && notification.FanoutMode == "" {
		return NewInvalidFrame("conversation signal fanout mode is required")
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
