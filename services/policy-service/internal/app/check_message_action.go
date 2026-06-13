package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type MessagePolicyEvaluator interface {
	DecideMessageAction(context.Context, types.CheckMessageActionCommand) (types.MessageActionDecision, error)
}

type PolicyDecisionAuditor interface {
	RecordPolicyDecision(context.Context, types.CheckMessageActionCommand, types.MessageActionDecision) error
}

type MessageOwnershipOverrideChecker interface {
	DecideMessageOwnershipOverride(
		context.Context,
		types.CheckMessageActionCommand,
	) (types.MessageActionDecision, bool, error)
}

type PolicyDecisionObserver interface {
	RecordPolicyDecisionMetric(action types.MessageAction, allowed bool, failed bool, latencyMS int64)
}

type CheckMessageActionUseCase struct {
	evaluator        MessagePolicyEvaluator
	auditor          PolicyDecisionAuditor
	ownershipChecker MessageOwnershipOverrideChecker
	observer         PolicyDecisionObserver
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

func WithMessageOwnershipOverrideChecker(checker MessageOwnershipOverrideChecker) CheckMessageActionOption {
	return func(useCase *CheckMessageActionUseCase) {
		useCase.ownershipChecker = checker
	}
}

func WithPolicyDecisionObserver(observer PolicyDecisionObserver) CheckMessageActionOption {
	return func(useCase *CheckMessageActionUseCase) {
		useCase.observer = observer
	}
}

func (u CheckMessageActionUseCase) Execute(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (decision types.MessageActionDecision, err error) {
	if err = command.Validate(); err != nil {
		return types.MessageActionDecision{}, err
	}
	started := time.Now()
	defer func() {
		if u.observer != nil {
			u.observer.RecordPolicyDecisionMetric(command.Action, decision.Allowed, err != nil, time.Since(started).Milliseconds())
		}
	}()
	if u.evaluator == nil {
		return types.MessageActionDecision{}, types.NewDependencyUnavailable("policy evaluator is not configured")
	}
	if ownershipDecision, handled, err := u.messageOwnershipDecision(ctx, command); err != nil {
		return types.MessageActionDecision{}, err
	} else if handled {
		decision = ownershipDecision
		if u.auditor != nil {
			if err := u.auditor.RecordPolicyDecision(ctx, command, decision); err != nil {
				return types.MessageActionDecision{}, err
			}
		}
		return decision, nil
	}
	decision, err = u.evaluator.DecideMessageAction(ctx, command)
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

func (u CheckMessageActionUseCase) messageOwnershipDecision(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
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
	if u.ownershipChecker != nil {
		decision, allowed, err := u.ownershipChecker.DecideMessageOwnershipOverride(ctx, command)
		if err != nil {
			return types.MessageActionDecision{}, false, err
		}
		if allowed {
			return decision, true, nil
		}
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
