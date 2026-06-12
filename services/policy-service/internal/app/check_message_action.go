package app

import (
	"context"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type MessagePolicyEvaluator interface {
	DecideMessageAction(context.Context, types.CheckMessageActionCommand) (types.MessageActionDecision, error)
}

type CheckMessageActionUseCase struct {
	evaluator MessagePolicyEvaluator
}

func NewCheckMessageActionUseCase(evaluator MessagePolicyEvaluator) CheckMessageActionUseCase {
	return CheckMessageActionUseCase{evaluator: evaluator}
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
	return u.evaluator.DecideMessageAction(ctx, command)
}
