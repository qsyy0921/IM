package postgres

import (
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type MessagePolicyEvaluatorObserver interface {
	RecordPolicyEvaluatorStage(action types.MessageAction, stage string, failed bool, latency time.Duration)
}

type MessagePolicyEvaluatorOption func(*MessagePolicyEvaluator)

func WithMessagePolicyEvaluatorObserver(observer MessagePolicyEvaluatorObserver) MessagePolicyEvaluatorOption {
	return func(evaluator *MessagePolicyEvaluator) {
		evaluator.observer = observer
	}
}

func (e MessagePolicyEvaluator) observeStage(action types.MessageAction, stage string, started time.Time, err error) {
	if e.observer == nil {
		return
	}
	e.observer.RecordPolicyEvaluatorStage(action, stage, err != nil, time.Since(started))
}
