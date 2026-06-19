package app

import (
	"context"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type RetrievalPort interface {
	RetrieveEvidence(context.Context, types.RetrieveEvidenceQuery) (types.RetrieveEvidenceResult, error)
}

type ToolPolicyPort interface {
	CheckToolAction(context.Context, types.CheckToolActionCommand) (types.ToolPolicyDecision, error)
}

type ProposalProvider interface {
	GenerateProposal(context.Context, types.AgentProposalGenerationRequest) (types.AgentProposalGenerationResult, error)
}
