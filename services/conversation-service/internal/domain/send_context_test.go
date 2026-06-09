package domain

import (
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestBuildSendContext(t *testing.T) {
	result, err := BuildSendContext(activeConversation(), activeMember())
	if err != nil {
		t.Fatalf("build send context: %v", err)
	}
	if result.TenantID != "tenant-1" ||
		result.ConversationID != "conv-1" ||
		result.MemberVersion != 5 ||
		result.PermissionVersion != 7 ||
		result.ConversationMode != types.ConversationModeLocalRowLock ||
		result.FanoutMode != types.FanoutModeWriteFanout ||
		result.FanoutPolicyVersion != 3 ||
		result.CurrentSeqShard != "local" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestBuildSendContextRejectsInactiveConversation(t *testing.T) {
	for _, status := range []types.ConversationStatus{
		types.ConversationStatusArchived,
		types.ConversationStatusDeleted,
		"",
	} {
		t.Run(string(status), func(t *testing.T) {
			conversation := activeConversation()
			conversation.Status = status

			_, err := BuildSendContext(conversation, activeMember())
			if !errors.Is(err, types.ErrConversationNotFound) {
				t.Fatalf("expected conversation not found, got %v", err)
			}
		})
	}
}

func TestBuildSendContextRejectsInactiveMember(t *testing.T) {
	for _, status := range []types.MemberStatus{
		types.MemberStatusLeft,
		types.MemberStatusBanned,
		"",
	} {
		t.Run(string(status), func(t *testing.T) {
			member := activeMember()
			member.Status = status

			_, err := BuildSendContext(activeConversation(), member)
			if !errors.Is(err, types.ErrMemberNotActive) {
				t.Fatalf("expected member not active, got %v", err)
			}
		})
	}
}

func activeConversation() Conversation {
	return Conversation{
		TenantID:            "tenant-1",
		ConversationID:      "conv-1",
		Status:              types.ConversationStatusActive,
		ConversationMode:    types.ConversationModeLocalRowLock,
		FanoutMode:          types.FanoutModeWriteFanout,
		FanoutPolicyVersion: 3,
		MemberVersion:       5,
		PermissionVersion:   7,
		CurrentSeqShard:     "local",
	}
}

func activeMember() Member {
	return Member{
		UserID:            "user-1",
		Status:            types.MemberStatusActive,
		MemberVersion:     5,
		PermissionVersion: 7,
	}
}
