package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestResolveConversationScalePolicy(t *testing.T) {
	tests := []struct {
		name             string
		conversationType types.ConversationType
		members          int64
		wantTier         ConversationScaleTier
		wantRuntime      ConversationScaleRuntime
		wantMode         types.ConversationMode
		wantFanout       types.FanoutMode
		wantVersion      int64
		wantShard        string
	}{
		{
			name:             "direct conversation stays write fanout",
			conversationType: types.ConversationTypeDirect,
			members:          2,
			wantTier:         ConversationScaleTierDirect,
			wantRuntime:      ConversationScaleRuntimeActive,
			wantMode:         types.ConversationModeLocalRowLock,
			wantFanout:       types.FanoutModeWriteFanout,
			wantVersion:      1,
			wantShard:        "local",
		},
		{
			name:             "small group uses write fanout",
			conversationType: types.ConversationTypeGroup,
			members:          SmallGroupMaxActiveMembers,
			wantTier:         ConversationScaleTierSmall,
			wantRuntime:      ConversationScaleRuntimeActive,
			wantMode:         types.ConversationModeLocalRowLock,
			wantFanout:       types.FanoutModeWriteFanout,
			wantVersion:      1,
			wantShard:        "local",
		},
		{
			name:             "medium group uses hybrid fanout",
			conversationType: types.ConversationTypeGroup,
			members:          SmallGroupMaxActiveMembers + 1,
			wantTier:         ConversationScaleTierMedium,
			wantRuntime:      ConversationScaleRuntimeActive,
			wantMode:         types.ConversationModeLocalRowLock,
			wantFanout:       types.FanoutModeHybridFanout,
			wantVersion:      2,
			wantShard:        "hybrid",
		},
		{
			name:             "large group uses sequencer read fanout",
			conversationType: types.ConversationTypeGroup,
			members:          MediumGroupMaxActiveMembers + 1,
			wantTier:         ConversationScaleTierLarge,
			wantRuntime:      ConversationScaleRuntimeActive,
			wantMode:         types.ConversationModeSequencerBlock,
			wantFanout:       types.FanoutModeReadFanout,
			wantVersion:      FanoutPolicyVersionReadFanout,
			wantShard:        "timeline",
		},
		{
			name:             "hot group uses broadcast signal",
			conversationType: types.ConversationTypeGroup,
			members:          LargeGroupMaxActiveMembers + 1,
			wantTier:         ConversationScaleTierHot,
			wantRuntime:      ConversationScaleRuntimeActive,
			wantMode:         types.ConversationModeSequencerBlock,
			wantFanout:       types.FanoutModeBroadcastSignal,
			wantVersion:      FanoutPolicyVersionBroadcastSignal,
			wantShard:        "timeline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveConversationScalePolicy(tt.conversationType, tt.members)
			if err != nil {
				t.Fatalf("resolve policy: %v", err)
			}
			if got.Tier != tt.wantTier ||
				got.Runtime != tt.wantRuntime ||
				got.ConversationMode != tt.wantMode ||
				got.FanoutMode != tt.wantFanout ||
				got.FanoutPolicyVersion != tt.wantVersion ||
				got.CurrentSeqShard != tt.wantShard {
				t.Fatalf("policy=%+v", got)
			}
		})
	}
}

func TestResolveConversationScalePolicyWithThresholds(t *testing.T) {
	policy, err := ResolveConversationScalePolicyWithThresholds(types.ConversationTypeGroup, 13, ConversationScaleThresholds{
		SmallGroupMaxActiveMembers:  3,
		MediumGroupMaxActiveMembers: 7,
		LargeGroupMaxActiveMembers:  12,
	})
	if err != nil {
		t.Fatalf("resolve policy: %v", err)
	}
	if policy.Tier != ConversationScaleTierHot ||
		policy.Runtime != ConversationScaleRuntimeActive ||
		policy.ConversationMode != types.ConversationModeSequencerBlock ||
		policy.FanoutMode != types.FanoutModeBroadcastSignal ||
		policy.FanoutPolicyVersion != FanoutPolicyVersionBroadcastSignal ||
		policy.CurrentSeqShard != "timeline" {
		t.Fatalf("policy=%+v", policy)
	}
}

func TestResolveConversationScalePolicyRejectsInvalidThresholds(t *testing.T) {
	_, err := ResolveConversationScalePolicyWithThresholds(types.ConversationTypeGroup, 10, ConversationScaleThresholds{
		SmallGroupMaxActiveMembers:  10,
		MediumGroupMaxActiveMembers: 10,
		LargeGroupMaxActiveMembers:  20,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
}

func TestNewConversationCreateRecordUsesScalePolicy(t *testing.T) {
	record, err := NewConversationCreateRecord(types.CreateConversationCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-scale",
			UserID:   "user-a",
		},
		ConversationID:   "group-scale",
		ConversationType: types.ConversationTypeGroup,
		IdempotencyKey:   "create-scale",
	}, []types.EventID{"event-scale"}, 1, time.Date(2026, 6, 28, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatalf("create record: %v", err)
	}
	if record.Conversation.FanoutMode != types.FanoutModeWriteFanout ||
		record.Conversation.ConversationMode != types.ConversationModeLocalRowLock ||
		record.Conversation.FanoutPolicyVersion != 1 ||
		record.Conversation.CurrentSeqShard != "local" {
		t.Fatalf("conversation policy=%+v", record.Conversation)
	}
}
