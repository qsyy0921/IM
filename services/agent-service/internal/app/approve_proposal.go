package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type ApproveAgentProposalUseCase struct {
	store ProposalRepository
}

func NewApproveAgentProposalUseCase(store ProposalRepository) ApproveAgentProposalUseCase {
	return ApproveAgentProposalUseCase{store: store}
}

func (usecase ApproveAgentProposalUseCase) Execute(
	ctx context.Context,
	command types.ApproveAgentProposalCommand,
) (types.ApproveAgentProposalResult, error) {
	if err := command.Validate(); err != nil {
		return types.ApproveAgentProposalResult{}, err
	}
	if usecase.store == nil {
		return types.ApproveAgentProposalResult{}, types.ErrProposalStoreUnavailable
	}
	return usecase.store.ApproveAgentProposal(ctx, command, newApprovalID())
}

func newApprovalID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "appr_recovery"
	}
	return "appr_" + hex.EncodeToString(buf[:])
}
