package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

type RedriveProviderFailureUseCase struct {
	repository ProviderFailureRedriveRepository
	execute    ExecuteApprovedActionUseCase
}

func NewRedriveProviderFailureUseCase(
	repository ProviderFailureRedriveRepository,
	execute ExecuteApprovedActionUseCase,
) RedriveProviderFailureUseCase {
	return RedriveProviderFailureUseCase{repository: repository, execute: execute}
}

func (usecase RedriveProviderFailureUseCase) Execute(
	ctx context.Context,
	command types.RedriveProviderFailureCommand,
) (types.RedriveProviderFailureResult, error) {
	if err := command.Validate(); err != nil {
		return types.RedriveProviderFailureResult{}, err
	}
	command = command.Normalized()
	if usecase.repository == nil {
		return types.RedriveProviderFailureResult{}, types.ErrExecutionAuditFailed
	}
	source, err := usecase.repository.GetProviderFailureForRedrive(
		ctx,
		command.AuthContext.TenantID,
		command.ProviderFailureID,
	)
	if err != nil {
		return types.RedriveProviderFailureResult{}, err
	}
	if err := validateProviderFailureRedriveSource(source, command); err != nil {
		return types.RedriveProviderFailureResult{}, err
	}
	executeCommand := command.ExecuteCommand().Normalized()
	executeCommand.RedriveProviderFailureID = source.ProviderFailureID
	executeCommand.RedriveReasonSHA256 = command.ReasonSHA256
	redriveResult, err := usecase.execute.Execute(ctx, executeCommand)
	if err != nil {
		return types.RedriveProviderFailureResult{}, err
	}
	return types.RedriveProviderFailureResult{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ProviderFailureID: source.ProviderFailureID,
		SourceExecutionID: source.ExecutionID,
		SourceResultID:    source.ResultID,
		RedriveResult:     redriveResult,
	}, nil
}

func validateProviderFailureRedriveSource(
	source types.ProviderFailureAuditRow,
	command types.RedriveProviderFailureCommand,
) error {
	if source.Status != types.ProviderFailureStatusDLQ {
		return fmt.Errorf("%w: source provider failure must be DLQ", types.ErrProviderFailureNotRedrivable)
	}
	if source.ProviderFailureID != command.ProviderFailureID {
		return fmt.Errorf("%w: provider failure mismatch", types.ErrProviderFailureNotRedrivable)
	}
	if strings.TrimSpace(source.SkillID) != command.SkillID ||
		strings.TrimSpace(source.ToolName) != command.ToolName ||
		strings.TrimSpace(source.ResourceType) != command.ResourceType ||
		strings.TrimSpace(source.ResourceID) != command.ResourceID {
		return fmt.Errorf("%w: redrive target must match source failure", types.ErrProviderFailureNotRedrivable)
	}
	if source.ProposalID == command.ProposalID ||
		source.ApprovalID == command.ApprovalID ||
		source.PreparedAuditID == command.PreparedAuditID {
		return fmt.Errorf("%w: redrive requires fresh proposal, approval and prepared audit", types.ErrProviderFailureNotRedrivable)
	}
	return nil
}
