package domain

import "github.com/qsyy0921/IM/services/conversation-service/internal/types"

type ConversationScaleTier string
type ConversationScaleRuntime string

const (
	ConversationScaleTierDirect ConversationScaleTier = "DIRECT"
	ConversationScaleTierSmall  ConversationScaleTier = "SMALL_GROUP"
	ConversationScaleTierMedium ConversationScaleTier = "MEDIUM_GROUP"
	ConversationScaleTierLarge  ConversationScaleTier = "LARGE_GROUP"
	ConversationScaleTierHot    ConversationScaleTier = "HOT_GROUP"
)

const (
	ConversationScaleRuntimeActive       ConversationScaleRuntime = "ACTIVE"
	ConversationScaleRuntimeContractOnly ConversationScaleRuntime = "CONTRACT_ONLY"
)

const (
	// Direct and small groups use the current durable write-fanout path.
	SmallGroupMaxActiveMembers int64 = 500
	// Medium groups use hybrid fanout; large groups use timeline pull.
	MediumGroupMaxActiveMembers int64 = 5000
	// Hot groups remain contract-only until timeline-service sequencer is active.
	LargeGroupMaxActiveMembers int64 = 50000
)

type ConversationScalePolicy struct {
	Tier                ConversationScaleTier
	Runtime             ConversationScaleRuntime
	ConversationMode    types.ConversationMode
	FanoutMode          types.FanoutMode
	FanoutPolicyVersion int64
	CurrentSeqShard     string
}

func ResolveConversationScalePolicy(
	conversationType types.ConversationType,
	activeMemberCount int64,
) (ConversationScalePolicy, error) {
	if activeMemberCount <= 0 {
		return ConversationScalePolicy{}, types.NewInvalidArgument("active member count is invalid")
	}
	switch conversationType {
	case types.ConversationTypeDirect:
		if activeMemberCount > 2 {
			return ConversationScalePolicy{}, types.NewInvalidArgument("direct conversation member count is invalid")
		}
		return activeConversationPolicy(
			ConversationScaleTierDirect,
			types.ConversationModeLocalRowLock,
			types.FanoutModeWriteFanout,
			1,
			"local",
		), nil
	case types.ConversationTypeGroup:
		return resolveGroupScalePolicy(activeMemberCount), nil
	default:
		return ConversationScalePolicy{}, types.NewInvalidArgument("conversation_type is not supported")
	}
}

func ResolveConversationCreatePolicy(
	conversationType types.ConversationType,
	initialMemberCount int64,
) (ConversationScalePolicy, error) {
	policy, err := ResolveConversationScalePolicy(conversationType, initialMemberCount)
	if err != nil {
		return ConversationScalePolicy{}, err
	}
	if policy.Runtime != ConversationScaleRuntimeActive {
		return ConversationScalePolicy{}, types.NewSequencerUnavailable("conversation scale policy is not active")
	}
	return policy, nil
}

func resolveGroupScalePolicy(activeMemberCount int64) ConversationScalePolicy {
	switch {
	case activeMemberCount <= SmallGroupMaxActiveMembers:
		return activeConversationPolicy(
			ConversationScaleTierSmall,
			types.ConversationModeLocalRowLock,
			types.FanoutModeWriteFanout,
			1,
			"local",
		)
	case activeMemberCount <= MediumGroupMaxActiveMembers:
		return activeConversationPolicy(
			ConversationScaleTierMedium,
			types.ConversationModeLocalRowLock,
			types.FanoutModeHybridFanout,
			2,
			"hybrid",
		)
	case activeMemberCount <= LargeGroupMaxActiveMembers:
		return activeConversationPolicy(
			ConversationScaleTierLarge,
			types.ConversationModeLocalRowLock,
			types.FanoutModeReadFanout,
			3,
			"read",
		)
	default:
		return contractOnlyConversationPolicy(
			ConversationScaleTierHot,
			types.ConversationModeSequencerBlock,
			types.FanoutModeBroadcastSignal,
			4,
			"timeline",
		)
	}
}

func activeConversationPolicy(
	tier ConversationScaleTier,
	conversationMode types.ConversationMode,
	fanoutMode types.FanoutMode,
	fanoutPolicyVersion int64,
	currentSeqShard string,
) ConversationScalePolicy {
	return ConversationScalePolicy{
		Tier:                tier,
		Runtime:             ConversationScaleRuntimeActive,
		ConversationMode:    conversationMode,
		FanoutMode:          fanoutMode,
		FanoutPolicyVersion: fanoutPolicyVersion,
		CurrentSeqShard:     currentSeqShard,
	}
}

func contractOnlyConversationPolicy(
	tier ConversationScaleTier,
	conversationMode types.ConversationMode,
	fanoutMode types.FanoutMode,
	fanoutPolicyVersion int64,
	currentSeqShard string,
) ConversationScalePolicy {
	return ConversationScalePolicy{
		Tier:                tier,
		Runtime:             ConversationScaleRuntimeContractOnly,
		ConversationMode:    conversationMode,
		FanoutMode:          fanoutMode,
		FanoutPolicyVersion: fanoutPolicyVersion,
		CurrentSeqShard:     currentSeqShard,
	}
}
