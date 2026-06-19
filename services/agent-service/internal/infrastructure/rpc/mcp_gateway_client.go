package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	mcpgatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/mcpgateway/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type MCPGatewayClient struct {
	client  mcpgatewayv1.MCPGatewayServiceClient
	timeout time.Duration
}

func toolActionToProto(action string) policyv1.ToolAction {
	switch action {
	case types.ToolActionCall:
		return policyv1.ToolAction_TOOL_ACTION_CALL
	case types.ToolActionApprove:
		return policyv1.ToolAction_TOOL_ACTION_APPROVE
	case types.ToolActionExecute:
		return policyv1.ToolAction_TOOL_ACTION_EXECUTE
	default:
		return policyv1.ToolAction_TOOL_ACTION_UNSPECIFIED
	}
}

func toolActionFromProto(action policyv1.ToolAction) string {
	switch action {
	case policyv1.ToolAction_TOOL_ACTION_CALL:
		return types.ToolActionCall
	case policyv1.ToolAction_TOOL_ACTION_APPROVE:
		return types.ToolActionApprove
	case policyv1.ToolAction_TOOL_ACTION_EXECUTE:
		return types.ToolActionExecute
	default:
		return ""
	}
}

func NewMCPGatewayClient(client mcpgatewayv1.MCPGatewayServiceClient, timeout time.Duration) MCPGatewayClient {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return MCPGatewayClient{client: client, timeout: timeout}
}

func DialMCPGatewayClient(_ context.Context, addr string, timeout time.Duration) (MCPGatewayClient, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return MCPGatewayClient{}, nil, errors.New("mcp-gateway address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return MCPGatewayClient{}, nil, err
	}
	return NewMCPGatewayClient(mcpgatewayv1.NewMCPGatewayServiceClient(conn), timeout), conn.Close, nil
}

func (client MCPGatewayClient) PrepareToolCall(
	ctx context.Context,
	command types.PrepareToolCallCommand,
) (types.ToolPrepareResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	callCtx = outgoingMetadataContext(callCtx, command.AuthContext)
	response, err := client.client.PrepareToolCall(callCtx, &mcpgatewayv1.PrepareToolCallRequest{
		AuthContext: &mcpgatewayv1.AuthContext{
			TenantId:  string(command.AuthContext.TenantID),
			UserId:    string(command.AuthContext.UserID),
			DeviceId:  command.AuthContext.DeviceID,
			SessionId: command.AuthContext.SessionID,
			TraceId:   command.AuthContext.TraceID,
			RequestId: command.AuthContext.RequestID,
		},
		SkillId:        command.SkillID,
		ToolName:       command.ToolName,
		Action:         toolActionToProto(command.Action),
		ResourceType:   command.ResourceType,
		ResourceId:     command.ResourceID,
		RiskLevel:      command.RiskLevel,
		Intent:         command.Intent,
		InputJson:      command.InputJSON,
		IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return types.ToolPrepareResult{}, mapMCPGatewayError(err)
	}
	return types.ToolPrepareResult{
		TenantID:          types.TenantID(response.GetTenantId()),
		UserID:            types.UserID(response.GetUserId()),
		SkillID:           response.GetSkillId(),
		ToolName:          response.GetToolName(),
		Action:            toolActionFromProto(response.GetAction()),
		ResourceType:      response.GetResourceType(),
		ResourceID:        response.GetResourceId(),
		RiskLevel:         response.GetRiskLevel(),
		Allowed:           response.GetAllowed(),
		RequiresApproval:  response.GetRequiresApproval(),
		PermissionVersion: response.GetPermissionVersion(),
		Classification:    response.GetClassification(),
		Reason:            response.GetReason(),
		DecisionSource:    response.GetDecisionSource(),
		AuditID:           response.GetAuditId(),
	}, nil
}
