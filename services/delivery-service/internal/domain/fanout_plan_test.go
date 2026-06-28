package domain

import (
	"testing"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestBuildFanoutPlan(t *testing.T) {
	tests := []struct {
		name                  string
		mode                  string
		strategy              FanoutProjectionStrategy
		materializesUserInbox bool
		requiresTimelineRead  bool
		requiresFanoutShard   bool
	}{
		{
			name:                  "write fanout",
			mode:                  types.DeliveryFanoutModeWriteFanout,
			strategy:              FanoutProjectionWriteInbox,
			materializesUserInbox: true,
		},
		{
			name:                  "hybrid fanout",
			mode:                  types.DeliveryFanoutModeHybridFanout,
			strategy:              FanoutProjectionHybridSegments,
			materializesUserInbox: true,
			requiresTimelineRead:  true,
			requiresFanoutShard:   true,
		},
		{
			name:                 "read fanout",
			mode:                 types.DeliveryFanoutModeReadFanout,
			strategy:             FanoutProjectionTimelinePull,
			requiresTimelineRead: true,
		},
		{
			name:                 "broadcast signal",
			mode:                 types.DeliveryFanoutModeBroadcastSignal,
			strategy:             FanoutProjectionBroadcastSignal,
			requiresTimelineRead: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildFanoutPlan(tc.mode)
			if err != nil {
				t.Fatalf("build fanout plan: %v", err)
			}
			if plan.Mode != tc.mode ||
				plan.Strategy != tc.strategy ||
				plan.MaterializesUserInbox != tc.materializesUserInbox ||
				plan.RequiresTimelineRead != tc.requiresTimelineRead ||
				plan.RequiresFanoutShard != tc.requiresFanoutShard {
				t.Fatalf("unexpected plan: %+v", plan)
			}
		})
	}
}

func TestEnsureTimelineProjectionSupported(t *testing.T) {
	command := types.ProjectTimelineEventCommand{
		TenantID:        "tenant-1",
		EventID:         "event-1",
		EventType:       types.TimelineEventMessagePersisted,
		ConversationID:  "conv-1",
		ConversationSeq: 1,
		MessageID:       "msg-1",
	}

	command.FanoutMode = types.DeliveryFanoutModeWriteFanout
	if err := EnsureTimelineProjectionSupported(command); err != nil {
		t.Fatalf("write fanout should be supported: %v", err)
	}

	for _, mode := range []string{
		types.DeliveryFanoutModeHybridFanout,
		types.DeliveryFanoutModeReadFanout,
		types.DeliveryFanoutModeBroadcastSignal,
	} {
		command.FanoutMode = mode
		if err := EnsureTimelineProjectionSupported(command); err != nil {
			t.Fatalf("%s should be supported: %v", mode, err)
		}
	}

	command.EventType = types.TimelineEventConversationMemberJoined
	if err := EnsureTimelineProjectionSupported(command); err != nil {
		t.Fatalf("membership events should not require delivery fanout mode: %v", err)
	}
}
