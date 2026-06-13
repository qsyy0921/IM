package grpc

import (
	"context"
	"testing"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/policy-service/internal/app"
	"github.com/qsyy0921/IM/services/policy-service/internal/domain"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServerCheckMessageAction(t *testing.T) {
	server := NewServer(app.NewCheckMessageActionUseCase(domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 9,
		Classification:    "INTERNAL",
	}))
	response, err := server.CheckMessageAction(context.Background(), &policyv1.CheckMessageActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		ConversationId: "conv-1",
		Action:         policyv1.MessageAction_MESSAGE_ACTION_SEND,
	})
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !response.GetAllowed() || response.GetPermissionVersion() != 9 || response.GetAction() != policyv1.MessageAction_MESSAGE_ACTION_SEND {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestServerCheckMessageActionInvalidRequest(t *testing.T) {
	server := NewServer(app.NewCheckMessageActionUseCase(domain.StaticMessagePolicy{Allowed: true}))
	_, err := server.CheckMessageAction(context.Background(), &policyv1.CheckMessageActionRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestServerCheckMessageActionDBWriteFailureIsUnavailable(t *testing.T) {
	server := NewServer(app.NewCheckMessageActionUseCase(domain.StaticMessagePolicy{
		Allowed:           true,
		PermissionVersion: 9,
		Classification:    "INTERNAL",
	}, app.WithPolicyDecisionAuditor(failingAuditor{err: types.NewDBWriteFailed("audit write failed")})))
	_, err := server.CheckMessageAction(context.Background(), &policyv1.CheckMessageActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		ConversationId: "conv-1",
		Action:         policyv1.MessageAction_MESSAGE_ACTION_SEND,
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable, got %v", err)
	}
}

type failingAuditor struct {
	err error
}

func (f failingAuditor) RecordPolicyDecision(context.Context, types.CheckMessageActionCommand, types.MessageActionDecision) error {
	return f.err
}
