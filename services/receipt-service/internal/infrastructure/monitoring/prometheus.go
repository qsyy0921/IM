package monitoring

import (
	"sort"
	"strconv"
	"strings"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func renderPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writePrometheusHeader(&builder, "nexusim_receipt_build_info", "Receipt service build marker.", "gauge")
	writePrometheusSample(&builder, "nexusim_receipt_build_info", map[string]string{"service": serviceName}, 1)

	writeGRPCPrometheus(&builder, snapshot.GRPC)
	writePGPoolPrometheus(&builder, snapshot.PGPool)
	writeReceiptPrometheus(&builder, snapshot.Receipt, snapshot.ReceiptError)
	writeOutboxPrometheus(&builder, snapshot.Outbox, snapshot.OutboxError)
	writeDeliveryProjectionWorkerPrometheus(&builder, snapshot.DeliveryProjectionWorker)
	writeOutboxRelayPrometheus(&builder, snapshot.OutboxRelay)
	writeTracePrometheus(&builder, snapshot.Trace)
	return builder.String()
}

func writeGRPCPrometheus(builder *strings.Builder, snapshot *GRPCSnapshot) {
	writePrometheusHeader(builder, "nexusim_receipt_grpc_method_requests_total", "Receipt service gRPC requests by method.", "counter")
	writePrometheusHeader(builder, "nexusim_receipt_grpc_requests_total", "Receipt service gRPC requests by method and code.", "counter")
	writePrometheusHeader(builder, "nexusim_receipt_grpc_method_errors_total", "Receipt service gRPC errors by method.", "counter")
	writePrometheusHeader(builder, "nexusim_receipt_grpc_latency_avg_milliseconds", "Receipt service average gRPC latency by method.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_grpc_latency_max_milliseconds", "Receipt service max gRPC latency by method.", "gauge")
	if snapshot == nil {
		return
	}
	methods := append([]GRPCMethodSnapshot(nil), snapshot.Methods...)
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Method < methods[j].Method
	})
	for _, method := range methods {
		labels := map[string]string{"method": method.Method}
		writePrometheusSample(builder, "nexusim_receipt_grpc_method_requests_total", labels, method.Count)
		writePrometheusSample(builder, "nexusim_receipt_grpc_method_errors_total", labels, method.ErrorCount)
		writePrometheusSample(builder, "nexusim_receipt_grpc_latency_avg_milliseconds", labels, method.LatencyAvgMS)
		writePrometheusSample(builder, "nexusim_receipt_grpc_latency_max_milliseconds", labels, method.LatencyMaxMS)
		for _, code := range sortedKeys(method.Codes) {
			writePrometheusSample(builder, "nexusim_receipt_grpc_requests_total", map[string]string{
				"method": method.Method,
				"code":   code,
			}, method.Codes[code])
		}
	}
}

func writePGPoolPrometheus(builder *strings.Builder, snapshot *PGPoolSnapshot) {
	writePrometheusHeader(builder, "nexusim_receipt_pg_pool_acquire_total", "Receipt service PostgreSQL pool acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_receipt_pg_pool_acquire_duration_milliseconds_total", "Receipt service PostgreSQL pool acquire duration total.", "counter")
	writePrometheusHeader(builder, "nexusim_receipt_pg_pool_canceled_acquire_total", "Receipt service PostgreSQL pool canceled acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_receipt_pg_pool_empty_acquire_total", "Receipt service PostgreSQL pool empty acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_receipt_pg_pool_conns", "Receipt service PostgreSQL pool connection counts.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_receipt_pg_pool_acquire_total", nil, snapshot.AcquireCount)
	writePrometheusSample(builder, "nexusim_receipt_pg_pool_acquire_duration_milliseconds_total", nil, snapshot.AcquireDurationMS)
	writePrometheusSample(builder, "nexusim_receipt_pg_pool_canceled_acquire_total", nil, snapshot.CanceledAcquireCount)
	writePrometheusSample(builder, "nexusim_receipt_pg_pool_empty_acquire_total", nil, snapshot.EmptyAcquireCount)
	writePrometheusSample(builder, "nexusim_receipt_pg_pool_conns", map[string]string{"state": "acquired"}, snapshot.AcquiredConns)
	writePrometheusSample(builder, "nexusim_receipt_pg_pool_conns", map[string]string{"state": "constructing"}, snapshot.ConstructingConns)
	writePrometheusSample(builder, "nexusim_receipt_pg_pool_conns", map[string]string{"state": "idle"}, snapshot.IdleConns)
	writePrometheusSample(builder, "nexusim_receipt_pg_pool_conns", map[string]string{"state": "max"}, snapshot.MaxConns)
	writePrometheusSample(builder, "nexusim_receipt_pg_pool_conns", map[string]string{"state": "total"}, snapshot.TotalConns)
}

func writeReceiptPrometheus(builder *strings.Builder, snapshot *ReceiptSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_receipt_metrics_query_error", "Receipt service projection metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_projection", "Receipt service projection counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_conversation_summary", "Receipt service conversation summary counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_kafka_checkpoints", "Receipt service Kafka checkpoint counts by state.", "gauge")
	writePrometheusSample(builder, "nexusim_receipt_metrics_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_receipt_projection", map[string]string{"state": "inbox_projection_total"}, snapshot.InboxProjectionTotal)
	writePrometheusSample(builder, "nexusim_receipt_projection", map[string]string{"state": "device_received_cursors"}, snapshot.DeviceReceivedCursors)
	writePrometheusSample(builder, "nexusim_receipt_projection", map[string]string{"state": "user_received_cursors"}, snapshot.UserReceivedCursors)
	writePrometheusSample(builder, "nexusim_receipt_projection", map[string]string{"state": "user_read_cursors"}, snapshot.UserReadCursors)
	writePrometheusSample(builder, "nexusim_receipt_projection", map[string]string{"state": "message_receipt_states"}, snapshot.MessageReceiptStates)
	writePrometheusSample(builder, "nexusim_receipt_conversation_summary", map[string]string{"state": "total"}, snapshot.ConversationSummaries)
	writePrometheusSample(builder, "nexusim_receipt_conversation_summary", map[string]string{"state": "archived"}, snapshot.ArchivedConversationCount)
	writePrometheusSample(builder, "nexusim_receipt_conversation_summary", map[string]string{"state": "pinned"}, snapshot.PinnedConversationCount)
	writePrometheusSample(builder, "nexusim_receipt_conversation_summary", map[string]string{"state": "muted"}, snapshot.MutedConversationCount)
	writePrometheusSample(builder, "nexusim_receipt_conversation_summary", map[string]string{"state": "unread"}, snapshot.UnreadConversationCount)
	writePrometheusSample(builder, "nexusim_receipt_kafka_checkpoints", map[string]string{"state": "checkpoints"}, snapshot.KafkaCheckpoints)
	writePrometheusSample(builder, "nexusim_receipt_kafka_checkpoints", map[string]string{"state": "consumer_groups"}, snapshot.KafkaConsumerGroups)
	writePrometheusSample(builder, "nexusim_receipt_kafka_checkpoints", map[string]string{"state": "conversation_list_checkpoints"}, snapshot.ConversationListCheckpoints)
}

func writeOutboxPrometheus(builder *strings.Builder, snapshot *OutboxSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_receipt_outbox_metrics_query_error", "Receipt outbox metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_outbox", "Receipt outbox rows by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_outbox_age_milliseconds", "Receipt outbox oldest row age by state.", "gauge")
	writePrometheusSample(builder, "nexusim_receipt_outbox_metrics_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_receipt_outbox", map[string]string{"state": "total"}, snapshot.Total)
	writePrometheusSample(builder, "nexusim_receipt_outbox", map[string]string{"state": "pending"}, snapshot.Pending)
	writePrometheusSample(builder, "nexusim_receipt_outbox", map[string]string{"state": "published"}, snapshot.Published)
	writePrometheusSample(builder, "nexusim_receipt_outbox", map[string]string{"state": "dlq"}, snapshot.DLQ)
	writePrometheusSample(builder, "nexusim_receipt_outbox", map[string]string{"state": "ready_pending"}, snapshot.ReadyPending)
	writeOptionalPrometheusSample(builder, "nexusim_receipt_outbox_age_milliseconds", map[string]string{"state": "oldest_pending"}, snapshot.OldestPendingAgeMS)
	writeOptionalPrometheusSample(builder, "nexusim_receipt_outbox_age_milliseconds", map[string]string{"state": "oldest_dlq"}, snapshot.OldestDLQAgeMS)
}

func writeDeliveryProjectionWorkerPrometheus(builder *strings.Builder, snapshot *types.ProjectionWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_receipt_delivery_projection_worker_errors_total", "Receipt delivery projection worker errors.", "counter")
	writePrometheusHeader(builder, "nexusim_receipt_delivery_projection_worker_consecutive_errors", "Receipt delivery projection worker consecutive errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_delivery_projection_worker_last_error_unix_milliseconds", "Receipt delivery projection worker last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_delivery_projection_worker_last_success_unix_milliseconds", "Receipt delivery projection worker last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_delivery_projection_worker_last_commit_unix_milliseconds", "Receipt delivery projection worker last commit timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_delivery_projection_worker_last_error_backoff_milliseconds", "Receipt delivery projection worker error backoff.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_receipt_delivery_projection_worker_errors_total", nil, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_receipt_delivery_projection_worker_consecutive_errors", nil, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_receipt_delivery_projection_worker_last_error_unix_milliseconds", nil, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_receipt_delivery_projection_worker_last_success_unix_milliseconds", nil, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_receipt_delivery_projection_worker_last_commit_unix_milliseconds", nil, snapshot.LastCommitAtMS)
	writePrometheusSample(builder, "nexusim_receipt_delivery_projection_worker_last_error_backoff_milliseconds", nil, snapshot.LastErrorBackoffMS)
}

func writeOutboxRelayPrometheus(builder *strings.Builder, snapshot *types.OutboxRelayWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_receipt_outbox_relay_errors_total", "Receipt outbox relay errors.", "counter")
	writePrometheusHeader(builder, "nexusim_receipt_outbox_relay_consecutive_errors", "Receipt outbox relay consecutive errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_outbox_relay_last_error_unix_milliseconds", "Receipt outbox relay last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_outbox_relay_last_success_unix_milliseconds", "Receipt outbox relay last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_outbox_relay_last_published_unix_milliseconds", "Receipt outbox relay last publish timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_outbox_relay_last_error_backoff_milliseconds", "Receipt outbox relay error backoff.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_receipt_outbox_relay_errors_total", nil, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_receipt_outbox_relay_consecutive_errors", nil, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_receipt_outbox_relay_last_error_unix_milliseconds", nil, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_receipt_outbox_relay_last_success_unix_milliseconds", nil, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_receipt_outbox_relay_last_published_unix_milliseconds", nil, snapshot.LastPublishedAtMS)
	writePrometheusSample(builder, "nexusim_receipt_outbox_relay_last_error_backoff_milliseconds", nil, snapshot.LastErrorBackoffMS)
}

func writeTracePrometheus(builder *strings.Builder, snapshot *TraceSnapshot) {
	writePrometheusHeader(builder, "nexusim_receipt_otel_traces_enabled", "Receipt service OpenTelemetry trace enabled flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_otel_traces_otlp_endpoint_configured", "Receipt service OpenTelemetry OTLP endpoint configured flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_otel_traces_otlp_insecure", "Receipt service OpenTelemetry OTLP insecure flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_receipt_otel_traces_sampling_ratio", "Receipt service OpenTelemetry trace sampling ratio.", "gauge")
	if snapshot == nil {
		return
	}
	exporter := snapshot.Exporter
	if strings.TrimSpace(exporter) == "" {
		exporter = "disabled"
	}
	labels := map[string]string{"exporter": exporter}
	writePrometheusSample(builder, "nexusim_receipt_otel_traces_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusSample(builder, "nexusim_receipt_otel_traces_otlp_endpoint_configured", labels, prometheusBool(snapshot.OTLPEndpointSet))
	writePrometheusSample(builder, "nexusim_receipt_otel_traces_otlp_insecure", labels, prometheusBool(snapshot.OTLPInsecure))
	writePrometheusSample(builder, "nexusim_receipt_otel_traces_sampling_ratio", labels, snapshot.SamplingRatio)
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

func writeOptionalPrometheusSample(builder *strings.Builder, name string, labels map[string]string, value *int64) {
	if value == nil {
		return
	}
	writePrometheusSample(builder, name, labels, *value)
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
