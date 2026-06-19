package grpc

import (
	"context"
	"errors"
	"testing"

	mcpgatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/mcpgateway/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/mcp-gateway/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPrepareToolCallMapsResult(t *testing.T) {
	executor := &fakePrepareExecutor{result: types.PrepareToolCallResult{
		TenantID:          "tenant-1",
		UserID:            "user-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.note.create",
		Action:            types.ToolActionCall,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 4,
		Classification:    "TOOL_ALLOWED",
		Reason:            "allowed",
		DecisionSource:    "test",
		AuditID:           "audit-1",
	}}
	server := NewServer(executor)

	response, err := server.PrepareToolCall(context.Background(), &mcpgatewayv1.PrepareToolCallRequest{
		AuthContext: &mcpgatewayv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		SkillId:  "skill-1",
		ToolName: "conversation.note.create",
		Action:   policyv1.ToolAction_TOOL_ACTION_CALL,
	})
	if err != nil {
		t.Fatalf("prepare tool call: %v", err)
	}
	if !response.GetAllowed() || response.GetAuditId() != "audit-1" ||
		response.GetAction() != policyv1.ToolAction_TOOL_ACTION_CALL {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestPrepareToolCallMapsUnavailable(t *testing.T) {
	server := NewServer(&fakePrepareExecutor{err: types.ErrSkillCatalogUnavailable})

	_, err := server.PrepareToolCall(context.Background(), &mcpgatewayv1.PrepareToolCallRequest{
		AuthContext: &mcpgatewayv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		SkillId:  "skill-1",
		ToolName: "tool-1",
		Action:   policyv1.ToolAction_TOOL_ACTION_CALL,
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestPrepareToolCallRequiresAuth(t *testing.T) {
	server := NewServer(&fakePrepareExecutor{err: errors.New("should not be called")})

	_, err := server.PrepareToolCall(context.Background(), &mcpgatewayv1.PrepareToolCallRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

type fakePrepareExecutor struct {
	result types.PrepareToolCallResult
	err    error
}

func (fake *fakePrepareExecutor) Execute(
	context.Context,
	types.PrepareToolCallCommand,
) (types.PrepareToolCallResult, error) {
	return fake.result, fake.err
}
