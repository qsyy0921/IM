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
	LargeGroupMaxActiveMembers int64 = 50000
)

type ConversationScaleThresholds struct {
	SmallGroupMaxActiveMembers  int64
	MediumGroupMaxActiveMembers int64
	LargeGroupMaxActiveMembers  int64
}

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
	return ResolveConversationScalePolicyWithThresholds(conversationType, activeMemberCount, DefaultConversationScaleThresholds())
}

func DefaultConversationScaleThresholds() ConversationScaleThresholds {
	return ConversationScaleThresholds{
		SmallGroupMaxActiveMembers:  SmallGroupMaxActiveMembers,
		MediumGroupMaxActiveMembers: MediumGroupMaxActiveMembers,
		LargeGroupMaxActiveMembers:  LargeGroupMaxActiveMembers,
	}
}

func ResolveConversationScalePolicyWithThresholds(
	conversationType types.ConversationType,
	activeMemberCount int64,
	thresholds ConversationScaleThresholds,
) (ConversationScalePolicy, error) {
	if activeMemberCount <= 0 {
		return ConversationScalePolicy{}, types.NewInvalidArgument("active member count is invalid")
	}
	if err := thresholds.Validate(); err != nil {
		return ConversationScalePolicy{}, err
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
		return resolveGroupScalePolicy(activeMemberCount, thresholds), nil
	default:
		return ConversationScalePolicy{}, types.NewInvalidArgument("conversation_type is not supported")
	}
}

func (thresholds ConversationScaleThresholds) Validate() error {
	if thresholds.SmallGroupMaxActiveMembers <= 0 ||
		thresholds.MediumGroupMaxActiveMembers <= thresholds.SmallGroupMaxActiveMembers ||
		thresholds.LargeGroupMaxActiveMembers <= thresholds.MediumGroupMaxActiveMembers {
		return types.NewInvalidArgument("conversation scale thresholds are invalid")
	}
	return nil
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

func resolveGroupScalePolicy(activeMemberCount int64, thresholds ConversationScaleThresholds) ConversationScalePolicy {
	switch {
	case activeMemberCount <= thresholds.SmallGroupMaxActiveMembers:
		return activeConversationPolicy(
			ConversationScaleTierSmall,
			types.ConversationModeLocalRowLock,
			types.FanoutModeWriteFanout,
			1,
			"local",
		)
	case activeMemberCount <= thresholds.MediumGroupMaxActiveMembers:
		return activeConversationPolicy(
			ConversationScaleTierMedium,
			types.ConversationModeLocalRowLock,
			types.FanoutModeHybridFanout,
			2,
			"hybrid",
		)
	case activeMemberCount <= thresholds.LargeGroupMaxActiveMembers:
		return activeConversationPolicy(
			ConversationScaleTierLarge,
			types.ConversationModeLocalRowLock,
			types.FanoutModeReadFanout,
			3,
			"read",
		)
	default:
		return activeConversationPolicy(
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
