package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type PolicyClient struct {
	client  policyv1.PolicyServiceClient
	timeout time.Duration
}

func NewPolicyClient(client policyv1.PolicyServiceClient, timeout time.Duration) PolicyClient {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return PolicyClient{client: client, timeout: timeout}
}

func DialPolicyClient(_ context.Context, addr string, timeout time.Duration) (PolicyClient, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return PolicyClient{}, nil, errors.New("policy-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return PolicyClient{}, nil, err
	}
	return NewPolicyClient(policyv1.NewPolicyServiceClient(conn), timeout), conn.Close, nil
}

func (client PolicyClient) CheckToolAction(
	ctx context.Context,
	command types.CheckToolActionCommand,
) (types.ToolPolicyDecision, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	callCtx = outgoingMetadataContext(callCtx, command.AuthContext)
	response, err := client.client.CheckToolAction(callCtx, &policyv1.CheckToolActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId:  string(command.AuthContext.TenantID),
			UserId:    string(command.AuthContext.UserID),
			DeviceId:  command.AuthContext.DeviceID,
			SessionId: command.AuthContext.SessionID,
			TraceId:   command.AuthContext.TraceID,
			RequestId: command.AuthContext.RequestID,
		},
		ToolName:     command.ToolName,
		Action:       toolActionToProto(command.Action),
		ResourceType: command.ResourceType,
		ResourceId:   command.ResourceID,
		RiskLevel:    command.RiskLevel,
		Intent:       command.Intent,
	})
	if err != nil {
		return types.ToolPolicyDecision{}, mapPolicyError(err)
	}
	return types.ToolPolicyDecision{
		TenantID:          types.TenantID(response.GetTenantId()),
		UserID:            types.UserID(response.GetUserId()),
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
	}, nil
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
