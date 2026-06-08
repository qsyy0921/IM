package domain

import (
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type TimelineEvent struct {
	EventID             types.EventID
	EventType           types.TimelineEventType
	EventVersion        string
	TenantID            types.TenantID
	ConversationID      types.ConversationID
	ConversationSeq     int64
	MessageID           types.MessageID
	FanoutMode          types.FanoutMode
	FanoutPolicyVersion int64
	PermissionVersion   int64
	TraceID             string
	PayloadJSON         []byte
	CreatedAt           time.Time
}

type OutboxEvent struct {
	EventID          types.EventID
	TenantID         types.TenantID
	ConversationID   types.ConversationID
	AggregateVersion int64
	EventType        types.TimelineEventType
	EventVersion     string
	PartitionKey     string
	PayloadJSON       []byte
	TraceID           string
}
