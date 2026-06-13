package app

import (
	"context"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type MessagePolicyEvaluator interface {
	DecideMessageAction(context.Context, types.CheckMessageActionCommand) (types.MessageActionDecision, error)
}

type PolicyDecisionAuditor interface {
	RecordPolicyDecision(context.Context, types.CheckMessageActionCommand, types.MessageActionDecision) error
}

type CheckMessageActionUseCase struct {
	evaluator MessagePolicyEvaluator
	auditor   PolicyDecisionAuditor
}

type CheckMessageActionOption func(*CheckMessageActionUseCase)

func NewCheckMessageActionUseCase(evaluator MessagePolicyEvaluator, opts ...CheckMessageActionOption) CheckMessageActionUseCase {
	useCase := CheckMessageActionUseCase{evaluator: evaluator}
	for _, opt := range opts {
		opt(&useCase)
	}
	return useCase
}

func WithPolicyDecisionAuditor(auditor PolicyDecisionAuditor) CheckMessageActionOption {
	return func(useCase *CheckMessageActionUseCase) {
		useCase.auditor = auditor
	}
}

func (u CheckMessageActionUseCase) Execute(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, error) {
	if err := command.Validate(); err != nil {
		return types.MessageActionDecision{}, err
	}
	if u.evaluator == nil {
		return types.MessageActionDecision{}, types.NewDependencyUnavailable("policy evaluator is not configured")
	}
	decision, err := u.evaluator.DecideMessageAction(ctx, command)
	if err != nil {
		return types.MessageActionDecision{}, err
	}
	if u.auditor != nil {
		if err := u.auditor.RecordPolicyDecision(ctx, command, decision); err != nil {
			return types.MessageActionDecision{}, err
		}
	}
	return decision, nil
}
