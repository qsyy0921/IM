package grpc

import (
	"context"
	"testing"
	"time"

	agentv1 "github.com/qsyy0921/IM/api/proto/nexusim/agent/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

func TestServerCreateAgentProposal(t *testing.T) {
	executor := &capturingExecutor{result: types.CreateAgentProposalResult{
		ProposalID:       "ap_1",
		Status:           types.AgentProposalStatusProposed,
		ProposalText:     "proposal",
		RequiresApproval: true,
		ToolPolicyDecision: types.ToolPolicyDecision{
			ToolName:          "conversation.note.create",
			Action:            types.ToolActionCall,
			ResourceType:      "conversation",
			ResourceID:        "conv-1",
			RiskLevel:         "LOW",
			Allowed:           true,
			PermissionVersion: 7,
		},
		Citations: []types.Citation{{
			EvidenceID:      "evidence-1",
			SourceType:      types.EvidenceSourceSearchMessage,
			SourceID:        "message-1",
			SourceEventID:   "event-1",
			ConversationID:  "conv-1",
			ConversationSeq: 2,
			OccurredAt:      time.UnixMilli(1710000000000),
		}},
		EvidencePack: types.EvidencePack{
			PackID:         "pack-1",
			TenantID:       "tenant-1",
			ConversationID: "conv-1",
		},
		AgentVersion:   types.AgentVersion,
		GeneratedByLLM: false,
	}}
	server := NewServer(executor)

	response, err := server.CreateAgentProposal(context.Background(), &agentv1.CreateAgentProposalRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId: "tenant-1",
			UserId:   "user-1",
			DeviceId: "device-1",
		},
		ConversationId: "conv-1",
		Objective:      "draft action plan",
		ToolName:       "conversation.note.create",
		ToolAction:     policyv1.ToolAction_TOOL_ACTION_CALL,
		ResourceType:   "conversation",
		ResourceId:     "conv-1",
		RiskLevel:      "LOW",
		IncludeSearch:  true,
		IncludeMemory:  true,
		MemoryStatuses: []retrievalv1.EvidenceMemoryStatus{
			retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE,
		},
	})
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if response.GetProposalId() != "ap_1" ||
		response.GetStatus() != agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_PROPOSED ||
		response.GetToolPolicyDecision().GetAction() != policyv1.ToolAction_TOOL_ACTION_CALL ||
		len(response.GetCitations()) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if executor.command.ToolAction != types.ToolActionCall ||
		executor.command.AuthContext.TenantID != "tenant-1" ||
		len(executor.command.MemoryStatuses) != 1 {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
}

func TestServerCreateAgentProposalRequiresAuth(t *testing.T) {
	server := NewServer(&capturingExecutor{})
	if _, err := server.CreateAgentProposal(context.Background(), &agentv1.CreateAgentProposalRequest{}); err == nil {
		t.Fatal("expected auth error")
	}
}

type capturingExecutor struct {
	command types.CreateAgentProposalCommand
	result  types.CreateAgentProposalResult
	err     error
}

func (executor *capturingExecutor) Execute(
	_ context.Context,
	command types.CreateAgentProposalCommand,
) (types.CreateAgentProposalResult, error) {
	executor.command = command
	return executor.result, executor.err
}
