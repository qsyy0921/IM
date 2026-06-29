package monitoring

import (
	"sort"
	"strconv"
	"strings"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func renderPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writePrometheusHeader(&builder, "nexusim_delivery_build_info", "Delivery service build marker.", "gauge")
	writePrometheusSample(&builder, "nexusim_delivery_build_info", map[string]string{"service": serviceName}, 1)

	writeGRPCPrometheus(&builder, snapshot.GRPC)
	writePGPoolPrometheus(&builder, snapshot.PGPool)
	writeDeliveryPrometheus(&builder, snapshot.Delivery, snapshot.DeliveryError)
	writeDeliveryOutboxPrometheus(&builder, snapshot.DeliveryOutbox, snapshot.DeliveryOutboxError)
	writeProjectionFailurePrometheus(&builder, snapshot.ProjectionFailures, snapshot.ProjectionFailuresError)
	writeTimelineWorkerPrometheus(&builder, snapshot.TimelineProjectionWorker)
	writeOutboxRelayPrometheus(&builder, snapshot.OutboxRelay)
	writeTracePrometheus(&builder, snapshot.Trace)
	return builder.String()
}

func writeGRPCPrometheus(builder *strings.Builder, snapshot *GRPCSnapshot) {
	writePrometheusHeader(builder, "nexusim_delivery_grpc_method_requests_total", "Delivery service gRPC requests by method.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_grpc_requests_total", "Delivery service gRPC requests by method and code.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_grpc_method_errors_total", "Delivery service gRPC errors by method.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_grpc_latency_avg_milliseconds", "Delivery service average gRPC latency by method.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_grpc_latency_max_milliseconds", "Delivery service max gRPC latency by method.", "gauge")
	if snapshot == nil {
		return
	}
	methods := append([]GRPCMethodSnapshot(nil), snapshot.Methods...)
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Method < methods[j].Method
	})
	for _, method := range methods {
		labels := map[string]string{"method": method.Method}
		writePrometheusSample(builder, "nexusim_delivery_grpc_method_requests_total", labels, method.Count)
		writePrometheusSample(builder, "nexusim_delivery_grpc_method_errors_total", labels, method.ErrorCount)
		writePrometheusSample(builder, "nexusim_delivery_grpc_latency_avg_milliseconds", labels, method.LatencyAvgMS)
		writePrometheusSample(builder, "nexusim_delivery_grpc_latency_max_milliseconds", labels, method.LatencyMaxMS)
		codes := sortedKeys(method.Codes)
		for _, code := range codes {
			writePrometheusSample(builder, "nexusim_delivery_grpc_requests_total", map[string]string{
				"method": method.Method,
				"code":   code,
			}, method.Codes[code])
		}
	}
}

func writePGPoolPrometheus(builder *strings.Builder, snapshot *PGPoolSnapshot) {
	writePrometheusHeader(builder, "nexusim_delivery_pg_pool_acquire_total", "Delivery service PostgreSQL pool acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_pg_pool_acquire_duration_milliseconds_total", "Delivery service PostgreSQL pool acquire duration total.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_pg_pool_canceled_acquire_total", "Delivery service PostgreSQL pool canceled acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_pg_pool_empty_acquire_total", "Delivery service PostgreSQL pool empty acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_pg_pool_conns", "Delivery service PostgreSQL pool connection counts.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_delivery_pg_pool_acquire_total", nil, snapshot.AcquireCount)
	writePrometheusSample(builder, "nexusim_delivery_pg_pool_acquire_duration_milliseconds_total", nil, snapshot.AcquireDurationMS)
	writePrometheusSample(builder, "nexusim_delivery_pg_pool_canceled_acquire_total", nil, snapshot.CanceledAcquireCount)
	writePrometheusSample(builder, "nexusim_delivery_pg_pool_empty_acquire_total", nil, snapshot.EmptyAcquireCount)
	writePrometheusSample(builder, "nexusim_delivery_pg_pool_conns", map[string]string{"state": "acquired"}, snapshot.AcquiredConns)
	writePrometheusSample(builder, "nexusim_delivery_pg_pool_conns", map[string]string{"state": "constructing"}, snapshot.ConstructingConns)
	writePrometheusSample(builder, "nexusim_delivery_pg_pool_conns", map[string]string{"state": "idle"}, snapshot.IdleConns)
	writePrometheusSample(builder, "nexusim_delivery_pg_pool_conns", map[string]string{"state": "max"}, snapshot.MaxConns)
	writePrometheusSample(builder, "nexusim_delivery_pg_pool_conns", map[string]string{"state": "total"}, snapshot.TotalConns)
}

func writeDeliveryPrometheus(builder *strings.Builder, snapshot *DeliverySnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_delivery_metrics_query_error", "Delivery service read-model metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_read_model", "Delivery service durable read-model counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_membership_projection", "Delivery membership projection counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_kafka_checkpoints", "Delivery Kafka checkpoint counts by state.", "gauge")
	writePrometheusSample(builder, "nexusim_delivery_metrics_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_delivery_read_model", map[string]string{"state": "user_inbox_total"}, snapshot.UserInboxTotal)
	writePrometheusSample(builder, "nexusim_delivery_read_model", map[string]string{"state": "distinct_users"}, snapshot.UserInboxDistinctUsers)
	writePrometheusSample(builder, "nexusim_delivery_read_model", map[string]string{"state": "distinct_conversations"}, snapshot.UserInboxDistinctConversations)
	writePrometheusSample(builder, "nexusim_delivery_read_model", map[string]string{"state": "device_delivery_cursors"}, snapshot.DeviceDeliveryCursors)
	writePrometheusSample(builder, "nexusim_delivery_membership_projection", map[string]string{"state": "total"}, snapshot.MembershipProjectionTotal)
	writePrometheusSample(builder, "nexusim_delivery_membership_projection", map[string]string{"state": "active"}, snapshot.MembershipProjectionActive)
	writePrometheusSample(builder, "nexusim_delivery_membership_projection", map[string]string{"state": "inactive"}, snapshot.MembershipProjectionInactive)
	writePrometheusSample(builder, "nexusim_delivery_kafka_checkpoints", map[string]string{"state": "checkpoints"}, snapshot.KafkaCheckpoints)
	writePrometheusSample(builder, "nexusim_delivery_kafka_checkpoints", map[string]string{"state": "consumer_groups"}, snapshot.KafkaConsumerGroups)
}

func writeDeliveryOutboxPrometheus(builder *strings.Builder, snapshot *DeliveryOutboxSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_delivery_outbox_metrics_query_error", "Delivery outbox metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_outbox", "Delivery outbox rows by state.", "gauge")
	writePrometheusSample(builder, "nexusim_delivery_outbox_metrics_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_delivery_outbox", map[string]string{"state": "total"}, snapshot.Total)
	writePrometheusSample(builder, "nexusim_delivery_outbox", map[string]string{"state": "pending"}, snapshot.Pending)
	writePrometheusSample(builder, "nexusim_delivery_outbox", map[string]string{"state": "pending_ready"}, snapshot.PendingReady)
	writePrometheusSample(builder, "nexusim_delivery_outbox", map[string]string{"state": "pending_scheduled"}, snapshot.PendingScheduled)
	writePrometheusSample(builder, "nexusim_delivery_outbox", map[string]string{"state": "published"}, snapshot.Published)
	writePrometheusSample(builder, "nexusim_delivery_outbox", map[string]string{"state": "dlq"}, snapshot.DLQ)
	writePrometheusSample(builder, "nexusim_delivery_outbox", map[string]string{"state": "max_pending_retry"}, snapshot.MaxPendingRetry)
}

func writeProjectionFailurePrometheus(builder *strings.Builder, snapshot *ProjectionFailureSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_delivery_projection_failure_metrics_query_error", "Delivery projection failure metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_projection_failures", "Delivery projection failures by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_projection_failures_by_class", "Unresolved delivery projection failures by class.", "gauge")
	writePrometheusSample(builder, "nexusim_delivery_projection_failure_metrics_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_delivery_projection_failures", map[string]string{"state": "unresolved_total"}, snapshot.Total)
	writePrometheusSample(builder, "nexusim_delivery_projection_failures", map[string]string{"state": "resolved_total"}, snapshot.ResolvedTotal)
	writePrometheusSample(builder, "nexusim_delivery_projection_failures", map[string]string{"state": "max_failure_count"}, snapshot.MaxFailureCount)
	writePrometheusSample(builder, "nexusim_delivery_projection_failures_by_class", map[string]string{"class": "decode_failed"}, snapshot.DecodeFailed)
	writePrometheusSample(builder, "nexusim_delivery_projection_failures_by_class", map[string]string{"class": "invalid_argument"}, snapshot.InvalidArgument)
	writePrometheusSample(builder, "nexusim_delivery_projection_failures_by_class", map[string]string{"class": "projection_dependency"}, snapshot.ProjectionDependency)
	writePrometheusSample(builder, "nexusim_delivery_projection_failures_by_class", map[string]string{"class": "db_read_failed"}, snapshot.DBReadFailed)
	writePrometheusSample(builder, "nexusim_delivery_projection_failures_by_class", map[string]string{"class": "db_write_failed"}, snapshot.DBWriteFailed)
	writePrometheusSample(builder, "nexusim_delivery_projection_failures_by_class", map[string]string{"class": "unknown"}, snapshot.Unknown)
}

func writeTimelineWorkerPrometheus(builder *strings.Builder, snapshot *types.ProjectionWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_delivery_timeline_worker_errors_total", "Delivery timeline projection worker errors.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_timeline_worker_consecutive_errors", "Delivery timeline projection worker consecutive errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_timeline_worker_last_error_unix_milliseconds", "Delivery timeline projection worker last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_timeline_worker_last_success_unix_milliseconds", "Delivery timeline projection worker last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_timeline_worker_last_commit_unix_milliseconds", "Delivery timeline projection worker last commit timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_timeline_worker_last_error_backoff_milliseconds", "Delivery timeline projection worker error backoff.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_delivery_timeline_worker_errors_total", nil, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_delivery_timeline_worker_consecutive_errors", nil, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_delivery_timeline_worker_last_error_unix_milliseconds", nil, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_delivery_timeline_worker_last_success_unix_milliseconds", nil, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_delivery_timeline_worker_last_commit_unix_milliseconds", nil, snapshot.LastCommitAtMS)
	writePrometheusSample(builder, "nexusim_delivery_timeline_worker_last_error_backoff_milliseconds", nil, snapshot.LastErrorBackoffMS)
}

func writeOutboxRelayPrometheus(builder *strings.Builder, snapshot *types.OutboxRelayWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_errors_total", "Delivery outbox relay errors.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_fetched_total", "Delivery outbox relay fetched rows.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_published_total", "Delivery outbox relay published rows.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_retried_total", "Delivery outbox relay retried rows.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_dead_lettered_total", "Delivery outbox relay dead-lettered rows.", "counter")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_consecutive_errors", "Delivery outbox relay consecutive errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_workers", "Delivery outbox relay worker count.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_last_run_duration_milliseconds", "Delivery outbox relay last run duration.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_last_publish_duration_milliseconds", "Delivery outbox relay last Kafka publish duration.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_last_error_unix_milliseconds", "Delivery outbox relay last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_last_success_unix_milliseconds", "Delivery outbox relay last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_last_published_unix_milliseconds", "Delivery outbox relay last publish timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_outbox_relay_last_error_backoff_milliseconds", "Delivery outbox relay error backoff.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_errors_total", nil, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_fetched_total", nil, snapshot.TotalFetched)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_published_total", nil, snapshot.TotalPublished)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_retried_total", nil, snapshot.TotalRetried)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_dead_lettered_total", nil, snapshot.TotalDeadLettered)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_consecutive_errors", nil, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_workers", nil, snapshot.WorkerCount)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_last_run_duration_milliseconds", nil, snapshot.LastRunDurationMS)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_last_publish_duration_milliseconds", nil, snapshot.LastPublishMS)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_last_error_unix_milliseconds", nil, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_last_success_unix_milliseconds", nil, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_last_published_unix_milliseconds", nil, snapshot.LastPublishedAtMS)
	writePrometheusSample(builder, "nexusim_delivery_outbox_relay_last_error_backoff_milliseconds", nil, snapshot.LastErrorBackoffMS)
}

func writeTracePrometheus(builder *strings.Builder, snapshot *TraceSnapshot) {
	writePrometheusHeader(builder, "nexusim_delivery_otel_traces_enabled", "Delivery service OpenTelemetry trace enabled flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_otel_traces_otlp_endpoint_configured", "Delivery service OpenTelemetry OTLP endpoint configured flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_otel_traces_otlp_insecure", "Delivery service OpenTelemetry OTLP insecure flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_delivery_otel_traces_sampling_ratio", "Delivery service OpenTelemetry trace sampling ratio.", "gauge")
	if snapshot == nil {
		return
	}
	exporter := snapshot.Exporter
	if strings.TrimSpace(exporter) == "" {
		exporter = "disabled"
	}
	labels := map[string]string{"exporter": exporter}
	writePrometheusSample(builder, "nexusim_delivery_otel_traces_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusSample(builder, "nexusim_delivery_otel_traces_otlp_endpoint_configured", labels, prometheusBool(snapshot.OTLPEndpointSet))
	writePrometheusSample(builder, "nexusim_delivery_otel_traces_otlp_insecure", labels, prometheusBool(snapshot.OTLPInsecure))
	writePrometheusSample(builder, "nexusim_delivery_otel_traces_sampling_ratio", labels, snapshot.SamplingRatio)
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
