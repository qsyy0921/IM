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
	if decision, denied, err := messageOwnershipDecision(command); err != nil {
		return types.MessageActionDecision{}, err
	} else if denied {
		if u.auditor != nil {
			if err := u.auditor.RecordPolicyDecision(ctx, command, decision); err != nil {
				return types.MessageActionDecision{}, err
			}
		}
		return decision, nil
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

func messageOwnershipDecision(command types.CheckMessageActionCommand) (types.MessageActionDecision, bool, error) {
	switch command.Action {
	case types.MessageActionEdit, types.MessageActionRevoke, types.MessageActionDelete:
	default:
		return types.MessageActionDecision{}, false, nil
	}
	if command.MessageSenderUserID == "" || command.MessageSenderUserID == command.AuthContext.UserID {
		return types.MessageActionDecision{}, false, nil
	}
	if command.ConversationPermissionVersion <= 0 {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy conversation permission version is required")
	}
	return types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		MessageID:         command.MessageID,
		Action:            command.Action,
		Allowed:           false,
		PermissionVersion: command.ConversationPermissionVersion,
		Classification:    "MESSAGE_OWNERSHIP_DENIED",
		Reason:            "message ownership policy denied",
	}, true, nil
}
