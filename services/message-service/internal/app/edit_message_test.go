package app

import (
	"context"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestEditMessageUseCase(t *testing.T) {
	repo := &fakeMessageRepository{
		editResult: domain.MessageChangeResult{
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
	useCase := NewEditMessageUseCase(policy, conversation, repo)

	result, err := useCase.Execute(context.Background(), testEditCommand())
	if err != nil {
		t.Fatalf("edit message: %v", err)
	}
	if result.MessageID != "msg-1" ||
		result.ConversationID != "conv-1" ||
		result.ConversationSeq != 2 ||
		result.ChangeVersion != 1 ||
		result.IdempotentReplay {
		t.Fatalf("unexpected result: %+v", result)
	}
	if conversation.calls != 1 || policy.calls != 1 || repo.editCalls != 1 {
		t.Fatalf("unexpected call counts conversation=%d policy=%d repo=%d", conversation.calls, policy.calls, repo.editCalls)
	}
	if repo.editInput.Command.MessageID != "msg-1" ||
		string(repo.editInput.Command.PayloadJSON) != `{"text":"updated"}` ||
		repo.editInput.Permission.PermissionVersion != 7 ||
		repo.editInput.Conversation.FanoutPolicyVersion != 3 {
		t.Fatalf("unexpected repository input: %+v", repo.editInput)
	}
}

func testEditCommand() types.EditMessageCommand {
	return types.EditMessageCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			RequestID: "request-1",
		},
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		IdempotencyKey: "edit-1",
		PayloadJSON:    []byte(`{"text":"updated"}`),
		Reason:         "typo",
		ReceivedAt:     time.Date(2026, 6, 10, 1, 0, 0, 0, time.UTC),
	}
}
