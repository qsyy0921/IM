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

	ConversationMemberRoleOwner  = "OWNER"
	ConversationMemberRoleAdmin  = "ADMIN"
	ConversationMemberRoleMember = "MEMBER"

	ConversationMemberStatusActive = "ACTIVE"
	ConversationMemberStatusLeft   = "LEFT"
	ConversationMemberStatusBanned = "BANNED"
)

type KafkaMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Value     []byte
}

type ContactMessage = KafkaMessage
type TimelineMessage = KafkaMessage

type ProjectConversationMemberEventCommand struct {
	TenantID             TenantID
	EventID              string
	EventType            string
	ConversationID       ConversationID
	ConversationSeq      int64
	MemberUserID         UserID
	MemberRole           string
	MemberStatus         string
	MemberVersion        int64
	PermissionVersion    int64
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

func (command ProjectConversationMemberEventCommand) Validate() error {
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
	case TimelineEventConversationMemberBoundaryCancelled:
		if command.MemberUserID == "" {
			return NewInvalidArgument("member_user_id is required")
		}
		if command.MemberVersion <= 0 {
			return NewInvalidArgument("member_version must be positive")
		}
	case TimelineEventConversationMemberJoined,
		TimelineEventConversationMemberLeft,
		TimelineEventConversationMemberRemoved,
		TimelineEventConversationMemberRoleChanged:
		if command.MemberUserID == "" {
			return NewInvalidArgument("member_user_id is required")
		}
		if !isConversationMemberRole(command.MemberRole) {
			return NewInvalidArgument("member_role is required")
		}
		if !isConversationMemberStatus(command.MemberStatus) {
			return NewInvalidArgument("member_status is required")
		}
		if command.MemberVersion <= 0 {
			return NewInvalidArgument("member_version must be positive")
		}
		if command.PermissionVersion <= 0 {
			return NewInvalidArgument("permission_version must be positive")
		}
	case TimelineEventConversationMemberOwnerTransferred:
		if command.PreviousOwnerUserID == "" || command.NewOwnerUserID == "" {
			return NewInvalidArgument("owner transfer users are required")
		}
		if !isConversationMemberRole(command.PreviousOwnerNewRole) || !isConversationMemberRole(command.NewOwnerNewRole) {
			return NewInvalidArgument("owner transfer roles are required")
		}
		if command.PreviousOwnerStatus != ConversationMemberStatusActive || command.NewOwnerStatus != ConversationMemberStatusActive {
			return NewInvalidArgument("owner transfer statuses must be ACTIVE")
		}
		if command.MemberVersion <= 0 {
			return NewInvalidArgument("member_version must be positive")
		}
		if command.PermissionVersion <= 0 {
			return NewInvalidArgument("permission_version must be positive")
		}
	default:
		return NewInvalidArgument("unsupported timeline event type")
	}
	return command.validateCheckpoint()
}

func (command ProjectConversationMemberEventCommand) ShouldCheckpoint() bool {
	return command.ConsumerGroup != "" && command.Topic != ""
}

func (command ProjectConversationMemberEventCommand) IgnoredMessageEvent() bool {
	switch command.EventType {
	case TimelineEventMessagePersisted, TimelineEventMessageEdited, TimelineEventMessageRevoked, TimelineEventMessageDeleted:
		return true
	default:
		return false
	}
}

func (command ProjectConversationMemberEventCommand) validateCheckpoint() error {
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

type ProjectConversationMemberEventResult struct {
	ProjectedMembers int
	Ignored          bool
}

func isConversationMemberRole(role string) bool {
	switch role {
	case ConversationMemberRoleOwner, ConversationMemberRoleAdmin, ConversationMemberRoleMember:
		return true
	default:
		return false
	}
}

func isConversationMemberStatus(status string) bool {
	switch status {
	case ConversationMemberStatusActive, ConversationMemberStatusLeft, ConversationMemberStatusBanned:
		return true
	default:
		return false
	}
}
