package app

import (
	"context"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type VerifyApprovedAgentProposalUseCase struct {
	store ProposalRepository
}

func NewVerifyApprovedAgentProposalUseCase(store ProposalRepository) VerifyApprovedAgentProposalUseCase {
	return VerifyApprovedAgentProposalUseCase{store: store}
}

func (usecase VerifyApprovedAgentProposalUseCase) Execute(
	ctx context.Context,
	command types.VerifyApprovedAgentProposalCommand,
) (types.VerifyApprovedAgentProposalResult, error) {
	if err := command.Validate(); err != nil {
		return types.VerifyApprovedAgentProposalResult{}, err
	}
	if usecase.store == nil {
		return types.VerifyApprovedAgentProposalResult{}, types.ErrProposalStoreUnavailable
	}
	return usecase.store.VerifyApprovedAgentProposal(ctx, command.Normalized())
}
