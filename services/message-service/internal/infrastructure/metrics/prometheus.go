package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	monitoringinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/monitoring"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func renderPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writePrometheusHeader(&builder, "nexusim_message_build_info", "Gauge", "Static message-service information.")
	writePrometheusSample(&builder, "nexusim_message_build_info", map[string]string{"service": "message-service"}, "1")
	writeLatencySnapshotsPrometheus(&builder, snapshot)
	writeValueSnapshotsPrometheus(&builder, snapshot)
	if snapshot.PGPool != nil {
		writePGPoolPrometheus(&builder, *snapshot.PGPool)
	}
	if snapshot.OutboxRelay != nil {
		writeOutboxRelayPrometheus(&builder, *snapshot.OutboxRelay)
	}
	if snapshot.Trace != nil {
		writeTracePrometheus(&builder, *snapshot.Trace)
	}
	return builder.String()
}

func writeLatencySnapshotsPrometheus(builder *strings.Builder, snapshot Snapshot) {
	latencies := map[string]LatencySnapshot{
		"send_message":                               snapshot.SendMessageLatencyMS,
		"send_message_recent":                        snapshot.SendMessageRecentLatencyMS,
		"send_message_command_build":                 snapshot.SendMessageCommandBuildLatencyMS,
		"send_message_command_build_recent":          snapshot.SendMessageCommandBuildRecentLatencyMS,
		"send_message_admission":                     snapshot.SendMessageAdmissionLatencyMS,
		"send_message_admission_recent":              snapshot.SendMessageAdmissionRecentLatencyMS,
		"send_message_dependency_read":               snapshot.SendMessageDependencyReadLatencyMS,
		"send_message_dependency_read_recent":        snapshot.SendMessageDependencyReadRecentLatencyMS,
		"send_message_conversation_context":          snapshot.SendMessageConversationLatencyMS,
		"send_message_conversation_context_recent":   snapshot.SendMessageConversationRecentLatencyMS,
		"send_message_policy_check":                  snapshot.SendMessagePolicyLatencyMS,
		"send_message_policy_check_recent":           snapshot.SendMessagePolicyRecentLatencyMS,
		"send_message_seq_floor":                     snapshot.SendMessageSeqFloorLatencyMS,
		"send_message_seq_floor_recent":              snapshot.SendMessageSeqFloorRecentLatencyMS,
		"send_message_sequencer_allocate":            snapshot.SendMessageSequencerLatencyMS,
		"send_message_sequencer_allocate_recent":     snapshot.SendMessageSequencerRecentLatencyMS,
		"send_message_repository_append_call":        snapshot.SendMessageRepositoryCallLatencyMS,
		"send_message_repository_append_call_recent": snapshot.SendMessageRepositoryCallRecentLatencyMS,
		"repository_append":                          snapshot.RepositoryAppendLatencyMS,
		"repository_append_recent":                   snapshot.RepositoryAppendRecentLatencyMS,
		"repository_begin":                           snapshot.RepositoryBeginLatencyMS,
		"repository_begin_recent":                    snapshot.RepositoryBeginRecentLatencyMS,
		"repository_pool_acquire":                    snapshot.RepositoryPoolAcquireLatencyMS,
		"repository_pool_acquire_recent":             snapshot.RepositoryPoolAcquireRecentLatencyMS,
		"repository_tx_begin":                        snapshot.RepositoryTxBeginLatencyMS,
		"repository_tx_begin_recent":                 snapshot.RepositoryTxBeginRecentLatencyMS,
		"repository_idempotency_lock":                snapshot.RepositoryIdempotencyLockLatencyMS,
		"repository_idempotency_lock_recent":         snapshot.RepositoryIdempotencyLockRecentLatencyMS,
		"repository_find_existing":                   snapshot.RepositoryFindExistingLatencyMS,
		"repository_find_existing_recent":            snapshot.RepositoryFindExistingRecentLatencyMS,
		"repository_ensure_seq":                      snapshot.RepositoryEnsureSeqLatencyMS,
		"repository_ensure_seq_recent":               snapshot.RepositoryEnsureSeqRecentLatencyMS,
		"repository_allocate_seq":                    snapshot.RepositoryAllocateSeqLatencyMS,
		"repository_allocate_seq_recent":             snapshot.RepositoryAllocateSeqRecentLatencyMS,
		"repository_insert_message":                  snapshot.RepositoryInsertMessageLatencyMS,
		"repository_insert_message_recent":           snapshot.RepositoryInsertMessageRecentLatencyMS,
		"repository_insert_timeline":                 snapshot.RepositoryInsertTimelineLatencyMS,
		"repository_insert_timeline_recent":          snapshot.RepositoryInsertTimelineRecentLatencyMS,
		"repository_insert_outbox":                   snapshot.RepositoryInsertOutboxLatencyMS,
		"repository_insert_outbox_recent":            snapshot.RepositoryInsertOutboxRecentLatencyMS,
		"repository_commit":                          snapshot.RepositoryCommitLatencyMS,
		"repository_commit_recent":                   snapshot.RepositoryCommitRecentLatencyMS,
		"conversation_seq_alloc":                     snapshot.ConversationSeqAllocLatencyMS,
		"conversation_seq_alloc_recent":              snapshot.ConversationSeqAllocRecentLatencyMS,
		"kafka_publish":                              snapshot.KafkaPublishLatencyMS,
		"kafka_publish_call":                         snapshot.KafkaPublishCallLatencyMS,
		"kafka_publish_record_estimate":              snapshot.KafkaPublishRecordLatencyEstimateMS,
		"outbox_process_ready":                       snapshot.OutboxProcessReadyLatencyMS,
		"outbox_process_ready_active":                snapshot.OutboxProcessReadyActiveLatencyMS,
		"outbox_process_ready_active_recent":         snapshot.OutboxProcessReadyActiveRecentLatencyMS,
		"outbox_process_ready_idle":                  snapshot.OutboxProcessReadyIdleLatencyMS,
		"outbox_fetch_ready":                         snapshot.OutboxFetchReadyLatencyMS,
		"outbox_mark_published":                      snapshot.OutboxMarkPublishedLatencyMS,
		"outbox_commit":                              snapshot.OutboxCommitLatencyMS,
	}
	writePrometheusHeader(builder, "nexusim_message_latency_samples_total", "Counter", "Total message-service latency samples by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_latency_avg_milliseconds", "Gauge", "Average message-service latency in milliseconds by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_latency_p50_milliseconds", "Gauge", "P50 message-service latency in milliseconds by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_latency_p95_milliseconds", "Gauge", "P95 message-service latency in milliseconds by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_latency_p99_milliseconds", "Gauge", "P99 message-service latency in milliseconds by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_latency_max_milliseconds", "Gauge", "Maximum message-service latency in milliseconds by fixed operation.")
	keys := sortedKeys(latencies)
	for _, operation := range keys {
		sample := latencies[operation]
		labels := map[string]string{"operation": operation}
		writePrometheusSample(builder, "nexusim_message_latency_samples_total", labels, strconv.FormatInt(sample.Count, 10))
		writePrometheusSample(builder, "nexusim_message_latency_avg_milliseconds", labels, prometheusFloat(sample.AvgMS))
		writePrometheusSample(builder, "nexusim_message_latency_p50_milliseconds", labels, prometheusFloat(sample.P50MS))
		writePrometheusSample(builder, "nexusim_message_latency_p95_milliseconds", labels, prometheusFloat(sample.P95MS))
		writePrometheusSample(builder, "nexusim_message_latency_p99_milliseconds", labels, prometheusFloat(sample.P99MS))
		writePrometheusSample(builder, "nexusim_message_latency_max_milliseconds", labels, prometheusFloat(sample.MaxMS))
	}
}

func writeValueSnapshotsPrometheus(builder *strings.Builder, snapshot Snapshot) {
	values := map[string]ValueSnapshot{
		"kafka_publish_records_per_call":        snapshot.KafkaPublishRecordsPerCall,
		"kafka_publish_records_per_call_recent": snapshot.KafkaPublishRecordsPerCallRecent,
		"outbox_fetched_per_call":               snapshot.OutboxFetchedPerCall,
		"outbox_fetched_per_call_recent":        snapshot.OutboxFetchedPerCallRecent,
	}
	writePrometheusHeader(builder, "nexusim_message_value_samples_total", "Counter", "Total message-service value samples by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_value_avg", "Gauge", "Average message-service sampled value by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_value_p50", "Gauge", "P50 message-service sampled value by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_value_p95", "Gauge", "P95 message-service sampled value by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_value_p99", "Gauge", "P99 message-service sampled value by fixed operation.")
	writePrometheusHeader(builder, "nexusim_message_value_max", "Gauge", "Maximum message-service sampled value by fixed operation.")
	keys := sortedKeys(values)
	for _, operation := range keys {
		sample := values[operation]
		labels := map[string]string{"operation": operation}
		writePrometheusSample(builder, "nexusim_message_value_samples_total", labels, strconv.FormatInt(sample.Count, 10))
		writePrometheusSample(builder, "nexusim_message_value_avg", labels, prometheusFloat(sample.Avg))
		writePrometheusSample(builder, "nexusim_message_value_p50", labels, prometheusFloat(sample.P50))
		writePrometheusSample(builder, "nexusim_message_value_p95", labels, prometheusFloat(sample.P95))
		writePrometheusSample(builder, "nexusim_message_value_p99", labels, prometheusFloat(sample.P99))
		writePrometheusSample(builder, "nexusim_message_value_max", labels, prometheusFloat(sample.Max))
	}
}

func writePGPoolPrometheus(builder *strings.Builder, snapshot PGPoolSnapshot) {
	writePrometheusHeader(builder, "nexusim_message_pg_pool_acquire_total", "Counter", "Total message-service PostgreSQL pool acquire count.")
	writePrometheusSample(builder, "nexusim_message_pg_pool_acquire_total", nil, strconv.FormatInt(snapshot.AcquireCount, 10))
	writePrometheusHeader(builder, "nexusim_message_pg_pool_acquire_duration_milliseconds_total", "Counter", "Total message-service PostgreSQL pool acquire duration in milliseconds.")
	writePrometheusSample(builder, "nexusim_message_pg_pool_acquire_duration_milliseconds_total", nil, strconv.FormatInt(snapshot.AcquireDurationMS, 10))
	writePrometheusHeader(builder, "nexusim_message_pg_pool_canceled_acquire_total", "Counter", "Total message-service PostgreSQL pool canceled acquire count.")
	writePrometheusSample(builder, "nexusim_message_pg_pool_canceled_acquire_total", nil, strconv.FormatInt(snapshot.CanceledAcquireCount, 10))
	writePrometheusHeader(builder, "nexusim_message_pg_pool_empty_acquire_total", "Counter", "Total message-service PostgreSQL pool empty acquire count.")
	writePrometheusSample(builder, "nexusim_message_pg_pool_empty_acquire_total", nil, strconv.FormatInt(snapshot.EmptyAcquireCount, 10))
	writePrometheusHeader(builder, "nexusim_message_pg_pool_conns", "Gauge", "message-service PostgreSQL pool connection counts by state.")
	writePrometheusSample(builder, "nexusim_message_pg_pool_conns", map[string]string{"state": "acquired"}, strconv.FormatInt(int64(snapshot.AcquiredConns), 10))
	writePrometheusSample(builder, "nexusim_message_pg_pool_conns", map[string]string{"state": "constructing"}, strconv.FormatInt(int64(snapshot.ConstructingConns), 10))
	writePrometheusSample(builder, "nexusim_message_pg_pool_conns", map[string]string{"state": "idle"}, strconv.FormatInt(int64(snapshot.IdleConns), 10))
	writePrometheusSample(builder, "nexusim_message_pg_pool_conns", map[string]string{"state": "total"}, strconv.FormatInt(int64(snapshot.TotalConns), 10))
	writePrometheusSample(builder, "nexusim_message_pg_pool_conns", map[string]string{"state": "max"}, strconv.FormatInt(int64(snapshot.MaxConns), 10))
}

func writeOutboxRelayPrometheus(builder *strings.Builder, snapshot types.OutboxRelayWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_message_outbox_relay_errors_total", "Counter", "Total message-service outbox relay runtime errors.")
	writePrometheusSample(builder, "nexusim_message_outbox_relay_errors_total", nil, strconv.FormatUint(snapshot.TotalErrors, 10))
	writePrometheusHeader(builder, "nexusim_message_outbox_relay_consecutive_errors", "Gauge", "Current consecutive message-service outbox relay runtime error count.")
	writePrometheusSample(builder, "nexusim_message_outbox_relay_consecutive_errors", nil, strconv.FormatUint(snapshot.ConsecutiveErrors, 10))
	writePrometheusHeader(builder, "nexusim_message_outbox_relay_last_error_unix_milliseconds", "Gauge", "Unix time of the last message-service outbox relay runtime error in milliseconds.")
	writePrometheusSample(builder, "nexusim_message_outbox_relay_last_error_unix_milliseconds", nil, strconv.FormatInt(snapshot.LastErrorAtMS, 10))
	writePrometheusHeader(builder, "nexusim_message_outbox_relay_last_success_unix_milliseconds", "Gauge", "Unix time of the last message-service outbox relay success in milliseconds.")
	writePrometheusSample(builder, "nexusim_message_outbox_relay_last_success_unix_milliseconds", nil, strconv.FormatInt(snapshot.LastSuccessAtMS, 10))
	writePrometheusHeader(builder, "nexusim_message_outbox_relay_last_published_unix_milliseconds", "Gauge", "Unix time of the last message-service outbox relay publish in milliseconds.")
	writePrometheusSample(builder, "nexusim_message_outbox_relay_last_published_unix_milliseconds", nil, strconv.FormatInt(snapshot.LastPublishedAtMS, 10))
	writePrometheusHeader(builder, "nexusim_message_outbox_relay_last_error_backoff_milliseconds", "Gauge", "Last message-service outbox relay error backoff duration in milliseconds.")
	writePrometheusSample(builder, "nexusim_message_outbox_relay_last_error_backoff_milliseconds", nil, strconv.FormatInt(snapshot.LastErrorBackoffMS, 10))
}

func writeTracePrometheus(builder *strings.Builder, snapshot monitoringinfra.TraceSnapshot) {
	labels := map[string]string{"exporter": snapshot.Exporter}
	writePrometheusHeader(builder, "nexusim_message_otel_traces_enabled", "Gauge", "Whether message-service OpenTelemetry tracing is enabled.")
	writePrometheusSample(builder, "nexusim_message_otel_traces_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusHeader(builder, "nexusim_message_otel_traces_otlp_endpoint_configured", "Gauge", "Whether message-service OpenTelemetry OTLP endpoint is configured.")
	writePrometheusSample(builder, "nexusim_message_otel_traces_otlp_endpoint_configured", labels, prometheusBool(snapshot.OTLPEndpointSet))
	writePrometheusHeader(builder, "nexusim_message_otel_traces_otlp_insecure", "Gauge", "Whether message-service OpenTelemetry OTLP transport is configured as insecure.")
	writePrometheusSample(builder, "nexusim_message_otel_traces_otlp_insecure", labels, prometheusBool(snapshot.OTLPInsecure))
	writePrometheusHeader(builder, "nexusim_message_otel_traces_sampling_ratio", "Gauge", "message-service OpenTelemetry trace sampling ratio.")
	writePrometheusSample(builder, "nexusim_message_otel_traces_sampling_ratio", labels, prometheusFloat(snapshot.SamplingRatio))
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writePrometheusHeader(builder *strings.Builder, name string, typ string, help string) {
	fmt.Fprintf(builder, "# HELP %s %s\n", name, help)
	fmt.Fprintf(builder, "# TYPE %s %s\n", name, strings.ToLower(typ))
}

func writePrometheusSample(builder *strings.Builder, name string, labels map[string]string, value string) {
	builder.WriteString(name)
	if len(labels) > 0 {
		builder.WriteString("{")
		keys := sortedKeys(labels)
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

func prometheusFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func prometheusBool(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
