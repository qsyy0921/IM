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
	executor := &capturingCheckMessageActionExecutor{
		decision: types.MessageActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-1",
			ConversationID:    "conv-1",
			Action:            types.MessageActionSend,
			Allowed:           true,
			PermissionVersion: 9,
			Classification:    "INTERNAL",
			DecisionSource:    types.PolicyDecisionSourceExactRule,
		},
	}
	server := NewServer(executor)
	response, err := server.CheckMessageAction(context.Background(), &policyv1.CheckMessageActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		ConversationId: "conv-1",
		Action:         policyv1.MessageAction_MESSAGE_ACTION_SEND,
		MessageText:    "hello policy",
	})
	if err != nil {
		t.Fatalf("check message action: %v", err)
	}
	if !response.GetAllowed() || response.GetPermissionVersion() != 9 || response.GetAction() != policyv1.MessageAction_MESSAGE_ACTION_SEND || response.GetDecisionSource() != string(types.PolicyDecisionSourceExactRule) {
		t.Fatalf("unexpected response: %+v", response)
	}
	if executor.command.MessageText != "hello policy" {
		t.Fatalf("expected message text to reach executor, got %+v", executor.command)
	}
}

func TestServerCheckMessageActionWithStaticPolicy(t *testing.T) {
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
	if !response.GetAllowed() || response.GetPermissionVersion() != 9 || response.GetAction() != policyv1.MessageAction_MESSAGE_ACTION_SEND || response.GetDecisionSource() != string(types.PolicyDecisionSourceFallback) {
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

func TestServerCheckToolAction(t *testing.T) {
	executor := &capturingCheckToolActionExecutor{
		decision: types.ToolActionDecision{
			TenantID:          "tenant-1",
			UserID:            "user-1",
			ToolName:          "conversation.owner_transfer",
			Action:            types.ToolActionExecute,
			ResourceType:      "conversation",
			ResourceID:        "conv-1",
			RiskLevel:         types.ToolRiskLevelHigh,
			Allowed:           true,
			RequiresApproval:  true,
			PermissionVersion: 12,
			Classification:    "TOOL_APPROVAL_REQUIRED",
			Reason:            "operator approval required",
			DecisionSource:    types.PolicyDecisionSourceToolRule,
		},
	}
	server := NewServer(
		app.NewCheckMessageActionUseCase(domain.StaticMessagePolicy{Allowed: true}),
		executor,
	)
	response, err := server.CheckToolAction(context.Background(), &policyv1.CheckToolActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		ToolName:     "conversation.owner_transfer",
		Action:       policyv1.ToolAction_TOOL_ACTION_EXECUTE,
		ResourceType: "conversation",
		ResourceId:   "conv-1",
		RiskLevel:    string(types.ToolRiskLevelHigh),
		Intent:       "transfer owner",
	})
	if err != nil {
		t.Fatalf("check tool action: %v", err)
	}
	if !response.GetAllowed() ||
		!response.GetRequiresApproval() ||
		response.GetPermissionVersion() != 12 ||
		response.GetAction() != policyv1.ToolAction_TOOL_ACTION_EXECUTE ||
		response.GetDecisionSource() != string(types.PolicyDecisionSourceToolRule) {
		t.Fatalf("unexpected response: %+v", response)
	}
	if executor.command.Intent != "transfer owner" || executor.command.RiskLevel != types.ToolRiskLevelHigh {
		t.Fatalf("expected tool command to reach executor, got %+v", executor.command)
	}
}

func TestServerCheckToolActionUnconfigured(t *testing.T) {
	server := NewServer(app.NewCheckMessageActionUseCase(domain.StaticMessagePolicy{Allowed: true}))
	_, err := server.CheckToolAction(context.Background(), &policyv1.CheckToolActionRequest{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("expected Unimplemented, got %v", err)
	}
}

func TestServerCheckToolActionInvalidRequest(t *testing.T) {
	server := NewServer(
		app.NewCheckMessageActionUseCase(domain.StaticMessagePolicy{Allowed: true}),
		app.NewCheckToolActionUseCase(domain.StaticToolPolicy{Allowed: false}),
	)
	_, err := server.CheckToolAction(context.Background(), &policyv1.CheckToolActionRequest{})
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

type capturingCheckMessageActionExecutor struct {
	command  types.CheckMessageActionCommand
	decision types.MessageActionDecision
	err      error
}

type capturingCheckToolActionExecutor struct {
	command  types.CheckToolActionCommand
	decision types.ToolActionDecision
	err      error
}

func (c *capturingCheckToolActionExecutor) Execute(
	_ context.Context,
	command types.CheckToolActionCommand,
) (types.ToolActionDecision, error) {
	c.command = command
	return c.decision, c.err
}

func (c *capturingCheckMessageActionExecutor) Execute(
	_ context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, error) {
	c.command = command
	return c.decision, c.err
}
