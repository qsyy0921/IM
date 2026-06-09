package types

import "encoding/json"

const (
	TimelineEventMessagePersisted                    = "message.persisted.v1"
	TimelineEventConversationMemberJoined            = "conversation.member.joined.v1"
	TimelineEventConversationMemberLeft              = "conversation.member.left.v1"
	TimelineEventConversationMemberRemoved           = "conversation.member.removed.v1"
	TimelineEventConversationMemberRoleChanged       = "conversation.member.role_changed.v1"
	TimelineEventConversationMemberBoundaryCancelled = "conversation.member.boundary_cancelled.v1"

	DeliveryMemberStatusActive = "ACTIVE"
	DeliveryMemberStatusLeft   = "LEFT"
	DeliveryMemberStatusBanned = "BANNED"
)

type ProjectTimelineEventCommand struct {
	TenantID          TenantID
	EventID           string
	EventType         string
	ConversationID    ConversationID
	ConversationSeq   int64
	FanoutMode        string
	PermissionVersion int64
	MessageID         string
	SenderID          UserID
	PayloadJSON       json.RawMessage
	MemberUserID      UserID
	MemberRole        string
	MemberStatus      string
	MemberVersion     int64
	ConsumerGroup     string
	Topic             string
	PartitionID       int32
	OffsetValue       int64
	TraceID           string
	CorrelationID     string
	CausationID       string
}

func (command ProjectTimelineEventCommand) Validate() error {
	if command.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if command.EventID == "" {
		return NewInvalidArgument("event_id is required")
	}
	if command.EventType == "" {
		return NewInvalidArgument("event_type is required")
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if command.ConversationSeq <= 0 {
		return NewInvalidArgument("conversation_seq must be positive")
	}
	switch command.EventType {
	case TimelineEventMessagePersisted:
		if command.MessageID == "" {
			return NewInvalidArgument("message_id is required")
		}
	case TimelineEventConversationMemberJoined,
		TimelineEventConversationMemberLeft,
		TimelineEventConversationMemberRemoved,
		TimelineEventConversationMemberRoleChanged,
		TimelineEventConversationMemberBoundaryCancelled:
		if command.MemberUserID == "" {
			return NewInvalidArgument("member_user_id is required")
		}
		if command.MemberVersion <= 0 {
			return NewInvalidArgument("member_version must be positive")
		}
	default:
		return NewInvalidArgument("unsupported timeline event type")
	}
	return nil
}

func (command ProjectTimelineEventCommand) ShouldCheckpoint() bool {
	return command.ConsumerGroup != "" && command.Topic != ""
}

type ProjectTimelineEventResult struct {
	ProjectedInboxCount int
	MembershipUpdated   bool
}
