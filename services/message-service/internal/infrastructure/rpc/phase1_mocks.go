package rpc

import (
	"context"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type StaticPolicy struct {
	Allowed           bool
	Reason            string
	PermissionVersion int64
	Classification    string
}

func NewStaticPolicy() StaticPolicy {
	return StaticPolicy{
		Allowed:           true,
		PermissionVersion: 0,
		Classification:    "INTERNAL",
	}
}

func (p StaticPolicy) CheckSendPermission(_ context.Context, _ types.SendMessageCommand, conversation types.ConversationSendContext) (types.PermissionDecision, error) {
	return types.PermissionDecision{
		Allowed:           p.Allowed,
		Reason:            p.Reason,
		PermissionVersion: staticPolicyPermissionVersion(p.PermissionVersion, conversation.PermissionVersion),
		Classification:    p.Classification,
	}, nil
}

func (p StaticPolicy) CheckEditPermission(_ context.Context, _ types.EditMessageCommand, conversation types.ConversationSendContext, _ types.MessagePolicyContext) (types.PermissionDecision, error) {
	return types.PermissionDecision{
		Allowed:           p.Allowed,
		Reason:            p.Reason,
		PermissionVersion: staticPolicyPermissionVersion(p.PermissionVersion, conversation.PermissionVersion),
		Classification:    p.Classification,
	}, nil
}

func (p StaticPolicy) CheckRevokePermission(_ context.Context, _ types.RevokeMessageCommand, conversation types.ConversationSendContext, _ types.MessagePolicyContext) (types.PermissionDecision, error) {
	return types.PermissionDecision{
		Allowed:           p.Allowed,
		Reason:            p.Reason,
		PermissionVersion: staticPolicyPermissionVersion(p.PermissionVersion, conversation.PermissionVersion),
		Classification:    p.Classification,
	}, nil
}

func (p StaticPolicy) CheckDeletePermission(_ context.Context, _ types.DeleteMessageCommand, conversation types.ConversationSendContext, _ types.MessagePolicyContext) (types.PermissionDecision, error) {
	return types.PermissionDecision{
		Allowed:           p.Allowed,
		Reason:            p.Reason,
		PermissionVersion: staticPolicyPermissionVersion(p.PermissionVersion, conversation.PermissionVersion),
		Classification:    p.Classification,
	}, nil
}

func staticPolicyPermissionVersion(policyVersion int64, conversationVersion int64) int64 {
	if policyVersion > 0 {
		return policyVersion
	}
	if conversationVersion > 0 {
		return conversationVersion
	}
	return 1
}

type StaticConversation struct {
	MemberVersion       int64
	PermissionVersion   int64
	ConversationMode    types.ConversationMode
	FanoutMode          types.FanoutMode
	FanoutPolicyVersion int64
	CurrentSeqShard     string
	DirectPeerUserID    types.UserID
}

func NewStaticConversation() StaticConversation {
	return StaticConversation{
		MemberVersion:       1,
		PermissionVersion:   1,
		ConversationMode:    types.ConversationModeLocalRowLock,
		FanoutMode:          types.FanoutModeWriteFanout,
		FanoutPolicyVersion: 1,
		CurrentSeqShard:     "local",
	}
}

func (c StaticConversation) GetSendContext(context.Context, types.SendMessageCommand) (types.ConversationSendContext, error) {
	return types.ConversationSendContext{
		MemberVersion:       c.MemberVersion,
		PermissionVersion:   c.PermissionVersion,
		ConversationMode:    c.ConversationMode,
		FanoutMode:          c.FanoutMode,
		FanoutPolicyVersion: c.FanoutPolicyVersion,
		CurrentSeqShard:     c.CurrentSeqShard,
		DirectPeerUserID:    c.DirectPeerUserID,
	}, nil
}

type NoopSequencer struct{}

func (NoopSequencer) AllocateSeqBlock(context.Context, types.SendMessageCommand) (types.SeqBlock, error) {
	return types.SeqBlock{}, types.NewSequencerUnavailable("sequencer is disabled in phase 1 LOCAL_ROW_LOCK mode")
}
