package monitoring

import (
	"sort"
	"strconv"
	"strings"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func renderPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writePrometheusHeader(&builder, "nexusim_contacts_build_info", "Contacts service build marker.", "gauge")
	writePrometheusSample(&builder, "nexusim_contacts_build_info", map[string]string{"service": serviceName}, 1)

	writeGRPCPrometheus(&builder, snapshot.GRPC)
	writePGPoolPrometheus(&builder, snapshot.PGPool)
	writeContactsPrometheus(&builder, snapshot.Contacts, snapshot.ContactsError)
	writeOutboxPrometheus(&builder, snapshot.Outbox, snapshot.OutboxError)
	writeOutboxRelayPrometheus(&builder, snapshot.OutboxRelay)
	writeTracePrometheus(&builder, snapshot.Trace)
	return builder.String()
}

func writeGRPCPrometheus(builder *strings.Builder, snapshot *GRPCSnapshot) {
	writePrometheusHeader(builder, "nexusim_contacts_grpc_method_requests_total", "Contacts service gRPC requests by method.", "counter")
	writePrometheusHeader(builder, "nexusim_contacts_grpc_requests_total", "Contacts service gRPC requests by method and code.", "counter")
	writePrometheusHeader(builder, "nexusim_contacts_grpc_method_errors_total", "Contacts service gRPC errors by method.", "counter")
	writePrometheusHeader(builder, "nexusim_contacts_grpc_latency_avg_milliseconds", "Contacts service average gRPC latency by method.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_grpc_latency_max_milliseconds", "Contacts service max gRPC latency by method.", "gauge")
	if snapshot == nil {
		return
	}
	methods := append([]GRPCMethodSnapshot(nil), snapshot.Methods...)
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Method < methods[j].Method
	})
	for _, method := range methods {
		labels := map[string]string{"method": method.Method}
		writePrometheusSample(builder, "nexusim_contacts_grpc_method_requests_total", labels, method.Count)
		writePrometheusSample(builder, "nexusim_contacts_grpc_method_errors_total", labels, method.ErrorCount)
		writePrometheusSample(builder, "nexusim_contacts_grpc_latency_avg_milliseconds", labels, method.LatencyAvgMS)
		writePrometheusSample(builder, "nexusim_contacts_grpc_latency_max_milliseconds", labels, method.LatencyMaxMS)
		for _, code := range sortedKeys(method.Codes) {
			writePrometheusSample(builder, "nexusim_contacts_grpc_requests_total", map[string]string{
				"method": method.Method,
				"code":   code,
			}, method.Codes[code])
		}
	}
}

func writePGPoolPrometheus(builder *strings.Builder, snapshot *PGPoolSnapshot) {
	writePrometheusHeader(builder, "nexusim_contacts_pg_pool_acquire_total", "Contacts service PostgreSQL pool acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_contacts_pg_pool_acquire_duration_milliseconds_total", "Contacts service PostgreSQL pool acquire duration total.", "counter")
	writePrometheusHeader(builder, "nexusim_contacts_pg_pool_canceled_acquire_total", "Contacts service PostgreSQL pool canceled acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_contacts_pg_pool_empty_acquire_total", "Contacts service PostgreSQL pool empty acquire count.", "counter")
	writePrometheusHeader(builder, "nexusim_contacts_pg_pool_conns", "Contacts service PostgreSQL pool connection counts.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_contacts_pg_pool_acquire_total", nil, snapshot.AcquireCount)
	writePrometheusSample(builder, "nexusim_contacts_pg_pool_acquire_duration_milliseconds_total", nil, snapshot.AcquireDurationMS)
	writePrometheusSample(builder, "nexusim_contacts_pg_pool_canceled_acquire_total", nil, snapshot.CanceledAcquireCount)
	writePrometheusSample(builder, "nexusim_contacts_pg_pool_empty_acquire_total", nil, snapshot.EmptyAcquireCount)
	writePrometheusSample(builder, "nexusim_contacts_pg_pool_conns", map[string]string{"state": "acquired"}, snapshot.AcquiredConns)
	writePrometheusSample(builder, "nexusim_contacts_pg_pool_conns", map[string]string{"state": "constructing"}, snapshot.ConstructingConns)
	writePrometheusSample(builder, "nexusim_contacts_pg_pool_conns", map[string]string{"state": "idle"}, snapshot.IdleConns)
	writePrometheusSample(builder, "nexusim_contacts_pg_pool_conns", map[string]string{"state": "max"}, snapshot.MaxConns)
	writePrometheusSample(builder, "nexusim_contacts_pg_pool_conns", map[string]string{"state": "total"}, snapshot.TotalConns)
}

func writeContactsPrometheus(builder *strings.Builder, snapshot *ContactsSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_contacts_metrics_query_error", "Contacts service metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_requests", "Contacts service contact request counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_request_status", "Contacts service contact request counts by stored status.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_edges", "Contacts service contact edge counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_edge_status", "Contacts service contact edge counts by stored status.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_command_idempotency_total", "Contacts service command idempotency row count.", "gauge")
	writePrometheusSample(builder, "nexusim_contacts_metrics_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	if snapshot.Requests != nil {
		writePrometheusSample(builder, "nexusim_contacts_requests", map[string]string{"state": "total"}, snapshot.Requests.Total)
		writePrometheusSample(builder, "nexusim_contacts_requests", map[string]string{"state": "pending"}, snapshot.Requests.Pending)
		writePrometheusSample(builder, "nexusim_contacts_requests", map[string]string{"state": "accepted"}, snapshot.Requests.Accepted)
		writePrometheusSample(builder, "nexusim_contacts_requests", map[string]string{"state": "declined"}, snapshot.Requests.Declined)
		writePrometheusSample(builder, "nexusim_contacts_requests", map[string]string{"state": "canceled"}, snapshot.Requests.Canceled)
		writePrometheusSample(builder, "nexusim_contacts_requests", map[string]string{"state": "expired"}, snapshot.Requests.Expired)
		for _, status := range snapshot.Requests.ByStatus {
			writePrometheusSample(builder, "nexusim_contacts_request_status", map[string]string{"status": status.Value}, status.Total)
		}
	}
	if snapshot.Edges != nil {
		writePrometheusSample(builder, "nexusim_contacts_edges", map[string]string{"state": "total"}, snapshot.Edges.Total)
		writePrometheusSample(builder, "nexusim_contacts_edges", map[string]string{"state": "active"}, snapshot.Edges.Active)
		writePrometheusSample(builder, "nexusim_contacts_edges", map[string]string{"state": "deleted"}, snapshot.Edges.Deleted)
		writePrometheusSample(builder, "nexusim_contacts_edges", map[string]string{"state": "blocked"}, snapshot.Edges.Blocked)
		writePrometheusSample(builder, "nexusim_contacts_edges", map[string]string{"state": "with_remark"}, snapshot.Edges.WithRemark)
		for _, status := range snapshot.Edges.ByStatus {
			writePrometheusSample(builder, "nexusim_contacts_edge_status", map[string]string{"status": status.Value}, status.Total)
		}
	}
	writePrometheusSample(builder, "nexusim_contacts_command_idempotency_total", nil, snapshot.CommandIdempotencyTotal)
}

func writeOutboxPrometheus(builder *strings.Builder, snapshot *OutboxSnapshot, queryError string) {
	writePrometheusHeader(builder, "nexusim_contacts_outbox_metrics_query_error", "Contacts outbox metrics query error state.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_outbox", "Contacts outbox rows by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_outbox_age_milliseconds", "Contacts outbox oldest row age by state.", "gauge")
	writePrometheusSample(builder, "nexusim_contacts_outbox_metrics_query_error", nil, prometheusBool(queryError != ""))
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_contacts_outbox", map[string]string{"state": "total"}, snapshot.Total)
	writePrometheusSample(builder, "nexusim_contacts_outbox", map[string]string{"state": "pending"}, snapshot.Pending)
	writePrometheusSample(builder, "nexusim_contacts_outbox", map[string]string{"state": "published"}, snapshot.Published)
	writePrometheusSample(builder, "nexusim_contacts_outbox", map[string]string{"state": "dlq"}, snapshot.DLQ)
	writePrometheusSample(builder, "nexusim_contacts_outbox", map[string]string{"state": "ready_pending"}, snapshot.ReadyPending)
	writeOptionalPrometheusSample(builder, "nexusim_contacts_outbox_age_milliseconds", map[string]string{"state": "oldest_pending"}, snapshot.OldestPendingAgeMS)
	writeOptionalPrometheusSample(builder, "nexusim_contacts_outbox_age_milliseconds", map[string]string{"state": "oldest_dlq"}, snapshot.OldestDLQAgeMS)
}

func writeOutboxRelayPrometheus(builder *strings.Builder, snapshot *types.OutboxRelayWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_contacts_outbox_relay_errors_total", "Contacts outbox relay errors.", "counter")
	writePrometheusHeader(builder, "nexusim_contacts_outbox_relay_consecutive_errors", "Contacts outbox relay consecutive errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_outbox_relay_last_error_unix_milliseconds", "Contacts outbox relay last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_outbox_relay_last_success_unix_milliseconds", "Contacts outbox relay last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_outbox_relay_last_published_unix_milliseconds", "Contacts outbox relay last publish timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_outbox_relay_last_error_backoff_milliseconds", "Contacts outbox relay error backoff.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_contacts_outbox_relay_errors_total", nil, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_contacts_outbox_relay_consecutive_errors", nil, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_contacts_outbox_relay_last_error_unix_milliseconds", nil, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_contacts_outbox_relay_last_success_unix_milliseconds", nil, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_contacts_outbox_relay_last_published_unix_milliseconds", nil, snapshot.LastPublishedAtMS)
	writePrometheusSample(builder, "nexusim_contacts_outbox_relay_last_error_backoff_milliseconds", nil, snapshot.LastErrorBackoffMS)
}

func writeTracePrometheus(builder *strings.Builder, snapshot *TraceSnapshot) {
	writePrometheusHeader(builder, "nexusim_contacts_otel_traces_enabled", "Contacts service OpenTelemetry trace enabled flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_otel_traces_otlp_endpoint_configured", "Contacts service OpenTelemetry OTLP endpoint configured flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_otel_traces_otlp_insecure", "Contacts service OpenTelemetry OTLP insecure flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_contacts_otel_traces_sampling_ratio", "Contacts service OpenTelemetry trace sampling ratio.", "gauge")
	if snapshot == nil {
		return
	}
	exporter := snapshot.Exporter
	if strings.TrimSpace(exporter) == "" {
		exporter = "disabled"
	}
	labels := map[string]string{"exporter": exporter}
	writePrometheusSample(builder, "nexusim_contacts_otel_traces_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusSample(builder, "nexusim_contacts_otel_traces_otlp_endpoint_configured", labels, prometheusBool(snapshot.OTLPEndpointSet))
	writePrometheusSample(builder, "nexusim_contacts_otel_traces_otlp_insecure", labels, prometheusBool(snapshot.OTLPInsecure))
	writePrometheusSample(builder, "nexusim_contacts_otel_traces_sampling_ratio", labels, snapshot.SamplingRatio)
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

func writeOptionalPrometheusSample(builder *strings.Builder, name string, labels map[string]string, value *int64) {
	if value == nil {
		return
	}
	writePrometheusSample(builder, name, labels, *value)
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
