package monitoring

import (
	"sort"
	"strconv"
	"strings"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func renderPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writePrometheusHeader(&builder, "nexusim_policy_build_info", "Policy service build marker.", "gauge")
	writePrometheusSample(&builder, "nexusim_policy_build_info", map[string]string{"service": serviceName}, 1)

	writeGRPCPrometheus(&builder, snapshot.GRPC)
	writeDecisionPrometheus(&builder, snapshot.Decisions)
	writePGPoolPrometheus(&builder, snapshot.PGPool)
	writeRuleStorePrometheus(&builder, snapshot.RuleStore, snapshot.RuleStoreError)
	writeProjectionPrometheus(&builder, snapshot.Projection, snapshot.ProjectionError)
	writeAuditOutboxPrometheus(&builder, snapshot.AuditOutbox, snapshot.AuditOutboxError)
	writeProjectionWorkerPrometheus(&builder, "contact", snapshot.ContactProjectionWorker)
	writeProjectionWorkerPrometheus(&builder, "timeline", snapshot.TimelineProjectionWorker)
	writeOutboxRelayPrometheus(&builder, snapshot.OutboxRelay)
	writeTracePrometheus(&builder, snapshot.Trace)
	return builder.String()
}

func writeGRPCPrometheus(builder *strings.Builder, snapshot *GRPCSnapshot) {
	writePrometheusHeader(builder, "nexusim_policy_grpc_method_requests_total", "Policy service gRPC requests by method.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_grpc_requests_total", "Policy service gRPC requests by method and code.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_grpc_method_errors_total", "Policy service gRPC errors by method.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_grpc_latency_avg_milliseconds", "Policy service average gRPC latency by method.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_grpc_latency_max_milliseconds", "Policy service max gRPC latency by method.", "gauge")
	if snapshot == nil {
		return
	}
	methods := append([]GRPCMethodSnapshot(nil), snapshot.Methods...)
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Method < methods[j].Method
	})
	for _, method := range methods {
		labels := map[string]string{"method": method.Method}
		writePrometheusSample(builder, "nexusim_policy_grpc_method_requests_total", labels, method.Count)
		writePrometheusSample(builder, "nexusim_policy_grpc_method_errors_total", labels, method.ErrorCount)
		writePrometheusSample(builder, "nexusim_policy_grpc_latency_avg_milliseconds", labels, method.LatencyAvgMS)
		writePrometheusSample(builder, "nexusim_policy_grpc_latency_max_milliseconds", labels, method.LatencyMaxMS)
		for _, code := range sortedKeys(method.Codes) {
			writePrometheusSample(builder, "nexusim_policy_grpc_requests_total", map[string]string{
				"method": method.Method,
				"code":   code,
			}, method.Codes[code])
		}
	}
}

func writeDecisionPrometheus(builder *strings.Builder, snapshot *DecisionSnapshot) {
	writePrometheusHeader(builder, "nexusim_policy_decisions_total", "Policy decisions by outcome.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_decision_action_total", "Policy decisions by action and outcome.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_decision_latency_avg_milliseconds", "Policy decision average latency by action.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_decision_latency_max_milliseconds", "Policy decision max latency by action.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_policy_decisions_total", map[string]string{"outcome": "total"}, snapshot.Total)
	writePrometheusSample(builder, "nexusim_policy_decisions_total", map[string]string{"outcome": "allowed"}, snapshot.Allowed)
	writePrometheusSample(builder, "nexusim_policy_decisions_total", map[string]string{"outcome": "denied"}, snapshot.Denied)
	writePrometheusSample(builder, "nexusim_policy_decisions_total", map[string]string{"outcome": "error"}, snapshot.Errors)
	actions := append([]DecisionActionSnapshot(nil), snapshot.Actions...)
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Action < actions[j].Action
	})
	for _, action := range actions {
		writePrometheusSample(builder, "nexusim_policy_decision_action_total", map[string]string{"action": action.Action, "outcome": "total"}, action.Total)
		writePrometheusSample(builder, "nexusim_policy_decision_action_total", map[string]string{"action": action.Action, "outcome": "allowed"}, action.Allowed)
		writePrometheusSample(builder, "nexusim_policy_decision_action_total", map[string]string{"action": action.Action, "outcome": "denied"}, action.Denied)
		writePrometheusSample(builder, "nexusim_policy_decision_action_total", map[string]string{"action": action.Action, "outcome": "error"}, action.Errors)
		writePrometheusSample(builder, "nexusim_policy_decision_latency_avg_milliseconds", map[string]string{"action": action.Action}, action.LatencyAvgMS)
		writePrometheusSample(builder, "nexusim_policy_decision_latency_max_milliseconds", map[string]string{"action": action.Action}, action.LatencyMaxMS)
	}
}

func writePGPoolPrometheus(builder *strings.Builder, snapshot *PGPoolSnapshot) {
	writePrometheusHeader(builder, "nexusim_policy_pg_pool_acquire_total", "Policy service PostgreSQL pool acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_pg_pool_acquire_duration_milliseconds_total", "Policy service PostgreSQL pool acquire duration total.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_pg_pool_canceled_acquire_total", "Policy service PostgreSQL pool canceled acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_pg_pool_empty_acquire_total", "Policy service PostgreSQL pool empty acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_pg_pool_conns", "Policy service PostgreSQL pool connection counts.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_policy_pg_pool_acquire_total", nil, snapshot.AcquireCount)
	writePrometheusSample(builder, "nexusim_policy_pg_pool_acquire_duration_milliseconds_total", nil, snapshot.AcquireDurationMS)
	writePrometheusSample(builder, "nexusim_policy_pg_pool_canceled_acquire_total", nil, snapshot.CanceledAcquireCount)
	writePrometheusSample(builder, "nexusim_policy_pg_pool_empty_acquire_total", nil, snapshot.EmptyAcquireCount)
	writePrometheusSample(builder, "nexusim_policy_pg_pool_conns", map[string]string{"state": "acquired"}, snapshot.AcquiredConns)
	writePrometheusSample(builder, "nexusim_policy_pg_pool_conns", map[string]string{"state": "constructing"}, snapshot.ConstructingConns)
	writePrometheusSample(builder, "nexusim_policy_pg_pool_conns", map[string]string{"state": "idle"}, snapshot.IdleConns)
	writePrometheusSample(builder, "nexusim_policy_pg_pool_conns", map[string]string{"state": "max"}, snapshot.MaxConns)
	writePrometheusSample(builder, "nexusim_policy_pg_pool_conns", map[string]string{"state": "total"}, snapshot.TotalConns)
}

func writeRuleStorePrometheus(builder *strings.Builder, snapshot *RuleSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_policy_rule_store_query_error", "Policy rule-store metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_rules", "Policy rule counts by scope and decision.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_rule_actions", "Policy allow/deny rule counts by scope, action and decision.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_role_rules", "Policy role rule counts by scope.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_role_rule_actions", "Policy role rule counts by scope, action and minimum role.", "gauge")
	writePrometheusSample(builder, "nexusim_policy_rule_store_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	writeRuleDecisionPrometheus(builder, "exact", snapshot.ExactMessageActions)
	writeRuleDecisionPrometheus(builder, "tenant", snapshot.TenantMessageActions)
	writeRoleRulePrometheus(builder, "conversation_role", snapshot.ConversationRoleActions)
	writeRoleRulePrometheus(builder, "ownership_override", snapshot.OwnershipOverrides)
}

func writeRuleDecisionPrometheus(builder *strings.Builder, scope string, snapshot *RuleDecisionSnapshot) {
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_policy_rules", map[string]string{"scope": scope, "decision": "total"}, snapshot.Total)
	writePrometheusSample(builder, "nexusim_policy_rules", map[string]string{"scope": scope, "decision": "allow"}, snapshot.Allow)
	writePrometheusSample(builder, "nexusim_policy_rules", map[string]string{"scope": scope, "decision": "deny"}, snapshot.Deny)
	actions := append([]RuleActionSnapshot(nil), snapshot.Actions...)
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Action < actions[j].Action
	})
	for _, action := range actions {
		writePrometheusSample(builder, "nexusim_policy_rule_actions", map[string]string{"scope": scope, "action": action.Action, "decision": "total"}, action.Total)
		writePrometheusSample(builder, "nexusim_policy_rule_actions", map[string]string{"scope": scope, "action": action.Action, "decision": "allow"}, action.Allow)
		writePrometheusSample(builder, "nexusim_policy_rule_actions", map[string]string{"scope": scope, "action": action.Action, "decision": "deny"}, action.Deny)
	}
}

func writeRoleRulePrometheus(builder *strings.Builder, scope string, snapshot *RuleRoleSnapshot) {
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_policy_role_rules", map[string]string{"scope": scope}, snapshot.Total)
	actions := append([]RuleRoleActionSnapshot(nil), snapshot.Actions...)
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Action == actions[j].Action {
			return actions[i].MinRole < actions[j].MinRole
		}
		return actions[i].Action < actions[j].Action
	})
	for _, action := range actions {
		writePrometheusSample(builder, "nexusim_policy_role_rule_actions", map[string]string{"scope": scope, "action": action.Action, "min_role": action.MinRole}, action.Total)
	}
}

func writeProjectionPrometheus(builder *strings.Builder, snapshot *ProjectionSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_policy_projection_query_error", "Policy projection metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_contact_edges_projection", "Policy contact edge projection counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_conversation_members_projection", "Policy conversation member projection counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_conversation_members_by_role", "Policy conversation member projection counts by role.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_conversation_members_by_status", "Policy conversation member projection counts by status.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_conversation_members_by_role_status", "Policy conversation member projection counts by role and status.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_kafka_checkpoints", "Policy Kafka checkpoint counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_kafka_checkpoint_topic_offsets", "Policy Kafka checkpoint topic offset range.", "gauge")
	writePrometheusSample(builder, "nexusim_policy_projection_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	if snapshot.ContactEdges != nil {
		writePrometheusSample(builder, "nexusim_policy_contact_edges_projection", map[string]string{"state": "total"}, snapshot.ContactEdges.Total)
		writePrometheusSample(builder, "nexusim_policy_contact_edges_projection", map[string]string{"state": "active"}, snapshot.ContactEdges.Active)
		writePrometheusSample(builder, "nexusim_policy_contact_edges_projection", map[string]string{"state": "blocked"}, snapshot.ContactEdges.Blocked)
		writePrometheusSample(builder, "nexusim_policy_contact_edges_projection", map[string]string{"state": "deleted"}, snapshot.ContactEdges.Deleted)
	}
	if snapshot.ConversationMembers != nil {
		writePrometheusSample(builder, "nexusim_policy_conversation_members_projection", map[string]string{"state": "total"}, snapshot.ConversationMembers.Total)
		writePrometheusSample(builder, "nexusim_policy_conversation_members_projection", map[string]string{"state": "active"}, snapshot.ConversationMembers.Active)
		writePrometheusSample(builder, "nexusim_policy_conversation_members_projection", map[string]string{"state": "left"}, snapshot.ConversationMembers.Left)
		writePrometheusSample(builder, "nexusim_policy_conversation_members_projection", map[string]string{"state": "banned"}, snapshot.ConversationMembers.Banned)
		for _, role := range snapshot.ConversationMembers.ByRole {
			writePrometheusSample(builder, "nexusim_policy_conversation_members_by_role", map[string]string{"role": role.Value}, role.Total)
		}
		for _, status := range snapshot.ConversationMembers.ByStatus {
			writePrometheusSample(builder, "nexusim_policy_conversation_members_by_status", map[string]string{"status": status.Value}, status.Total)
		}
		for _, pair := range snapshot.ConversationMembers.ByPair {
			writePrometheusSample(builder, "nexusim_policy_conversation_members_by_role_status", map[string]string{"role": pair.Role, "status": pair.Status}, pair.Total)
		}
	}
	if snapshot.KafkaCheckpoints != nil {
		writePrometheusSample(builder, "nexusim_policy_kafka_checkpoints", map[string]string{"state": "total"}, snapshot.KafkaCheckpoints.Total)
		for _, topic := range snapshot.KafkaCheckpoints.Topics {
			writePrometheusSample(builder, "nexusim_policy_kafka_checkpoints", map[string]string{"topic": topic.Topic, "state": "rows"}, topic.Rows)
			writePrometheusSample(builder, "nexusim_policy_kafka_checkpoints", map[string]string{"topic": topic.Topic, "state": "consumer_groups"}, topic.ConsumerGroups)
			writePrometheusSample(builder, "nexusim_policy_kafka_checkpoints", map[string]string{"topic": topic.Topic, "state": "partitions"}, topic.Partitions)
			writePrometheusSample(builder, "nexusim_policy_kafka_checkpoint_topic_offsets", map[string]string{"topic": topic.Topic, "bound": "min"}, topic.MinOffsetValue)
			writePrometheusSample(builder, "nexusim_policy_kafka_checkpoint_topic_offsets", map[string]string{"topic": topic.Topic, "bound": "max"}, topic.MaxOffsetValue)
		}
	}
}

func writeAuditOutboxPrometheus(builder *strings.Builder, snapshot *AuditOutboxSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_policy_audit_outbox_query_error", "Policy audit outbox metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_audit_outbox", "Policy decision audit outbox rows by state.", "gauge")
	writePrometheusSample(builder, "nexusim_policy_audit_outbox_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_policy_audit_outbox", map[string]string{"state": "total"}, snapshot.Total)
	writePrometheusSample(builder, "nexusim_policy_audit_outbox", map[string]string{"state": "pending"}, snapshot.Pending)
	writePrometheusSample(builder, "nexusim_policy_audit_outbox", map[string]string{"state": "published"}, snapshot.Published)
	writePrometheusSample(builder, "nexusim_policy_audit_outbox", map[string]string{"state": "dlq"}, snapshot.DLQ)
}

func writeProjectionWorkerPrometheus(builder *strings.Builder, worker string, snapshot *types.ProjectionWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_policy_projection_worker_errors_total", "Policy projection worker errors.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_projection_worker_consecutive_errors", "Policy projection worker consecutive errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_projection_worker_last_error_unix_milliseconds", "Policy projection worker last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_projection_worker_last_success_unix_milliseconds", "Policy projection worker last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_projection_worker_last_commit_unix_milliseconds", "Policy projection worker last commit timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_projection_worker_last_error_backoff_milliseconds", "Policy projection worker error backoff.", "gauge")
	if snapshot == nil {
		return
	}
	labels := map[string]string{"worker": worker}
	writePrometheusSample(builder, "nexusim_policy_projection_worker_errors_total", labels, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_policy_projection_worker_consecutive_errors", labels, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_policy_projection_worker_last_error_unix_milliseconds", labels, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_policy_projection_worker_last_success_unix_milliseconds", labels, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_policy_projection_worker_last_commit_unix_milliseconds", labels, snapshot.LastCommitAtMS)
	writePrometheusSample(builder, "nexusim_policy_projection_worker_last_error_backoff_milliseconds", labels, snapshot.LastErrorBackoffMS)
}

func writeOutboxRelayPrometheus(builder *strings.Builder, snapshot *types.OutboxRelayWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_policy_outbox_relay_errors_total", "Policy outbox relay errors.", "counter")
	writePrometheusHeader(builder, "nexusim_policy_outbox_relay_consecutive_errors", "Policy outbox relay consecutive errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_outbox_relay_last_error_unix_milliseconds", "Policy outbox relay last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_outbox_relay_last_success_unix_milliseconds", "Policy outbox relay last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_outbox_relay_last_published_unix_milliseconds", "Policy outbox relay last publish timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_outbox_relay_last_error_backoff_milliseconds", "Policy outbox relay error backoff.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_policy_outbox_relay_errors_total", nil, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_policy_outbox_relay_consecutive_errors", nil, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_policy_outbox_relay_last_error_unix_milliseconds", nil, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_policy_outbox_relay_last_success_unix_milliseconds", nil, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_policy_outbox_relay_last_published_unix_milliseconds", nil, snapshot.LastPublishedAtMS)
	writePrometheusSample(builder, "nexusim_policy_outbox_relay_last_error_backoff_milliseconds", nil, snapshot.LastErrorBackoffMS)
}

func writeTracePrometheus(builder *strings.Builder, snapshot *TraceSnapshot) {
	writePrometheusHeader(builder, "nexusim_policy_otel_traces_enabled", "Policy service OpenTelemetry trace enabled flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_otel_traces_otlp_endpoint_configured", "Policy service OpenTelemetry OTLP endpoint configured flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_otel_traces_otlp_insecure", "Policy service OpenTelemetry OTLP insecure flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_policy_otel_traces_sampling_ratio", "Policy service OpenTelemetry trace sampling ratio.", "gauge")
	if snapshot == nil {
		return
	}
	exporter := snapshot.Exporter
	if strings.TrimSpace(exporter) == "" {
		exporter = "disabled"
	}
	labels := map[string]string{"exporter": exporter}
	writePrometheusSample(builder, "nexusim_policy_otel_traces_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusSample(builder, "nexusim_policy_otel_traces_otlp_endpoint_configured", labels, prometheusBool(snapshot.OTLPEndpointSet))
	writePrometheusSample(builder, "nexusim_policy_otel_traces_otlp_insecure", labels, prometheusBool(snapshot.OTLPInsecure))
	writePrometheusSample(builder, "nexusim_policy_otel_traces_sampling_ratio", labels, snapshot.SamplingRatio)
}

func writePrometheusHeader(builder *strings.Builder, name string, help string, metricType string) {
	builder.WriteString("# HELP ")
	builder.WriteString(name)
	builder.WriteByte(' ')
	builder.WriteString(help)
	builder.WriteByte('\n')
	builder.WriteString("# TYPE ")
	builder.WriteString(name)
	builder.WriteByte(' ')
	builder.WriteString(metricType)
	builder.WriteByte('\n')
}

func writePrometheusSample(builder *strings.Builder, name string, labels map[string]string, value any) {
	builder.WriteString(name)
	if len(labels) > 0 {
		builder.WriteByte('{')
		keys := sortedKeys(labels)
		for i, key := range keys {
			if i > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(key)
			builder.WriteString("=\"")
			builder.WriteString(prometheusEscapeLabelValue(labels[key]))
			builder.WriteByte('"')
		}
		builder.WriteByte('}')
	}
	builder.WriteByte(' ')
	builder.WriteString(prometheusFloat(value))
	builder.WriteByte('\n')
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func prometheusEscapeLabelValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
	return replacer.Replace(value)
}

func prometheusFloat(value any) string {
	switch typed := value.(type) {
	case int:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int64:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case uint64:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return "0"
	}
}

func prometheusBool(value bool) int {
	if value {
		return 1
	}
	return 0
}
