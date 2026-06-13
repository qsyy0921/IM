package monitoring

import (
	"testing"

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
