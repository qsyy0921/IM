package postgres

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type MessagePolicyEvaluatorObserver interface {
	RecordPolicyEvaluatorStage(action types.MessageAction, stage string, failed bool, latency time.Duration)
}

type MessageDecisionCache interface {
	GetMessageDecision(context.Context, string) (types.MessageActionDecision, bool, error)
	SetMessageDecision(context.Context, string, types.MessageActionDecision, time.Duration) error
}

type MessagePolicyEvaluatorOption func(*MessagePolicyEvaluator)

func WithMessagePolicyEvaluatorObserver(observer MessagePolicyEvaluatorObserver) MessagePolicyEvaluatorOption {
	return func(evaluator *MessagePolicyEvaluator) {
		evaluator.observer = observer
	}
}

func WithMessageDecisionCache(cache MessageDecisionCache, ttl time.Duration) MessagePolicyEvaluatorOption {
	return func(evaluator *MessagePolicyEvaluator) {
		evaluator.decisionCache = cache
		evaluator.decisionCacheTTL = ttl
	}
}

func (e MessagePolicyEvaluator) observeStage(action types.MessageAction, stage string, started time.Time, err error) {
	if e.observer == nil {
		return
	}
	e.observer.RecordPolicyEvaluatorStage(action, stage, err != nil, time.Since(started))
}
