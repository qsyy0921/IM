package types

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

	SearchTombstoneNone               = "NONE"
	SearchTombstoneRevoked            = "REVOKED"
	SearchTombstoneDeleted            = "DELETED"
	SearchTombstoneComplianceRedacted = "COMPLIANCE_REDACTED"

	SearchMemberStatusActive  = "ACTIVE"
	SearchMemberStatusLeft    = "LEFT"
	SearchMemberStatusRemoved = "REMOVED"
	SearchMemberStatusBanned  = "BANNED"

	SearchMemberRoleOwner  = "OWNER"
	SearchMemberRoleAdmin  = "ADMIN"
	SearchMemberRoleMember = "MEMBER"
)

type TimelineMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Value     []byte
}

type ProjectTimelineEventCommand struct {
	TenantID            TenantID
	EventID             string
	EventType           string
	ConversationID      ConversationID
	ConversationSeq     int64
	ConsumerGroup       string
	Topic               string
	PartitionID         int32
	OffsetValue         int64
	MessageID           string
	SenderID            UserID
	MessageType         string
	SearchableText      string
	TombstoneStatus     string
	TargetUserID        UserID
	MemberRole          string
	MemberStatus        string
	MemberVersion       int64
	PermissionVersion   int64
	PreviousOwnerUserID UserID
	PreviousOwnerRole   string
	PreviousOwnerStatus string
	NewOwnerUserID      UserID
	NewOwnerRole        string
	NewOwnerStatus      string
	TraceID             string
	CorrelationID       string
	CausationID         string
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
	case TimelineEventMessagePersisted, TimelineEventMessageEdited:
		if command.MessageID == "" {
			return NewInvalidArgument("message_id is required")
		}
	case TimelineEventMessageRevoked, TimelineEventMessageDeleted:
		if command.MessageID == "" {
			return NewInvalidArgument("message_id is required")
		}
	case TimelineEventConversationMemberJoined,
		TimelineEventConversationMemberLeft,
		TimelineEventConversationMemberRemoved,
		TimelineEventConversationMemberRoleChanged,
		TimelineEventConversationMemberBoundaryCancelled:
		if command.TargetUserID == "" {
			return NewInvalidArgument("target_user_id is required")
		}
		if command.MemberVersion <= 0 {
			return NewInvalidArgument("member_version must be positive")
		}
	case TimelineEventConversationMemberOwnerTransferred:
		if command.PreviousOwnerUserID == "" {
			return NewInvalidArgument("previous_owner_user_id is required")
		}
		if command.PreviousOwnerRole == "" {
			return NewInvalidArgument("previous_owner_role is required")
		}
		if command.NewOwnerUserID == "" {
			return NewInvalidArgument("new_owner_user_id is required")
		}
		if command.NewOwnerRole == "" {
			return NewInvalidArgument("new_owner_role is required")
		}
		if command.MemberVersion <= 0 {
			return NewInvalidArgument("member_version must be positive")
		}
	default:
		return NewUnsupportedPayload("unsupported timeline event type")
	}
	if command.ConsumerGroup == "" {
		return NewInvalidArgument("consumer_group is required")
	}
	if command.Topic == "" {
		return NewInvalidArgument("topic is required")
	}
	if command.PartitionID < 0 {
		return NewInvalidArgument("partition_id must be non-negative")
	}
	if command.OffsetValue < 0 {
		return NewInvalidArgument("offset_value must be non-negative")
	}
	return nil
}

type ProjectTimelineEventResult struct {
	TenantID          TenantID
	EventID           string
	ConversationID    ConversationID
	ConversationSeq   int64
	ProjectedDocument bool
	ProjectedMember   bool
}
