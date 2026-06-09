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
		PermissionVersion: 1,
		Classification:    "INTERNAL",
	}
}

func (p StaticPolicy) CheckSendPermission(context.Context, types.SendMessageCommand) (types.PermissionDecision, error) {
	return types.PermissionDecision{
		Allowed:           p.Allowed,
		Reason:            p.Reason,
		PermissionVersion: p.PermissionVersion,
		Classification:    p.Classification,
	}, nil
}

func (p StaticPolicy) CheckEditPermission(context.Context, types.EditMessageCommand) (types.PermissionDecision, error) {
	return types.PermissionDecision{
		Allowed:           p.Allowed,
		Reason:            p.Reason,
		PermissionVersion: p.PermissionVersion,
		Classification:    p.Classification,
	}, nil
}

func (p StaticPolicy) CheckRevokePermission(context.Context, types.RevokeMessageCommand) (types.PermissionDecision, error) {
	return types.PermissionDecision{
		Allowed:           p.Allowed,
		Reason:            p.Reason,
		PermissionVersion: p.PermissionVersion,
		Classification:    p.Classification,
	}, nil
}

type StaticConversation struct {
	MemberVersion       int64
	PermissionVersion   int64
	ConversationMode    types.ConversationMode
	FanoutMode          types.FanoutMode
	FanoutPolicyVersion int64
	CurrentSeqShard     string
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
	}, nil
}

type NoopSequencer struct{}

func (NoopSequencer) AllocateSeqBlock(context.Context, types.SendMessageCommand) (types.SeqBlock, error) {
	return types.SeqBlock{}, types.NewSequencerUnavailable("sequencer is disabled in phase 1 LOCAL_ROW_LOCK mode")
}
