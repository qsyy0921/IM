package monitoring

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func renderPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writePrometheusHeader(&builder, "nexusim_identity_metrics_generated_at_unix_milliseconds", "Gauge", "Unix time when the identity-service metrics snapshot was generated in milliseconds.")
	writePrometheusSample(&builder, "nexusim_identity_metrics_generated_at_unix_milliseconds", nil, strconv.FormatInt(snapshot.GeneratedAtMS, 10))
	writePrometheusHeader(&builder, "nexusim_identity_build_info", "Gauge", "Static identity-service information.")
	writePrometheusSample(&builder, "nexusim_identity_build_info", map[string]string{"service": snapshot.Service}, "1")

	if snapshot.PGPool != nil {
		writePGPoolPrometheus(&builder, *snapshot.PGPool)
	}
	if snapshot.Identity != nil {
		writeIdentityPrometheus(&builder, *snapshot.Identity)
	}
	if snapshot.ChallengeDeliveryOutbox != nil {
		writeChallengeDeliveryOutboxPrometheus(&builder, *snapshot.ChallengeDeliveryOutbox)
	}
	if snapshot.GRPC != nil {
		writeGRPCPrometheus(&builder, *snapshot.GRPC)
	}
	if snapshot.ChallengeDelivery != nil {
		writeChallengeDeliveryPrometheus(&builder, *snapshot.ChallengeDelivery)
	}
	if snapshot.ChallengeDeliveryWorker != nil {
		writeChallengeDeliveryWorkerPrometheus(&builder, *snapshot.ChallengeDeliveryWorker)
	}
	if snapshot.OutboxRelay != nil {
		writeOutboxRelayPrometheus(&builder, *snapshot.OutboxRelay)
	}
	if snapshot.Trace != nil {
		writeTracePrometheus(&builder, *snapshot.Trace)
	}
	return builder.String()
}

func writePGPoolPrometheus(builder *strings.Builder, snapshot PGPoolSnapshot) {
	writePrometheusHeader(builder, "nexusim_identity_pg_pool_acquire_total", "Counter", "Total identity-service PostgreSQL pool acquire count.")
	writePrometheusSample(builder, "nexusim_identity_pg_pool_acquire_total", nil, strconv.FormatInt(snapshot.AcquireCount, 10))
	writePrometheusHeader(builder, "nexusim_identity_pg_pool_acquire_duration_milliseconds_total", "Counter", "Total identity-service PostgreSQL pool acquire duration in milliseconds.")
	writePrometheusSample(builder, "nexusim_identity_pg_pool_acquire_duration_milliseconds_total", nil, strconv.FormatInt(snapshot.AcquireDurationMS, 10))
	writePrometheusHeader(builder, "nexusim_identity_pg_pool_canceled_acquire_total", "Counter", "Total identity-service PostgreSQL pool canceled acquire count.")
	writePrometheusSample(builder, "nexusim_identity_pg_pool_canceled_acquire_total", nil, strconv.FormatInt(snapshot.CanceledAcquireCount, 10))
	writePrometheusHeader(builder, "nexusim_identity_pg_pool_empty_acquire_total", "Counter", "Total identity-service PostgreSQL pool empty acquire count.")
	writePrometheusSample(builder, "nexusim_identity_pg_pool_empty_acquire_total", nil, strconv.FormatInt(snapshot.EmptyAcquireCount, 10))
	writePrometheusHeader(builder, "nexusim_identity_pg_pool_conns", "Gauge", "identity-service PostgreSQL pool connection counts by state.")
	writePrometheusSample(builder, "nexusim_identity_pg_pool_conns", map[string]string{"state": "acquired"}, strconv.FormatInt(int64(snapshot.AcquiredConns), 10))
	writePrometheusSample(builder, "nexusim_identity_pg_pool_conns", map[string]string{"state": "constructing"}, strconv.FormatInt(int64(snapshot.ConstructingConns), 10))
	writePrometheusSample(builder, "nexusim_identity_pg_pool_conns", map[string]string{"state": "idle"}, strconv.FormatInt(int64(snapshot.IdleConns), 10))
	writePrometheusSample(builder, "nexusim_identity_pg_pool_conns", map[string]string{"state": "total"}, strconv.FormatInt(int64(snapshot.TotalConns), 10))
	writePrometheusSample(builder, "nexusim_identity_pg_pool_conns", map[string]string{"state": "max"}, strconv.FormatInt(int64(snapshot.MaxConns), 10))
}

func writeIdentityPrometheus(builder *strings.Builder, snapshot IdentitySnapshot) {
	writePrometheusHeader(builder, "nexusim_identity_users", "Gauge", "Number of identity users.")
	writePrometheusSample(builder, "nexusim_identity_users", nil, strconv.FormatInt(snapshot.Users, 10))
	writePrometheusHeader(builder, "nexusim_identity_users_with_failures", "Gauge", "Number of identity users with password login failures.")
	writePrometheusSample(builder, "nexusim_identity_users_with_failures", nil, strconv.FormatInt(snapshot.UsersWithFailures, 10))
	writePrometheusHeader(builder, "nexusim_identity_password_login_locked", "Gauge", "Number of identity users currently password-login locked.")
	writePrometheusSample(builder, "nexusim_identity_password_login_locked", nil, strconv.FormatInt(snapshot.PasswordLoginLocked, 10))
	writePrometheusHeader(builder, "nexusim_identity_mfa_recovery_failures", "Gauge", "Number of identity users with MFA recovery-code failures.")
	writePrometheusSample(builder, "nexusim_identity_mfa_recovery_failures", nil, strconv.FormatInt(snapshot.MFARecoveryFailures, 10))
	writePrometheusHeader(builder, "nexusim_identity_mfa_recovery_locked", "Gauge", "Number of identity users currently MFA recovery-code locked.")
	writePrometheusSample(builder, "nexusim_identity_mfa_recovery_locked", nil, strconv.FormatInt(snapshot.MFARecoveryLocked, 10))
	writePrometheusHeader(builder, "nexusim_identity_mfa_factors", "Gauge", "Number of identity MFA factors.")
	writePrometheusSample(builder, "nexusim_identity_mfa_factors", nil, strconv.FormatInt(snapshot.MFAFactors, 10))
	writePrometheusHeader(builder, "nexusim_identity_mfa_factors_with_failures", "Gauge", "Number of identity MFA factors with login failures.")
	writePrometheusSample(builder, "nexusim_identity_mfa_factors_with_failures", nil, strconv.FormatInt(snapshot.MFAFactorsWithFailures, 10))
	writePrometheusHeader(builder, "nexusim_identity_mfa_login_locked", "Gauge", "Number of ACTIVE identity MFA factors currently login locked.")
	writePrometheusSample(builder, "nexusim_identity_mfa_login_locked", nil, strconv.FormatInt(snapshot.MFALoginLocked, 10))
	writePrometheusHeader(builder, "nexusim_identity_challenge_request_limits", "Gauge", "Number of identity challenge request limiter rows.")
	writePrometheusSample(builder, "nexusim_identity_challenge_request_limits", nil, strconv.FormatInt(snapshot.ChallengeRequestLimits, 10))
	writePrometheusHeader(builder, "nexusim_identity_challenge_request_limits_locked", "Gauge", "Number of identity challenge request limiter rows currently locked.")
	writePrometheusSample(builder, "nexusim_identity_challenge_request_limits_locked", nil, strconv.FormatInt(snapshot.ChallengeRequestLimitsLocked, 10))
	writePrometheusHeader(builder, "nexusim_identity_devices", "Gauge", "Number of identity devices by status.")
	writePrometheusSample(builder, "nexusim_identity_devices", map[string]string{"status": "active"}, strconv.FormatInt(snapshot.ActiveDevices, 10))
	writePrometheusSample(builder, "nexusim_identity_devices", map[string]string{"status": "revoked"}, strconv.FormatInt(snapshot.RevokedDevices, 10))
	writePrometheusHeader(builder, "nexusim_identity_sessions", "Gauge", "Number of identity sessions by status.")
	writePrometheusSample(builder, "nexusim_identity_sessions", map[string]string{"status": "active"}, strconv.FormatInt(snapshot.ActiveSessions, 10))
	writePrometheusSample(builder, "nexusim_identity_sessions", map[string]string{"status": "revoked"}, strconv.FormatInt(snapshot.RevokedSessions, 10))
	writePrometheusSample(builder, "nexusim_identity_sessions", map[string]string{"status": "expired"}, strconv.FormatInt(snapshot.ExpiredSessions, 10))
}

func writeChallengeDeliveryOutboxPrometheus(builder *strings.Builder, snapshot ChallengeDeliveryOutboxSnapshot) {
	writePrometheusHeader(builder, "nexusim_identity_challenge_delivery_outbox", "Gauge", "Number of identity challenge delivery outbox rows by status.")
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_outbox", map[string]string{"status": "total"}, strconv.FormatInt(snapshot.Total, 10))
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_outbox", map[string]string{"status": "pending"}, strconv.FormatInt(snapshot.Pending, 10))
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_outbox", map[string]string{"status": "pending_ready"}, strconv.FormatInt(snapshot.PendingReady, 10))
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_outbox", map[string]string{"status": "pending_scheduled"}, strconv.FormatInt(snapshot.PendingScheduled, 10))
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_outbox", map[string]string{"status": "pending_expired"}, strconv.FormatInt(snapshot.PendingExpired, 10))
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_outbox", map[string]string{"status": "delivered"}, strconv.FormatInt(snapshot.Delivered, 10))
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_outbox", map[string]string{"status": "dlq"}, strconv.FormatInt(snapshot.DLQ, 10))
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_outbox", map[string]string{"status": "canceled"}, strconv.FormatInt(snapshot.Canceled, 10))
	writePrometheusHeader(builder, "nexusim_identity_challenge_delivery_outbox_max_pending_retry", "Gauge", "Maximum retry count among PENDING identity challenge delivery outbox rows.")
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_outbox_max_pending_retry", nil, strconv.FormatInt(snapshot.MaxPendingRetry, 10))
	writePrometheusHeader(builder, "nexusim_identity_challenge_delivery_outbox_failure_classes", "Gauge", "Number of identity challenge delivery outbox rows by low-sensitive failure class.")
	writeSortedInt64MapPrometheus(builder, "nexusim_identity_challenge_delivery_outbox_failure_classes", "failure_class", snapshot.FailureClasses)
}

func writeGRPCPrometheus(builder *strings.Builder, snapshot GRPCSnapshot) {
	writePrometheusHeader(builder, "nexusim_identity_grpc_requests_total", "Counter", "Total identity-service gRPC requests by method and status code.")
	writePrometheusHeader(builder, "nexusim_identity_grpc_errors_total", "Counter", "Total identity-service non-OK gRPC requests by method.")
	writePrometheusHeader(builder, "nexusim_identity_grpc_latency_avg_milliseconds", "Gauge", "Average identity-service gRPC request latency by method.")
	writePrometheusHeader(builder, "nexusim_identity_grpc_latency_max_milliseconds", "Gauge", "Maximum identity-service gRPC request latency by method.")
	methods := append([]GRPCMethodSnapshot(nil), snapshot.Methods...)
	sort.Slice(methods, func(i, j int) bool { return methods[i].Method < methods[j].Method })
	for _, method := range methods {
		codes := make([]string, 0, len(method.Codes))
		for code := range method.Codes {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			writePrometheusSample(builder, "nexusim_identity_grpc_requests_total", map[string]string{"method": method.Method, "code": code}, strconv.FormatInt(method.Codes[code], 10))
		}
		writePrometheusSample(builder, "nexusim_identity_grpc_errors_total", map[string]string{"method": method.Method}, strconv.FormatInt(method.ErrorCount, 10))
		writePrometheusSample(builder, "nexusim_identity_grpc_latency_avg_milliseconds", map[string]string{"method": method.Method}, strconv.FormatInt(method.LatencyAvgMS, 10))
		writePrometheusSample(builder, "nexusim_identity_grpc_latency_max_milliseconds", map[string]string{"method": method.Method}, strconv.FormatInt(method.LatencyMaxMS, 10))
	}
}

func writeChallengeDeliveryPrometheus(builder *strings.Builder, snapshot ChallengeDeliverySnapshot) {
	labels := map[string]string{"mode": snapshot.Mode}
	writePrometheusHeader(builder, "nexusim_identity_challenge_delivery_requests_total", "Counter", "Total identity challenge delivery requests by outcome.")
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_requests_total", mergePrometheusLabels(labels, map[string]string{"outcome": "success"}), strconv.FormatInt(snapshot.SuccessCount, 10))
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_requests_total", mergePrometheusLabels(labels, map[string]string{"outcome": "failure"}), strconv.FormatInt(snapshot.FailureCount, 10))
	writePrometheusHeader(builder, "nexusim_identity_challenge_delivery_failure_classes_total", "Counter", "Total identity challenge delivery failures by low-sensitive class.")
	writeSortedInt64MapPrometheusWithBase(builder, "nexusim_identity_challenge_delivery_failure_classes_total", labels, "failure_class", snapshot.FailureClasses)
	writePrometheusHeader(builder, "nexusim_identity_challenge_delivery_latency_avg_milliseconds", "Gauge", "Average identity challenge delivery latency in milliseconds.")
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_latency_avg_milliseconds", labels, strconv.FormatInt(snapshot.LatencyAvgMS, 10))
	writePrometheusHeader(builder, "nexusim_identity_challenge_delivery_latency_max_milliseconds", "Gauge", "Maximum identity challenge delivery latency in milliseconds.")
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_latency_max_milliseconds", labels, strconv.FormatInt(snapshot.LatencyMaxMS, 10))
	writePrometheusHeader(builder, "nexusim_identity_challenge_delivery_last_success_unix_milliseconds", "Gauge", "Unix time of the last successful identity challenge delivery in milliseconds.")
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_last_success_unix_milliseconds", labels, strconv.FormatInt(snapshot.LastSuccessUnixMS, 10))
	writePrometheusHeader(builder, "nexusim_identity_challenge_delivery_last_failure_unix_milliseconds", "Gauge", "Unix time of the last failed identity challenge delivery in milliseconds.")
	writePrometheusSample(builder, "nexusim_identity_challenge_delivery_last_failure_unix_milliseconds", labels, strconv.FormatInt(snapshot.LastFailureUnixMS, 10))
}

func writeChallengeDeliveryWorkerPrometheus(builder *strings.Builder, snapshot types.ChallengeDeliveryWorkerSnapshot) {
	writeWorkerPrometheus(builder, "nexusim_identity_challenge_delivery_worker", snapshot.TotalErrors, snapshot.ConsecutiveErrors, snapshot.LastErrorAtMS, snapshot.LastSuccessAtMS, 0, snapshot.LastErrorBackoffMS)
}

func writeOutboxRelayPrometheus(builder *strings.Builder, snapshot types.OutboxRelayWorkerSnapshot) {
	writeWorkerPrometheus(builder, "nexusim_identity_outbox_relay", snapshot.TotalErrors, snapshot.ConsecutiveErrors, snapshot.LastErrorAtMS, snapshot.LastSuccessAtMS, snapshot.LastPublishedAtMS, snapshot.LastErrorBackoffMS)
}

func writeWorkerPrometheus(builder *strings.Builder, prefix string, totalErrors uint64, consecutiveErrors uint64, lastErrorAtMS int64, lastSuccessAtMS int64, lastPublishedAtMS int64, lastBackoffMS int64) {
	writePrometheusHeader(builder, prefix+"_errors_total", "Counter", "Total worker errors.")
	writePrometheusSample(builder, prefix+"_errors_total", nil, strconv.FormatUint(totalErrors, 10))
	writePrometheusHeader(builder, prefix+"_consecutive_errors", "Gauge", "Current consecutive worker error count.")
	writePrometheusSample(builder, prefix+"_consecutive_errors", nil, strconv.FormatUint(consecutiveErrors, 10))
	writePrometheusHeader(builder, prefix+"_last_error_unix_milliseconds", "Gauge", "Unix time of the last worker error in milliseconds.")
	writePrometheusSample(builder, prefix+"_last_error_unix_milliseconds", nil, strconv.FormatInt(lastErrorAtMS, 10))
	writePrometheusHeader(builder, prefix+"_last_success_unix_milliseconds", "Gauge", "Unix time of the last worker success in milliseconds.")
	writePrometheusSample(builder, prefix+"_last_success_unix_milliseconds", nil, strconv.FormatInt(lastSuccessAtMS, 10))
	if lastPublishedAtMS > 0 {
		writePrometheusHeader(builder, prefix+"_last_published_unix_milliseconds", "Gauge", "Unix time of the last published worker item in milliseconds.")
		writePrometheusSample(builder, prefix+"_last_published_unix_milliseconds", nil, strconv.FormatInt(lastPublishedAtMS, 10))
	}
	writePrometheusHeader(builder, prefix+"_last_error_backoff_milliseconds", "Gauge", "Last worker error backoff duration in milliseconds.")
	writePrometheusSample(builder, prefix+"_last_error_backoff_milliseconds", nil, strconv.FormatInt(lastBackoffMS, 10))
}

func writeTracePrometheus(builder *strings.Builder, snapshot TraceSnapshot) {
	labels := map[string]string{"exporter": snapshot.Exporter}
	writePrometheusHeader(builder, "nexusim_identity_otel_traces_enabled", "Gauge", "Whether identity-service OpenTelemetry tracing is enabled.")
	writePrometheusSample(builder, "nexusim_identity_otel_traces_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusHeader(builder, "nexusim_identity_otel_traces_otlp_endpoint_configured", "Gauge", "Whether identity-service OpenTelemetry OTLP endpoint is configured.")
	writePrometheusSample(builder, "nexusim_identity_otel_traces_otlp_endpoint_configured", labels, prometheusBool(snapshot.OTLPEndpointSet))
	writePrometheusHeader(builder, "nexusim_identity_otel_traces_otlp_insecure", "Gauge", "Whether identity-service OpenTelemetry OTLP transport is configured as insecure.")
	writePrometheusSample(builder, "nexusim_identity_otel_traces_otlp_insecure", labels, prometheusBool(snapshot.OTLPInsecure))
	writePrometheusHeader(builder, "nexusim_identity_otel_traces_sampling_ratio", "Gauge", "identity-service OpenTelemetry trace sampling ratio.")
	writePrometheusSample(builder, "nexusim_identity_otel_traces_sampling_ratio", labels, strconv.FormatFloat(snapshot.SamplingRatio, 'f', -1, 64))
}

func writeSortedInt64MapPrometheus(builder *strings.Builder, name string, labelKey string, values map[string]int64) {
	writeSortedInt64MapPrometheusWithBase(builder, name, nil, labelKey, values)
}

func writeSortedInt64MapPrometheusWithBase(builder *strings.Builder, name string, baseLabels map[string]string, labelKey string, values map[string]int64) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		labels := mergePrometheusLabels(baseLabels, map[string]string{labelKey: key})
		writePrometheusSample(builder, name, labels, strconv.FormatInt(values[key], 10))
	}
}

func mergePrometheusLabels(first map[string]string, second map[string]string) map[string]string {
	labels := make(map[string]string, len(first)+len(second))
	for key, value := range first {
		labels[key] = value
	}
	for key, value := range second {
		labels[key] = value
	}
	return labels
}

func writePrometheusHeader(builder *strings.Builder, name string, typ string, help string) {
	fmt.Fprintf(builder, "# HELP %s %s\n", name, help)
	fmt.Fprintf(builder, "# TYPE %s %s\n", name, strings.ToLower(typ))
}

func writePrometheusSample(builder *strings.Builder, name string, labels map[string]string, value string) {
	builder.WriteString(name)
	if len(labels) > 0 {
		builder.WriteString("{")
		keys := make([]string, 0, len(labels))
		for key := range labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for index, key := range keys {
			if index > 0 {
				builder.WriteString(",")
			}
			builder.WriteString(key)
			builder.WriteString("=\"")
			builder.WriteString(prometheusEscapeLabelValue(labels[key]))
			builder.WriteString("\"")
		}
		builder.WriteString("}")
	}
	builder.WriteString(" ")
	builder.WriteString(value)
	builder.WriteString("\n")
}

func prometheusEscapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}

func prometheusBool(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
