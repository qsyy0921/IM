package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/policy-service/internal/domain"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestCheckMessageActionUseCaseAllowsStaticDecision(t *testing.T) {
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "CONTACT",
	})
	result, err := useCase.Execute(context.Background(), testPolicyCommand(types.MessageActionSend))
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !result.Allowed || result.PermissionVersion != 7 || result.Classification != "CONTACT" {
		t.Fatalf("unexpected decision: %+v", result)
	}
}

func TestCheckMessageActionUseCaseDeniesStaticDecision(t *testing.T) {
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{
		Allowed:           false,
		PermissionVersion: 3,
		Classification:    "BLOCKED",
		Reason:            "blocked by contact policy",
	})
	result, err := useCase.Execute(context.Background(), testPolicyCommand(types.MessageActionSend))
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if result.Allowed || result.Reason != "blocked by contact policy" {
		t.Fatalf("unexpected deny decision: %+v", result)
	}
}

func TestCheckMessageActionUseCaseValidatesCommand(t *testing.T) {
	useCase := NewCheckMessageActionUseCase(domain.StaticMessagePolicy{Allowed: true})
	_, err := useCase.Execute(context.Background(), types.CheckMessageActionCommand{})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func testPolicyCommand(action types.MessageAction) types.CheckMessageActionCommand {
	command := types.CheckMessageActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ConversationID: "conv-1",
		Action:         action,
	}
	if action != types.MessageActionSend {
		command.MessageID = "msg-1"
	}
	return command
}
