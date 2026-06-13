package types

const (
	ContactEventRequestCreated  = "contact.request.created.v1"
	ContactEventRequestAccepted = "contact.request.accepted.v1"
	ContactEventRequestDeclined = "contact.request.declined.v1"
	ContactEventRequestCanceled = "contact.request.canceled.v1"
	ContactEventEdgeDeleted     = "contact.edge.deleted.v1"
	ContactEventEdgeBlocked     = "contact.edge.blocked.v1"
	ContactEventEdgeUnblocked   = "contact.edge.unblocked.v1"
	ContactEventRemarkUpdated   = "contact.edge.remark_updated.v1"

	ContactEdgeStatusActive  = "ACTIVE"
	ContactEdgeStatusDeleted = "DELETED"
	ContactEdgeStatusBlocked = "BLOCKED"
)

type ContactMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Value     []byte
}

type ProjectContactEventCommand struct {
	TenantID       TenantID
	EventID        string
	EventType      string
	OwnerUserID    UserID
	ContactUserID  UserID
	SenderUserID   UserID
	ReceiverUserID UserID
	Status         string
	EdgeVersion    int64
	ConsumerGroup  string
	Topic          string
	PartitionID    int32
	OffsetValue    int64
	TraceID        string
	CorrelationID  string
	CausationID    string
}

func (command ProjectContactEventCommand) Validate() error {
	if command.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if command.EventID == "" {
		return NewInvalidArgument("event_id is required")
	}
	if command.EventType == "" {
		return NewInvalidArgument("event_type is required")
	}
	switch command.EventType {
	case ContactEventRequestCreated, ContactEventRequestDeclined, ContactEventRequestCanceled:
		if command.SenderUserID == "" || command.ReceiverUserID == "" {
			return NewInvalidArgument("contact request users are required")
		}
	case ContactEventRequestAccepted:
		if command.SenderUserID == "" || command.ReceiverUserID == "" {
			return NewInvalidArgument("contact accepted users are required")
		}
		if command.EdgeVersion <= 0 {
			return NewInvalidArgument("edge_version must be positive")
		}
	case ContactEventEdgeDeleted, ContactEventEdgeBlocked, ContactEventEdgeUnblocked, ContactEventRemarkUpdated:
		if command.OwnerUserID == "" || command.ContactUserID == "" {
			return NewInvalidArgument("contact edge users are required")
		}
		if command.EdgeVersion <= 0 {
			return NewInvalidArgument("edge_version must be positive")
		}
	default:
		return NewInvalidArgument("unsupported contact event type")
	}
	if command.Status != "" {
		switch command.Status {
		case ContactEdgeStatusActive, ContactEdgeStatusDeleted, ContactEdgeStatusBlocked, "PENDING", "ACCEPTED", "DECLINED", "CANCELED":
		default:
			return NewInvalidArgument("unsupported contact status")
		}
	}
	return command.validateCheckpoint()
}

func (command ProjectContactEventCommand) ShouldCheckpoint() bool {
	return command.ConsumerGroup != "" && command.Topic != ""
}

func (command ProjectContactEventCommand) validateCheckpoint() error {
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

type ProjectContactEventResult struct {
	ProjectedEdges int
}
