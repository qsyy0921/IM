package monitoring

import (
	"sync"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type DecisionMetrics struct {
	mu      sync.Mutex
	actions map[string]*decisionActionMetrics
}

type decisionActionMetrics struct {
	total          int64
	allowed        int64
	denied         int64
	errors         int64
	totalLatencyMS int64
	maxLatencyMS   int64
}

func NewDecisionMetrics() *DecisionMetrics {
	return &DecisionMetrics{actions: make(map[string]*decisionActionMetrics)}
}

func (metrics *DecisionMetrics) RecordPolicyDecisionMetric(action types.MessageAction, allowed bool, failed bool, latencyMS int64) {
	metrics.Record(string(action), allowed, failed, latencyMS)
}

func (metrics *DecisionMetrics) Record(action string, allowed bool, failed bool, latencyMS int64) {
	if metrics == nil {
		return
	}
	if action == "" {
		action = "UNSPECIFIED"
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	actionMetrics := metrics.actions[action]
	if actionMetrics == nil {
		actionMetrics = &decisionActionMetrics{}
		metrics.actions[action] = actionMetrics
	}
	actionMetrics.total++
	if failed {
		actionMetrics.errors++
	} else if allowed {
		actionMetrics.allowed++
	} else {
		actionMetrics.denied++
	}
	actionMetrics.totalLatencyMS += latencyMS
	if latencyMS > actionMetrics.maxLatencyMS {
		actionMetrics.maxLatencyMS = latencyMS
	}
}

func (metrics *DecisionMetrics) Snapshot() DecisionSnapshot {
	if metrics == nil {
		return DecisionSnapshot{}
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	snapshot := DecisionSnapshot{Actions: make([]DecisionActionSnapshot, 0, len(metrics.actions))}
	for action, actionMetrics := range metrics.actions {
		actionSnapshot := DecisionActionSnapshot{
			Action:       action,
			Total:        actionMetrics.total,
			Allowed:      actionMetrics.allowed,
			Denied:       actionMetrics.denied,
			Errors:       actionMetrics.errors,
			LatencyAvgMS: averageLatency(actionMetrics.totalLatencyMS, actionMetrics.total),
			LatencyMaxMS: actionMetrics.maxLatencyMS,
		}
		snapshot.Total += actionMetrics.total
		snapshot.Allowed += actionMetrics.allowed
		snapshot.Denied += actionMetrics.denied
		snapshot.Errors += actionMetrics.errors
		snapshot.Actions = append(snapshot.Actions, actionSnapshot)
	}
	return snapshot
}

type DecisionSnapshot struct {
	Total   int64                    `json:"total"`
	Allowed int64                    `json:"allowed"`
	Denied  int64                    `json:"denied"`
	Errors  int64                    `json:"errors"`
	Actions []DecisionActionSnapshot `json:"actions"`
}

type DecisionActionSnapshot struct {
	Action       string `json:"action"`
	Total        int64  `json:"total"`
	Allowed      int64  `json:"allowed"`
	Denied       int64  `json:"denied"`
	Errors       int64  `json:"errors"`
	LatencyAvgMS int64  `json:"latency_avg_ms"`
	LatencyMaxMS int64  `json:"latency_max_ms"`
}
