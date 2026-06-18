package types

type TimelineMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Value     []byte
}

type ProjectTimelineEventCommand struct {
	TenantID          TenantID
	EventID           string
	EventType         string
	ConversationID    ConversationID
	ConversationSeq   int64
	ConsumerGroup     string
	Topic             string
	PartitionID       int32
	OffsetValue       int64
	MessageID         string
	SenderID          UserID
	MessageType       string
	SearchableText    string
	TombstoneStatus   string
	TargetUserID      UserID
	MemberRole        string
	MemberStatus      string
	MemberVersion     int64
	PermissionVersion int64
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
