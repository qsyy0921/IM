package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	agentv1 "github.com/qsyy0921/IM/api/proto/nexusim/agent/v1"
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AgentProposalClient struct {
	client  agentv1.AgentServiceClient
	timeout time.Duration
}

func NewAgentProposalClient(client agentv1.AgentServiceClient, timeout time.Duration) AgentProposalClient {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return AgentProposalClient{client: client, timeout: timeout}
}

func DialAgentProposalClient(_ context.Context, addr string, timeout time.Duration) (AgentProposalClient, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return AgentProposalClient{}, nil, errors.New("agent-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return AgentProposalClient{}, nil, err
	}
	return NewAgentProposalClient(agentv1.NewAgentServiceClient(conn), timeout), conn.Close, nil
}

func (client AgentProposalClient) VerifyApprovedProposal(
	ctx context.Context,
	command types.VerifyApprovedProposalCommand,
) (types.ApprovedProposal, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	callCtx = outgoingMetadataContext(callCtx, command.AuthContext)
	response, err := client.client.VerifyApprovedAgentProposal(callCtx, &agentv1.VerifyApprovedAgentProposalRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId:  string(command.AuthContext.TenantID),
			UserId:    string(command.AuthContext.UserID),
			DeviceId:  command.AuthContext.DeviceID,
			SessionId: command.AuthContext.SessionID,
			TraceId:   command.AuthContext.TraceID,
			RequestId: command.AuthContext.RequestID,
		},
		ProposalId:      command.ProposalID,
		ApprovalId:      command.ApprovalID,
		PreparedAuditId: command.PreparedAuditID,
		SkillId:         command.SkillID,
		ToolName:        command.ToolName,
		ResourceType:    command.ResourceType,
		ResourceId:      command.ResourceID,
	})
	if err != nil {
		return types.ApprovedProposal{}, mapAgentProposalError(err)
	}
	return types.ApprovedProposal{
		ProposalID:      response.GetProposalId(),
		ApprovalID:      response.GetApprovalId(),
		Status:          proposalStatusFromProto(response.GetStatus()),
		UserID:          types.UserID(response.GetUserId()),
		ConversationID:  response.GetConversationId(),
		SkillID:         response.GetSkillId(),
		PreparedAuditID: response.GetPreparedAuditId(),
		ToolName:        response.GetToolName(),
		ResourceType:    response.GetResourceType(),
		ResourceID:      response.GetResourceId(),
		RiskLevel:       response.GetRiskLevel(),
		ApprovedAt:      time.UnixMilli(response.GetApprovedAtUnixMs()),
	}, nil
}

func proposalStatusFromProto(status agentv1.AgentProposalStatus) string {
	switch status {
	case agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_APPROVED:
		return "APPROVED"
	case agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_PROPOSED:
		return "PROPOSED"
	case agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_BLOCKED:
		return "BLOCKED"
	case agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_INSUFFICIENT_EVIDENCE:
		return "INSUFFICIENT_EVIDENCE"
	default:
		return ""
	}
}
