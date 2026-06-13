package monitoring

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestInstrumentedEvaluatorRecordsAllowDenyAndErrors(t *testing.T) {
	metrics := NewDecisionMetrics()
	evaluator := NewInstrumentedEvaluator(fakeEvaluator{
		decision: types.MessageActionDecision{Allowed: true},
	}, metrics)
	command := types.CheckMessageActionCommand{Action: types.MessageActionSend}
	if _, err := evaluator.DecideMessageAction(context.Background(), command); err != nil {
		t.Fatalf("allow decision: %v", err)
	}

	evaluator = NewInstrumentedEvaluator(fakeEvaluator{
		decision: types.MessageActionDecision{Allowed: false},
	}, metrics)
	command.Action = types.MessageActionEdit
	if _, err := evaluator.DecideMessageAction(context.Background(), command); err != nil {
		t.Fatalf("deny decision: %v", err)
	}

	evaluator = NewInstrumentedEvaluator(fakeEvaluator{err: errors.New("policy backend failed")}, metrics)
	command.Action = types.MessageActionDelete
	if _, err := evaluator.DecideMessageAction(context.Background(), command); err == nil {
		t.Fatal("expected backend error")
	}

	snapshot := metrics.Snapshot()
	if snapshot.Total != 3 || snapshot.Allowed != 1 || snapshot.Denied != 1 || snapshot.Errors != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if len(snapshot.Actions) != 3 {
		t.Fatalf("expected one action bucket per action, got %+v", snapshot.Actions)
	}
}

type fakeEvaluator struct {
	decision types.MessageActionDecision
	err      error
}

func (f fakeEvaluator) DecideMessageAction(context.Context, types.CheckMessageActionCommand) (types.MessageActionDecision, error) {
	return f.decision, f.err
}
