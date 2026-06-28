package types

const (
	DeliveryFanoutModeWriteFanout     = "WRITE_FANOUT"
	DeliveryFanoutModeHybridFanout    = "HYBRID_FANOUT"
	DeliveryFanoutModeReadFanout      = "READ_FANOUT"
	DeliveryFanoutModeBroadcastSignal = "BROADCAST_SIGNAL"
)

func IsMessageTimelineEvent(eventType string) bool {
	switch eventType {
	case TimelineEventMessagePersisted,
		TimelineEventMessageEdited,
		TimelineEventMessageRevoked,
		TimelineEventMessageDeleted:
		return true
	default:
		return false
	}
}

func validateDeliveryFanoutMode(mode string) error {
	switch mode {
	case DeliveryFanoutModeWriteFanout,
		DeliveryFanoutModeHybridFanout,
		DeliveryFanoutModeReadFanout,
		DeliveryFanoutModeBroadcastSignal:
		return nil
	case "":
		return NewInvalidArgument("fanout_mode is required for message timeline event")
	default:
		return NewInvalidArgument("unsupported fanout_mode")
	}
}
