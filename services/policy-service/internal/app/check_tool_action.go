package app

import (
	"context"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type ToolPolicyEvaluator interface {
	DecideToolAction(context.Context, types.CheckToolActionCommand) (types.ToolActionDecision, error)
}

type ToolDecisionAuditor interface {
	RecordToolDecision(context.Context, types.CheckToolActionCommand, types.ToolActionDecision) error
}

type CheckToolActionUseCase struct {
	evaluator ToolPolicyEvaluator
	auditor   ToolDecisionAuditor
}

type CheckToolActionOption func(*CheckToolActionUseCase)

func NewCheckToolActionUseCase(evaluator ToolPolicyEvaluator, opts ...CheckToolActionOption) CheckToolActionUseCase {
	useCase := CheckToolActionUseCase{evaluator: evaluator}
	for _, opt := range opts {
		opt(&useCase)
	}
	return useCase
}

func WithToolDecisionAuditor(auditor ToolDecisionAuditor) CheckToolActionOption {
	return func(useCase *CheckToolActionUseCase) {
		useCase.auditor = auditor
	}
}

func (u CheckToolActionUseCase) Execute(
	ctx context.Context,
	command types.CheckToolActionCommand,
) (types.ToolActionDecision, error) {
	if err := command.Validate(); err != nil {
		return types.ToolActionDecision{}, err
	}
	if u.evaluator == nil {
		return types.ToolActionDecision{}, types.NewDependencyUnavailable("tool policy evaluator is not configured")
	}
	decision, err := u.evaluator.DecideToolAction(ctx, command)
	if err != nil {
		return types.ToolActionDecision{}, err
	}
	if u.auditor != nil {
		if err := u.auditor.RecordToolDecision(ctx, command, decision); err != nil {
			return types.ToolActionDecision{}, err
		}
	}
	return decision, nil
}
