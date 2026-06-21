package rpc

import (
	"context"
	"testing"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestStaticPolicyUsesConversationPermissionVersionByDefault(t *testing.T) {
	policy := NewStaticPolicy()
	decision, err := policy.CheckSendPermission(
		context.Background(),
		types.SendMessageCommand{},
		types.ConversationSendContext{PermissionVersion: 12},
	)
	if err != nil {
		t.Fatalf("check send permission: %v", err)
	}
	if decision.PermissionVersion != 12 {
		t.Fatalf("permission version=%d, want 12", decision.PermissionVersion)
	}
}

func TestStaticPolicyAllowsExplicitPermissionVersionOverride(t *testing.T) {
	policy := NewStaticPolicy()
	policy.PermissionVersion = 7
	decision, err := policy.CheckSendPermission(
		context.Background(),
		types.SendMessageCommand{},
		types.ConversationSendContext{PermissionVersion: 12},
	)
	if err != nil {
		t.Fatalf("check send permission: %v", err)
	}
	if decision.PermissionVersion != 7 {
		t.Fatalf("permission version=%d, want 7", decision.PermissionVersion)
	}
}
