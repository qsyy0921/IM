package domain

import "github.com/qsyy0921/IM/services/conversation-service/internal/types"

type Conversation struct {
	TenantID            types.TenantID
	ConversationID      types.ConversationID
	ConversationType    types.ConversationType
	Status              types.ConversationStatus
	ConversationMode    types.ConversationMode
	FanoutMode          types.FanoutMode
	FanoutPolicyVersion int64
	MemberVersion       int64
	PermissionVersion   int64
	CurrentSeqShard     string
	DirectPeerUserID    types.UserID
}

type Member struct {
	UserID            types.UserID
	Role              types.MemberRole
	Status            types.MemberStatus
	MemberVersion     int64
	PermissionVersion int64
}

func BuildSendContext(conversation Conversation, member Member) (types.ConversationSendContext, error) {
	if conversation.Status != types.ConversationStatusActive {
		return types.ConversationSendContext{}, types.NewConversationNotFound("conversation is not active")
	}
	if member.Status != types.MemberStatusActive {
		return types.ConversationSendContext{}, types.NewMemberNotActive("member is not active")
	}
	if conversation.ConversationType == types.ConversationTypeDirect && conversation.DirectPeerUserID == "" {
		return types.ConversationSendContext{}, types.NewMemberNotActive("direct peer is not active")
	}
	return types.ConversationSendContext{
		TenantID:            conversation.TenantID,
		ConversationID:      conversation.ConversationID,
		MemberVersion:       conversation.MemberVersion,
		PermissionVersion:   conversation.PermissionVersion,
		ConversationMode:    conversation.ConversationMode,
		FanoutMode:          conversation.FanoutMode,
		FanoutPolicyVersion: conversation.FanoutPolicyVersion,
		CurrentSeqShard:     conversation.CurrentSeqShard,
		DirectPeerUserID:    conversation.directPeerUserID(),
	}, nil
}

func (conversation Conversation) directPeerUserID() types.UserID {
	if conversation.ConversationType != types.ConversationTypeDirect {
		return ""
	}
	return conversation.DirectPeerUserID
}
