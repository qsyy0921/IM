package monitoring

import (
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestDecisionMetricsRecordsAllowDenyAndErrors(t *testing.T) {
	metrics := NewDecisionMetrics()
	metrics.RecordPolicyDecisionMetric(types.MessageActionSend, true, false, 7)
	metrics.RecordPolicyDecisionMetric(types.MessageActionEdit, false, false, 5)
	metrics.RecordPolicyDecisionMetric(types.MessageActionDelete, false, true, 3)

	snapshot := metrics.Snapshot()
	if snapshot.Total != 3 || snapshot.Allowed != 1 || snapshot.Denied != 1 || snapshot.Errors != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if len(snapshot.Actions) != 3 {
		t.Fatalf("expected one action bucket per action, got %+v", snapshot.Actions)
	}
}

func TestDecisionMetricsRecordsPolicyStages(t *testing.T) {
	metrics := NewDecisionMetrics()
	metrics.RecordPolicyDecisionStage(types.MessageActionSend, "decision_audit_outbox", false, 7)
	metrics.RecordPolicyDecisionStage(types.MessageActionSend, "decision_audit_outbox", true, 11)
	metrics.RecordPolicyEvaluatorStage(types.MessageActionSend, "tenant_rule_lookup", false, 13*time.Millisecond)
	metrics.RecordPolicyEvaluatorStage(types.MessageActionSend, "tenant_rule_lookup", false, 17*time.Millisecond)

	snapshot := metrics.Snapshot()
	if len(snapshot.Stages) != 1 {
		t.Fatalf("expected decision stage snapshot, got %+v", snapshot.Stages)
	}
	if stage := snapshot.Stages[0]; stage.Action != "SEND" || stage.Stage != "decision_audit_outbox" || stage.Total != 2 || stage.Errors != 1 || stage.LatencyMaxMS != 11 {
		t.Fatalf("unexpected decision stage: %+v", stage)
	}
	if len(snapshot.EvaluatorStages) != 1 {
		t.Fatalf("expected evaluator stage snapshot, got %+v", snapshot.EvaluatorStages)
	}
	if stage := snapshot.EvaluatorStages[0]; stage.Action != "SEND" || stage.Stage != "tenant_rule_lookup" || stage.Total != 2 || stage.LatencyMaxMS != 17 {
		t.Fatalf("unexpected evaluator stage: %+v", stage)
	}
}

func TestDecisionStagePrometheusOutput(t *testing.T) {
	metrics := NewDecisionMetrics()
	metrics.RecordPolicyDecisionStage(types.MessageActionSend, "decision_audit_outbox", false, 7)
	metrics.RecordPolicyEvaluatorStage(types.MessageActionSend, "tenant_rule_lookup", false, 13*time.Millisecond)

	decisionSnapshot := metrics.Snapshot()
	body := renderPrometheus(Snapshot{Decisions: &decisionSnapshot})
	for _, want := range []string{
		`nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_outbox"} 7`,
		`nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="tenant_rule_lookup"} 13`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected prometheus body to contain %q, got %s", want, body)
		}
	}
}
