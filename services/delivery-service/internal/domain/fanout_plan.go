package domain

import "github.com/qsyy0921/IM/services/delivery-service/internal/types"

type FanoutProjectionStrategy string

const (
	FanoutProjectionWriteInbox      FanoutProjectionStrategy = "WRITE_INBOX"
	FanoutProjectionHybridSegments  FanoutProjectionStrategy = "HYBRID_SEGMENTS"
	FanoutProjectionTimelinePull    FanoutProjectionStrategy = "TIMELINE_PULL"
	FanoutProjectionBroadcastSignal FanoutProjectionStrategy = "BROADCAST_SIGNAL"
)

type FanoutPlan struct {
	Mode                  string
	Strategy              FanoutProjectionStrategy
	MaterializesUserInbox bool
	RequiresTimelineRead  bool
	RequiresFanoutShard   bool
}

func BuildFanoutPlan(mode string) (FanoutPlan, error) {
	switch mode {
	case types.DeliveryFanoutModeWriteFanout:
		return FanoutPlan{
			Mode:                  mode,
			Strategy:              FanoutProjectionWriteInbox,
			MaterializesUserInbox: true,
		}, nil
	case types.DeliveryFanoutModeHybridFanout:
		return FanoutPlan{
			Mode:                  mode,
			Strategy:              FanoutProjectionHybridSegments,
			MaterializesUserInbox: true,
			RequiresTimelineRead:  true,
			RequiresFanoutShard:   true,
		}, nil
	case types.DeliveryFanoutModeReadFanout:
		return FanoutPlan{
			Mode:                 mode,
			Strategy:             FanoutProjectionTimelinePull,
			RequiresTimelineRead: true,
		}, nil
	case types.DeliveryFanoutModeBroadcastSignal:
		return FanoutPlan{
			Mode:                 mode,
			Strategy:             FanoutProjectionBroadcastSignal,
			RequiresTimelineRead: true,
		}, nil
	case "":
		return FanoutPlan{}, types.NewInvalidArgument("fanout_mode is required")
	default:
		return FanoutPlan{}, types.NewInvalidArgument("unsupported fanout_mode")
	}
}

func EnsureTimelineProjectionSupported(command types.ProjectTimelineEventCommand) error {
	if !types.IsMessageTimelineEvent(command.EventType) {
		return nil
	}
	_, err := BuildFanoutPlan(command.FanoutMode)
	return err
}
