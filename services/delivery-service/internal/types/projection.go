package types

import "encoding/json"

const (
	TimelineEventMessagePersisted                    = "message.persisted.v1"
	TimelineEventMessageEdited                       = "message.edited.v1"
	TimelineEventMessageRevoked                      = "message.revoked.v1"
	TimelineEventMessageDeleted                      = "message.deleted.v1"
	TimelineEventConversationMemberJoined            = "conversation.member.joined.v1"
	TimelineEventConversationMemberLeft              = "conversation.member.left.v1"
	TimelineEventConversationMemberRemoved           = "conversation.member.removed.v1"
	TimelineEventConversationMemberRoleChanged       = "conversation.member.role_changed.v1"
	TimelineEventConversationMemberBoundaryCancelled = "conversation.member.boundary_cancelled.v1"
	TimelineEventConversationMemberOwnerTransferred  = "conversation.member.owner_transferred.v1"

	DeliveryMemberStatusActive = "ACTIVE"
	DeliveryMemberStatusLeft   = "LEFT"
	DeliveryMemberStatusBanned = "BANNED"
)

type ProjectTimelineEventCommand struct {
	TenantID             TenantID
	EventID              string
	EventType            string
	ConversationID       ConversationID
	ConversationSeq      int64
	FanoutMode           string
	PermissionVersion    int64
	MessageID            string
	SenderID             UserID
	PayloadJSON          json.RawMessage
	MemberUserID         UserID
	MemberRole           string
	MemberStatus         string
	MemberVersion        int64
	PreviousOwnerUserID  UserID
	PreviousOwnerNewRole string
	PreviousOwnerStatus  string
	NewOwnerUserID       UserID
	NewOwnerNewRole      string
	NewOwnerStatus       string
	ConsumerGroup        string
	Topic                string
	PartitionID          int32
	OffsetValue          int64
	TraceID              string
	CorrelationID        string
	CausationID          string
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
	case TimelineEventMessagePersisted, TimelineEventMessageEdited, TimelineEventMessageRevoked, TimelineEventMessageDeleted:
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
	case TimelineEventConversationMemberOwnerTransferred:
		if command.PreviousOwnerUserID == "" {
			return NewInvalidArgument("previous_owner_user_id is required")
		}
		if command.PreviousOwnerNewRole == "" {
			return NewInvalidArgument("previous_owner_new_role is required")
		}
		if command.PreviousOwnerStatus != DeliveryMemberStatusActive {
			return NewInvalidArgument("previous_owner_status must be ACTIVE")
		}
		if command.NewOwnerUserID == "" {
			return NewInvalidArgument("new_owner_user_id is required")
		}
		if command.NewOwnerNewRole == "" {
			return NewInvalidArgument("new_owner_new_role is required")
		}
		if command.NewOwnerStatus != DeliveryMemberStatusActive {
			return NewInvalidArgument("new_owner_status must be ACTIVE")
		}
		if command.MemberVersion <= 0 {
			return NewInvalidArgument("member_version must be positive")
		}
	default:
		return NewInvalidArgument("unsupported timeline event type")
	}
	if err := command.validateCheckpoint(); err != nil {
		return err
	}
	return nil
}

func (command ProjectTimelineEventCommand) ShouldCheckpoint() bool {
	return command.ConsumerGroup != "" && command.Topic != ""
}

func (command ProjectTimelineEventCommand) validateCheckpoint() error {
	hasCheckpointField := command.ConsumerGroup != "" || command.Topic != "" || command.PartitionID != 0 || command.OffsetValue != 0
	if !hasCheckpointField {
		return nil
	}
	if command.ConsumerGroup == "" {
		return NewInvalidArgument("consumer_group is required for checkpoint")
	}
	if command.Topic == "" {
		return NewInvalidArgument("topic is required for checkpoint")
	}
	if command.PartitionID < 0 {
		return NewInvalidArgument("partition_id must be non-negative")
	}
	if command.OffsetValue <= 0 {
		return NewInvalidArgument("offset_value must be next offset")
	}
	return nil
}

type ProjectTimelineEventResult struct {
	ProjectedInboxCount int
	MembershipUpdated   bool
}

type TimelineMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Value     []byte
}
