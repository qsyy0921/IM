package monitoring

import (
	"sort"
	"sync"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type DecisionMetrics struct {
	mu        sync.Mutex
	actions   map[string]*decisionActionMetrics
	stages    map[string]*decisionStageMetrics
	evaluator map[string]*decisionStageMetrics
}

type decisionActionMetrics struct {
	total          int64
	allowed        int64
	denied         int64
	errors         int64
	totalLatencyMS int64
	maxLatencyMS   int64
}

type decisionStageMetrics struct {
	total          int64
	errors         int64
	totalLatencyMS int64
	maxLatencyMS   int64
	recent         []int64
}

func NewDecisionMetrics() *DecisionMetrics {
	return &DecisionMetrics{
		actions:   make(map[string]*decisionActionMetrics),
		stages:    make(map[string]*decisionStageMetrics),
		evaluator: make(map[string]*decisionStageMetrics),
	}
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

func (metrics *DecisionMetrics) RecordPolicyDecisionStage(action types.MessageAction, stage string, failed bool, latencyMS int64) {
	if metrics == nil {
		return
	}
	metrics.recordStage(metrics.stages, string(action), stage, failed, latencyMS)
}

func (metrics *DecisionMetrics) RecordPolicyEvaluatorStage(action types.MessageAction, stage string, failed bool, latency time.Duration) {
	if metrics == nil {
		return
	}
	metrics.recordStage(metrics.evaluator, string(action), stage, failed, latency.Milliseconds())
}

func (metrics *DecisionMetrics) recordStage(buckets map[string]*decisionStageMetrics, action string, stage string, failed bool, latencyMS int64) {
	if action == "" {
		action = "UNSPECIFIED"
	}
	if stage == "" {
		stage = "unspecified"
	}
	key := action + "\x00" + stage
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	stageMetrics := buckets[key]
	if stageMetrics == nil {
		stageMetrics = &decisionStageMetrics{}
		buckets[key] = stageMetrics
	}
	stageMetrics.total++
	if failed {
		stageMetrics.errors++
	}
	stageMetrics.totalLatencyMS += latencyMS
	if latencyMS > stageMetrics.maxLatencyMS {
		stageMetrics.maxLatencyMS = latencyMS
	}
	stageMetrics.recent = append(stageMetrics.recent, latencyMS)
	if len(stageMetrics.recent) > 4096 {
		copy(stageMetrics.recent, stageMetrics.recent[len(stageMetrics.recent)-4096:])
		stageMetrics.recent = stageMetrics.recent[:4096]
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
	snapshot.Stages = decisionStageSnapshots(metrics.stages)
	snapshot.EvaluatorStages = decisionStageSnapshots(metrics.evaluator)
	return snapshot
}

type DecisionSnapshot struct {
	Total           int64                    `json:"total"`
	Allowed         int64                    `json:"allowed"`
	Denied          int64                    `json:"denied"`
	Errors          int64                    `json:"errors"`
	Actions         []DecisionActionSnapshot `json:"actions"`
	Stages          []DecisionStageSnapshot  `json:"stages,omitempty"`
	EvaluatorStages []DecisionStageSnapshot  `json:"evaluator_stages,omitempty"`
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

type DecisionStageSnapshot struct {
	Action       string `json:"action"`
	Stage        string `json:"stage"`
	Total        int64  `json:"total"`
	Errors       int64  `json:"errors"`
	LatencyAvgMS int64  `json:"latency_avg_ms"`
	LatencyP95MS int64  `json:"latency_p95_ms"`
	LatencyP99MS int64  `json:"latency_p99_ms"`
	LatencyMaxMS int64  `json:"latency_max_ms"`
}

func decisionStageSnapshots(values map[string]*decisionStageMetrics) []DecisionStageSnapshot {
	snapshots := make([]DecisionStageSnapshot, 0, len(values))
	for key, metrics := range values {
		action, stage := splitDecisionStageKey(key)
		snapshot := DecisionStageSnapshot{
			Action:       action,
			Stage:        stage,
			Total:        metrics.total,
			Errors:       metrics.errors,
			LatencyAvgMS: averageLatency(metrics.totalLatencyMS, metrics.total),
			LatencyMaxMS: metrics.maxLatencyMS,
		}
		if len(metrics.recent) > 0 {
			recent := append([]int64(nil), metrics.recent...)
			sort.Slice(recent, func(i, j int) bool { return recent[i] < recent[j] })
			snapshot.LatencyP95MS = percentileLatency(recent, 0.95)
			snapshot.LatencyP99MS = percentileLatency(recent, 0.99)
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Action == snapshots[j].Action {
			return snapshots[i].Stage < snapshots[j].Stage
		}
		return snapshots[i].Action < snapshots[j].Action
	})
	return snapshots
}

func splitDecisionStageKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			return key[:i], key[i+1:]
		}
	}
	return key, "unspecified"
}

func percentileLatency(sorted []int64, percentile float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	if percentile <= 0 {
		return sorted[0]
	}
	if percentile >= 1 {
		return sorted[len(sorted)-1]
	}
	index := int(float64(len(sorted)-1) * percentile)
	return sorted[index]
}
