package app

import (
	"context"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type RetrievalPort interface {
	RetrieveEvidence(context.Context, types.RetrieveEvidenceQuery) (types.RetrieveEvidenceResult, error)
}

type ToolPreparePort interface {
	PrepareToolCall(context.Context, types.PrepareToolCallCommand) (types.ToolPrepareResult, error)
}

type ProposalProvider interface {
	GenerateProposal(context.Context, types.AgentProposalGenerationRequest) (types.AgentProposalGenerationResult, error)
}
