package app

import (
	"context"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestRevokeMessageUseCase(t *testing.T) {
	repo := &fakeMessageRepository{
		revokeResult: domain.MessageChangeResult{
			MessageID:        "msg-1",
			ConversationSeq:  2,
			ChangeVersion:    1,
			AcceptedAt:       time.Date(2026, 6, 10, 1, 0, 0, 0, time.UTC),
			IdempotentReplay: false,
		},
	}
	policy := &fakePolicy{decision: types.PermissionDecision{
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "INTERNAL",
	}}
	conversation := &fakeConversation{context: types.ConversationSendContext{
		PermissionVersion:   7,
		ConversationMode:    types.ConversationModeLocalRowLock,
		FanoutMode:          types.FanoutModeWriteFanout,
		FanoutPolicyVersion: 3,
		CurrentSeqShard:     "local",
	}}
	useCase := NewRevokeMessageUseCase(policy, conversation, repo)

	result, err := useCase.Execute(context.Background(), testRevokeCommand())
	if err != nil {
		t.Fatalf("revoke message: %v", err)
	}
	if result.MessageID != "msg-1" ||
		result.ConversationID != "conv-1" ||
		result.ConversationSeq != 2 ||
		result.ChangeVersion != 1 ||
		result.IdempotentReplay {
		t.Fatalf("unexpected result: %+v", result)
	}
	if conversation.calls != 1 || policy.calls != 1 || repo.revokeCalls != 1 {
		t.Fatalf("unexpected call counts conversation=%d policy=%d repo=%d", conversation.calls, policy.calls, repo.revokeCalls)
	}
	if repo.revokeInput.Command.MessageID != "msg-1" ||
		repo.revokeInput.Permission.PermissionVersion != 7 ||
		repo.revokeInput.Conversation.FanoutPolicyVersion != 3 {
		t.Fatalf("unexpected repository input: %+v", repo.revokeInput)
	}
}

func testRevokeCommand() types.RevokeMessageCommand {
	return types.RevokeMessageCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			RequestID: "request-1",
		},
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		IdempotencyKey: "revoke-1",
		Reason:         "mistake",
		ReceivedAt:     time.Date(2026, 6, 10, 1, 0, 0, 0, time.UTC),
	}
}
