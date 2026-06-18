package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
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

func (client PolicyClient) CheckRetrieveEvidence(
	ctx context.Context,
	check types.RetrievalPolicyCheck,
) (types.RetrievalPolicyDecision, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	callCtx = outgoingMetadataContext(callCtx, check.AuthContext)

	response, err := client.client.CheckToolAction(callCtx, &policyv1.CheckToolActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId:  string(check.AuthContext.TenantID),
			UserId:    string(check.AuthContext.UserID),
			DeviceId:  check.AuthContext.DeviceID,
			SessionId: check.AuthContext.SessionID,
			TraceId:   check.AuthContext.TraceID,
			RequestId: check.AuthContext.RequestID,
		},
		ToolName:     types.RetrievalPolicyToolName,
		Action:       policyv1.ToolAction_TOOL_ACTION_CALL,
		ResourceType: retrievalResourceType(check),
		ResourceId:   retrievalResourceID(check),
		RiskLevel:    types.RetrievalPolicyRiskLow,
		Intent:       types.RetrievalPolicyIntent,
	})
	if err != nil {
		return types.RetrievalPolicyDecision{}, mapPolicyError(err)
	}
	if response == nil {
		return types.RetrievalPolicyDecision{}, types.ErrRetrievalUnavailable
	}
	return types.RetrievalPolicyDecision{
		Allowed:           response.GetAllowed(),
		RequiresApproval:  response.GetRequiresApproval(),
		PermissionVersion: response.GetPermissionVersion(),
		Classification:    response.GetClassification(),
		Reason:            response.GetReason(),
		DecisionSource:    response.GetDecisionSource(),
	}, nil
}

func retrievalResourceType(check types.RetrievalPolicyCheck) string {
	if strings.TrimSpace(string(check.ConversationID)) == "" {
		return types.RetrievalPolicyResourceTypeTenant
	}
	return types.RetrievalPolicyResourceTypeConversation
}

func retrievalResourceID(check types.RetrievalPolicyCheck) string {
	if strings.TrimSpace(string(check.ConversationID)) == "" {
		return string(check.AuthContext.TenantID)
	}
	return string(check.ConversationID)
}
