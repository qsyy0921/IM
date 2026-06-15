package monitoring

import (
	"sort"
	"strconv"
	"strings"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func renderPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writePrometheusHeader(&builder, "nexusim_conversation_build_info", "Conversation service build marker.", "gauge")
	writePrometheusSample(&builder, "nexusim_conversation_build_info", map[string]string{"service": serviceName}, 1)

	writeGRPCPrometheus(&builder, snapshot.GRPC)
	writePGPoolPrometheus(&builder, snapshot.PGPool)
	writeConversationPrometheus(&builder, snapshot.Conversation, snapshot.ConversationError)
	writeMemberChangeWorkerPrometheus(&builder, snapshot.MemberChangeWorker)
	writeTracePrometheus(&builder, snapshot.Trace)
	return builder.String()
}

func writeGRPCPrometheus(builder *strings.Builder, snapshot *GRPCSnapshot) {
	writePrometheusHeader(builder, "nexusim_conversation_grpc_method_requests_total", "Conversation service gRPC requests by method.", "counter")
	writePrometheusHeader(builder, "nexusim_conversation_grpc_requests_total", "Conversation service gRPC requests by method and code.", "counter")
	writePrometheusHeader(builder, "nexusim_conversation_grpc_method_errors_total", "Conversation service gRPC errors by method.", "counter")
	writePrometheusHeader(builder, "nexusim_conversation_grpc_latency_avg_milliseconds", "Conversation service average gRPC latency by method.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_grpc_latency_max_milliseconds", "Conversation service max gRPC latency by method.", "gauge")
	if snapshot == nil {
		return
	}
	methods := append([]GRPCMethodSnapshot(nil), snapshot.Methods...)
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Method < methods[j].Method
	})
	for _, method := range methods {
		labels := map[string]string{"method": method.Method}
		writePrometheusSample(builder, "nexusim_conversation_grpc_method_requests_total", labels, method.Count)
		writePrometheusSample(builder, "nexusim_conversation_grpc_method_errors_total", labels, method.ErrorCount)
		writePrometheusSample(builder, "nexusim_conversation_grpc_latency_avg_milliseconds", labels, method.LatencyAvgMS)
		writePrometheusSample(builder, "nexusim_conversation_grpc_latency_max_milliseconds", labels, method.LatencyMaxMS)
		codes := sortedKeys(method.Codes)
		for _, code := range codes {
			writePrometheusSample(builder, "nexusim_conversation_grpc_requests_total", map[string]string{
				"method": method.Method,
				"code":   code,
			}, method.Codes[code])
		}
	}
}

func writePGPoolPrometheus(builder *strings.Builder, snapshot *PGPoolSnapshot) {
	writePrometheusHeader(builder, "nexusim_conversation_pg_pool_acquire_total", "Conversation service PostgreSQL pool acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_conversation_pg_pool_acquire_duration_milliseconds_total", "Conversation service PostgreSQL pool acquire duration total.", "counter")
	writePrometheusHeader(builder, "nexusim_conversation_pg_pool_canceled_acquire_total", "Conversation service PostgreSQL pool canceled acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_conversation_pg_pool_empty_acquire_total", "Conversation service PostgreSQL pool empty acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_conversation_pg_pool_conns", "Conversation service PostgreSQL pool connection counts.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_conversation_pg_pool_acquire_total", nil, snapshot.AcquireCount)
	writePrometheusSample(builder, "nexusim_conversation_pg_pool_acquire_duration_milliseconds_total", nil, snapshot.AcquireDurationMS)
	writePrometheusSample(builder, "nexusim_conversation_pg_pool_canceled_acquire_total", nil, snapshot.CanceledAcquireCount)
	writePrometheusSample(builder, "nexusim_conversation_pg_pool_empty_acquire_total", nil, snapshot.EmptyAcquireCount)
	writePrometheusSample(builder, "nexusim_conversation_pg_pool_conns", map[string]string{"state": "acquired"}, snapshot.AcquiredConns)
	writePrometheusSample(builder, "nexusim_conversation_pg_pool_conns", map[string]string{"state": "constructing"}, snapshot.ConstructingConns)
	writePrometheusSample(builder, "nexusim_conversation_pg_pool_conns", map[string]string{"state": "idle"}, snapshot.IdleConns)
	writePrometheusSample(builder, "nexusim_conversation_pg_pool_conns", map[string]string{"state": "max"}, snapshot.MaxConns)
	writePrometheusSample(builder, "nexusim_conversation_pg_pool_conns", map[string]string{"state": "total"}, snapshot.TotalConns)
}

func writeConversationPrometheus(builder *strings.Builder, snapshot *ConversationSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_conversation_metrics_query_error", "Conversation service metrics query error state.", "gauge")
	queryErrorValue := 0
	if queryError != "" {
		queryErrorValue = 1
	}
	writePrometheusSample(builder, "nexusim_conversation_metrics_query_error", nil, queryErrorValue)

	writePrometheusHeader(builder, "nexusim_conversation_conversations", "Conversation rows by fixed state.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_conversations_by_type", "Conversation rows by type.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_conversations_by_status", "Conversation rows by status.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_members", "Conversation member rows by fixed state.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_members_by_role", "Conversation member rows by role.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_members_by_status", "Conversation member rows by status.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_members_by_role_status", "Conversation member rows by role and status.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_member_changes", "Member change saga rows by fixed state.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_member_changes_by_status", "Member change saga rows by status.", "gauge")
	if snapshot == nil {
		return
	}
	if snapshot.Conversations != nil {
		writePrometheusSample(builder, "nexusim_conversation_conversations", map[string]string{"state": "total"}, snapshot.Conversations.Total)
		writePrometheusSample(builder, "nexusim_conversation_conversations", map[string]string{"state": "active"}, snapshot.Conversations.Active)
		writePrometheusSample(builder, "nexusim_conversation_conversations", map[string]string{"state": "archived"}, snapshot.Conversations.Archived)
		writePrometheusSample(builder, "nexusim_conversation_conversations", map[string]string{"state": "deleted"}, snapshot.Conversations.Deleted)
		writeGroupCountPrometheus(builder, "nexusim_conversation_conversations_by_type", "type", snapshot.Conversations.ByType)
		writeGroupCountPrometheus(builder, "nexusim_conversation_conversations_by_status", "status", snapshot.Conversations.ByStatus)
	}
	if snapshot.Members != nil {
		writePrometheusSample(builder, "nexusim_conversation_members", map[string]string{"state": "total"}, snapshot.Members.Total)
		writePrometheusSample(builder, "nexusim_conversation_members", map[string]string{"state": "active"}, snapshot.Members.Active)
		writePrometheusSample(builder, "nexusim_conversation_members", map[string]string{"state": "left"}, snapshot.Members.Left)
		writePrometheusSample(builder, "nexusim_conversation_members", map[string]string{"state": "banned"}, snapshot.Members.Banned)
		writeGroupCountPrometheus(builder, "nexusim_conversation_members_by_role", "role", snapshot.Members.ByRole)
		writeGroupCountPrometheus(builder, "nexusim_conversation_members_by_status", "status", snapshot.Members.ByStatus)
		roleStatusCounts := append([]RoleStatusCount(nil), snapshot.Members.ByRoleStatus...)
		sort.Slice(roleStatusCounts, func(i, j int) bool {
			if roleStatusCounts[i].Role == roleStatusCounts[j].Role {
				return roleStatusCounts[i].Status < roleStatusCounts[j].Status
			}
			return roleStatusCounts[i].Role < roleStatusCounts[j].Role
		})
		for _, entry := range roleStatusCounts {
			writePrometheusSample(builder, "nexusim_conversation_members_by_role_status", map[string]string{
				"role":   entry.Role,
				"status": entry.Status,
			}, entry.Total)
		}
	}
	if snapshot.MemberChanges != nil {
		writePrometheusSample(builder, "nexusim_conversation_member_changes", map[string]string{"state": "total"}, snapshot.MemberChanges.Total)
		writePrometheusSample(builder, "nexusim_conversation_member_changes", map[string]string{"state": "pending_boundary"}, snapshot.MemberChanges.PendingBoundary)
		writePrometheusSample(builder, "nexusim_conversation_member_changes", map[string]string{"state": "boundary_allocated"}, snapshot.MemberChanges.BoundaryAllocated)
		writePrometheusSample(builder, "nexusim_conversation_member_changes", map[string]string{"state": "member_updated"}, snapshot.MemberChanges.MemberUpdated)
		writePrometheusSample(builder, "nexusim_conversation_member_changes", map[string]string{"state": "outbox_enqueued"}, snapshot.MemberChanges.OutboxEnqueued)
		writePrometheusSample(builder, "nexusim_conversation_member_changes", map[string]string{"state": "event_published"}, snapshot.MemberChanges.EventPublished)
		writePrometheusSample(builder, "nexusim_conversation_member_changes", map[string]string{"state": "done"}, snapshot.MemberChanges.Done)
		writePrometheusSample(builder, "nexusim_conversation_member_changes", map[string]string{"state": "failed_compensated"}, snapshot.MemberChanges.FailedCompensated)
		writeGroupCountPrometheus(builder, "nexusim_conversation_member_changes_by_status", "status", snapshot.MemberChanges.ByStatus)
	}
}

func writeGroupCountPrometheus(builder *strings.Builder, metric string, label string, counts []GroupCountSnapshot) {
	entries := append([]GroupCountSnapshot(nil), counts...)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Value < entries[j].Value
	})
	for _, entry := range entries {
		writePrometheusSample(builder, metric, map[string]string{label: entry.Value}, entry.Total)
	}
}

func writeMemberChangeWorkerPrometheus(builder *strings.Builder, snapshot *types.MemberChangeWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_conversation_member_change_worker_errors_total", "Conversation member-change worker errors.", "counter")
	writePrometheusHeader(builder, "nexusim_conversation_member_change_worker_consecutive_errors", "Conversation member-change worker consecutive errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_member_change_worker_last_error_unix_milliseconds", "Conversation member-change worker last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_member_change_worker_last_success_unix_milliseconds", "Conversation member-change worker last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_member_change_worker_last_advanced_unix_milliseconds", "Conversation member-change worker last advancement timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_member_change_worker_last_advanced_count", "Conversation member-change worker last advanced count.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_member_change_worker_last_error_backoff_milliseconds", "Conversation member-change worker error backoff.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_member_change_worker_poll_interval_milliseconds", "Conversation member-change worker poll interval.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_conversation_member_change_worker_errors_total", nil, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_conversation_member_change_worker_consecutive_errors", nil, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_conversation_member_change_worker_last_error_unix_milliseconds", nil, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_conversation_member_change_worker_last_success_unix_milliseconds", nil, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_conversation_member_change_worker_last_advanced_unix_milliseconds", nil, snapshot.LastAdvancedAtMS)
	writePrometheusSample(builder, "nexusim_conversation_member_change_worker_last_advanced_count", nil, snapshot.LastAdvancedCount)
	writePrometheusSample(builder, "nexusim_conversation_member_change_worker_last_error_backoff_milliseconds", nil, snapshot.LastErrorBackoffMS)
	writePrometheusSample(builder, "nexusim_conversation_member_change_worker_poll_interval_milliseconds", nil, snapshot.LastPollIntervalMS)
}

func writeTracePrometheus(builder *strings.Builder, snapshot *TraceSnapshot) {
	writePrometheusHeader(builder, "nexusim_conversation_otel_traces_enabled", "Conversation service OpenTelemetry trace enabled flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_otel_traces_otlp_endpoint_configured", "Conversation service OpenTelemetry OTLP endpoint configured flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_otel_traces_otlp_insecure", "Conversation service OpenTelemetry OTLP insecure flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_conversation_otel_traces_sampling_ratio", "Conversation service OpenTelemetry trace sampling ratio.", "gauge")
	if snapshot == nil {
		return
	}
	exporter := snapshot.Exporter
	if strings.TrimSpace(exporter) == "" {
		exporter = "disabled"
	}
	labels := map[string]string{"exporter": exporter}
	writePrometheusSample(builder, "nexusim_conversation_otel_traces_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusSample(builder, "nexusim_conversation_otel_traces_otlp_endpoint_configured", labels, prometheusBool(snapshot.OTLPEndpointSet))
	writePrometheusSample(builder, "nexusim_conversation_otel_traces_otlp_insecure", labels, prometheusBool(snapshot.OTLPInsecure))
	writePrometheusSample(builder, "nexusim_conversation_otel_traces_sampling_ratio", labels, snapshot.SamplingRatio)
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
		for index, key := range keys {
			if index > 0 {
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

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func prometheusEscapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
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
