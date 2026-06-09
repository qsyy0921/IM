package types

const (
	DeliveryEventInboxItemCreated = "delivery.inbox_item.created.v1"
	DeliveryEventAckRecorded      = "delivery.ack.recorded.v1"
)

type ProjectDeliveryEventCommand struct {
	TenantID        TenantID
	EventID         string
	EventType       string
	UserID          UserID
	DeviceID        string
	ConversationID  ConversationID
	ConversationSeq int64
	SourceEventID   string
	MessageID       string
	SenderID        UserID
	LastReceivedSeq int64
	ConsumerGroup   string
	Topic           string
	PartitionID     int32
	OffsetValue     int64
	TraceID         string
	CorrelationID   string
	CausationID     string
}

func (command ProjectDeliveryEventCommand) Validate() error {
	if command.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if command.EventID == "" {
		return NewInvalidArgument("event_id is required")
	}
	if command.EventType == "" {
		return NewInvalidArgument("event_type is required")
	}
	if command.UserID == "" {
		return NewInvalidArgument("user_id is required")
	}
	if command.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	switch command.EventType {
	case DeliveryEventInboxItemCreated:
		if command.ConversationSeq <= 0 {
			return NewInvalidArgument("conversation_seq must be positive")
		}
		if command.SourceEventID == "" {
			return NewInvalidArgument("source_event_id is required")
		}
		if command.MessageID == "" {
			return NewInvalidArgument("message_id is required")
		}
		if command.SenderID == "" {
			return NewInvalidArgument("sender_id is required")
		}
	case DeliveryEventAckRecorded:
		if command.DeviceID == "" {
			return NewInvalidArgument("device_id is required")
		}
		if command.LastReceivedSeq <= 0 {
			return NewInvalidArgument("last_received_seq must be positive")
		}
	default:
		return NewInvalidArgument("unsupported delivery event type")
	}
	return command.validateCheckpoint()
}

func (command ProjectDeliveryEventCommand) ShouldCheckpoint() bool {
	return command.ConsumerGroup != "" && command.Topic != ""
}

func (command ProjectDeliveryEventCommand) validateCheckpoint() error {
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

type ProjectDeliveryEventResult struct {
	ProjectedInboxItem bool
	AdvancedReceived   bool
}
