package monitoring

import (
	"sort"
	"strconv"
	"strings"

	authinfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/auth"
	"github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/memory"
	redisroute "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/redisroute"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

func renderPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writePrometheusHeader(&builder, "nexusim_push_gateway_build_info", "Push gateway build marker.", "gauge")
	writePrometheusSample(&builder, "nexusim_push_gateway_build_info", map[string]string{"service": serviceName}, 1)

	writeMemoryPrometheus(&builder, snapshot.Memory)
	writeRedisRoutePrometheus(&builder, snapshot.RedisRegistryMetrics, snapshot.RedisSubscriberStats)
	writeRedisSubscriberWorkerPrometheus(&builder, snapshot.RedisSubscriberWorker)
	writeAuthJWKPrometheus(&builder, snapshot.AuthJWKStats)
	writeConsumerWorkersPrometheus(&builder, snapshot.DeliveryConsumer, snapshot.IdentityConsumer)
	writeWebSocketWriterPrometheus(&builder, snapshot.WebSocketWriter)
	writeTracePrometheus(&builder, snapshot.Trace)
	return builder.String()
}

func writeMemoryPrometheus(builder *strings.Builder, snapshot *memory.Metrics) {
	writePrometheusHeader(builder, "nexusim_push_gateway_sessions", "Push gateway session counts by state.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_session_events_total", "Push gateway session event counters.", "counter")
	writePrometheusHeader(builder, "nexusim_push_gateway_resume_buffer", "Push gateway in-memory resume buffer gauges.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_resume_buffer_events_total", "Push gateway in-memory resume buffer event counters.", "counter")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_push_gateway_sessions", map[string]string{"state": "connected"}, snapshot.ConnectedSessions)
	writePrometheusSample(builder, "nexusim_push_gateway_session_events_total", map[string]string{"event": "queue_full"}, snapshot.SessionQueueFullCount)
	writePrometheusSample(builder, "nexusim_push_gateway_session_events_total", map[string]string{"event": "slow_evicted"}, snapshot.SlowSessionEvictedCount)
	writePrometheusSample(builder, "nexusim_push_gateway_session_events_total", map[string]string{"event": "identity_evicted"}, snapshot.IdentitySessionEvictedCount)
	writePrometheusSample(builder, "nexusim_push_gateway_resume_buffer", map[string]string{"state": "stored_frames"}, snapshot.ResumeBufferStoredFrames)
	writePrometheusSample(builder, "nexusim_push_gateway_resume_buffer", map[string]string{"state": "tokens"}, snapshot.ResumeBufferTokenCount)
	writePrometheusSample(builder, "nexusim_push_gateway_resume_buffer_events_total", map[string]string{"event": "replay"}, snapshot.ResumeBufferReplayCount)
	writePrometheusSample(builder, "nexusim_push_gateway_resume_buffer_events_total", map[string]string{"event": "miss"}, snapshot.ResumeBufferMissCount)
	writePrometheusSample(builder, "nexusim_push_gateway_resume_buffer_events_total", map[string]string{"event": "expired"}, snapshot.ResumeBufferExpiredCount)
}

func writeRedisRoutePrometheus(builder *strings.Builder, registry *redisroute.Metrics, subscriber *redisroute.Metrics) {
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_route_events_total", "Push gateway Redis route counters.", "counter")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_resume_events_total", "Push gateway Redis-backed resume counters.", "counter")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds", "Push gateway Redis subscriber local fanout duration histogram.", "histogram")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds", "Push gateway Redis subscriber max observed local fanout duration.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth", "Push gateway Redis subscriber conversation signal fanout queue depth.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds", "Push gateway Redis subscriber conversation signal fanout queue wait duration histogram.", "histogram")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds", "Push gateway Redis subscriber conversation signal max observed queue wait duration.", "gauge")
	writeRedisRoutePrometheusSamples(builder, "registry", registry)
	writeRedisRoutePrometheusSamples(builder, "subscriber", subscriber)
}

func writeRedisRoutePrometheusSamples(builder *strings.Builder, role string, snapshot *redisroute.Metrics) {
	if snapshot == nil {
		return
	}
	labels := func(event string) map[string]string {
		return map[string]string{"role": role, "event": event}
	}
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("register_error"), snapshot.RedisRouteRegisterErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("renew_error"), snapshot.RedisRouteRenewErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("renew_session_evicted"), snapshot.RedisRouteRenewSessionEvictedCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("lookup_error"), snapshot.RedisRouteLookupErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("remote_matched_sessions"), snapshot.RedisRouteRemoteMatchedSessions)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("remote_publish_call"), snapshot.RedisRouteRemotePublishCallCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("remote_publish_error"), snapshot.RedisRouteRemotePublishErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("remote_no_subscriber"), snapshot.RedisRouteRemoteNoSubscriberCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("remote_enqueued_sessions"), snapshot.RedisRouteRemoteEnqueuedSessions)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("stale_removed"), snapshot.RedisRouteStaleRemovedCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("cleanup_error"), snapshot.RedisRouteCleanupErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("subscriber_message"), snapshot.RedisRouteSubscriberMessageCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("subscriber_malformed"), snapshot.RedisRouteSubscriberMalformedCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("subscriber_enqueued"), snapshot.RedisRouteSubscriberEnqueuedCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("subscriber_evicted"), snapshot.RedisRouteSubscriberEvictedCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("subscriber_error"), snapshot.RedisRouteSubscriberErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("subscriber_signal_fanout_queued"), snapshot.RedisRouteSubscriberSignalFanoutQueuedCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("subscriber_signal_fanout_queue_full"), snapshot.RedisRouteSubscriberSignalFanoutQueueFullCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_route_events_total", labels("subscriber_signal_fanout_worker_error"), snapshot.RedisRouteSubscriberSignalFanoutWorkerErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_resume_events_total", labels("replay"), snapshot.RedisResumeReplayCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_resume_events_total", labels("miss"), snapshot.RedisResumeMissCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_resume_events_total", labels("append"), snapshot.RedisResumeAppendCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_resume_events_total", labels("append_error"), snapshot.RedisResumeAppendErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_resume_events_total", labels("permission_denied"), snapshot.RedisResumePermissionDeniedCount)
	if role == "subscriber" {
		writeRedisSubscriberFanoutDurationPrometheusSamples(builder, "delivery_notify", snapshot.RedisRouteSubscriberNotifyFanoutDuration)
		writeRedisSubscriberFanoutDurationPrometheusSamples(builder, "conversation_signal", snapshot.RedisRouteSubscriberSignalFanoutDuration)
		writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth", nil, snapshot.RedisRouteSubscriberSignalFanoutQueueDepth)
		writeRedisSubscriberSignalQueueWaitDurationPrometheusSamples(builder, snapshot.RedisRouteSubscriberSignalFanoutQueueWaitDuration)
	}
}

func writeRedisSubscriberFanoutDurationPrometheusSamples(builder *strings.Builder, operation string, snapshot redisroute.DurationSnapshot) {
	labels := map[string]string{"operation": operation}
	for _, bucket := range snapshot.Buckets {
		bucketLabels := map[string]string{"operation": operation, "le": bucket.LE}
		writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket", bucketLabels, bucket.Count)
	}
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum", labels, snapshot.SumMS)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count", labels, snapshot.Count)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds", labels, snapshot.MaxMS)
}

func writeRedisSubscriberSignalQueueWaitDurationPrometheusSamples(builder *strings.Builder, snapshot redisroute.DurationSnapshot) {
	for _, bucket := range snapshot.Buckets {
		writePrometheusSample(
			builder,
			"nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket",
			map[string]string{"le": bucket.LE},
			bucket.Count,
		)
	}
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum", nil, snapshot.SumMS)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count", nil, snapshot.Count)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds", nil, snapshot.MaxMS)
}

func writeRedisSubscriberWorkerPrometheus(builder *strings.Builder, snapshot *types.RedisSubscriberWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_worker_errors_total", "Push gateway Redis subscriber worker runtime errors.", "counter")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_worker_consecutive_errors", "Push gateway Redis subscriber worker consecutive runtime errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_worker_last_error_unix_milliseconds", "Push gateway Redis subscriber worker last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_worker_last_success_unix_milliseconds", "Push gateway Redis subscriber worker last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_redis_subscriber_worker_last_error_backoff_milliseconds", "Push gateway Redis subscriber worker error backoff.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_worker_errors_total", nil, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_worker_consecutive_errors", nil, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_worker_last_error_unix_milliseconds", nil, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_worker_last_success_unix_milliseconds", nil, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_push_gateway_redis_subscriber_worker_last_error_backoff_milliseconds", nil, snapshot.LastErrorBackoffMS)
}

func writeAuthJWKPrometheus(builder *strings.Builder, snapshot *authinfra.JWKStats) {
	writePrometheusHeader(builder, "nexusim_push_gateway_auth_jwks_remote_url_configured", "Push gateway JWT auth remote JWKS URL configured flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_auth_jwks_cached_keys", "Push gateway cached JWT public key count.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_auth_jwks_refresh_failures_total", "Push gateway JWT JWKS refresh failures.", "counter")
	writePrometheusHeader(builder, "nexusim_push_gateway_auth_jwks_last_refresh_success_unix_milliseconds", "Push gateway JWT JWKS last successful refresh timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_auth_jwks_last_refresh_failure_unix_milliseconds", "Push gateway JWT JWKS last failed refresh timestamp.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_push_gateway_auth_jwks_remote_url_configured", nil, prometheusBool(snapshot.RemoteURLConfigured))
	writePrometheusSample(builder, "nexusim_push_gateway_auth_jwks_cached_keys", nil, snapshot.CachedKeyCount)
	writePrometheusSample(builder, "nexusim_push_gateway_auth_jwks_refresh_failures_total", nil, snapshot.RefreshFailures)
	writePrometheusSample(builder, "nexusim_push_gateway_auth_jwks_last_refresh_success_unix_milliseconds", nil, snapshot.LastRefreshSuccess)
	writePrometheusSample(builder, "nexusim_push_gateway_auth_jwks_last_refresh_failure_unix_milliseconds", nil, snapshot.LastRefreshFailure)
}

func writeConsumerWorkersPrometheus(builder *strings.Builder, delivery *types.ConsumerWorkerSnapshot, identity *types.ConsumerWorkerSnapshot) {
	writePrometheusHeader(builder, "nexusim_push_gateway_consumer_worker_errors_total", "Push gateway Kafka consumer worker runtime errors.", "counter")
	writePrometheusHeader(builder, "nexusim_push_gateway_consumer_worker_consecutive_errors", "Push gateway Kafka consumer worker consecutive runtime errors.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_consumer_worker_last_error_unix_milliseconds", "Push gateway Kafka consumer worker last error timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_consumer_worker_last_success_unix_milliseconds", "Push gateway Kafka consumer worker last success timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_consumer_worker_last_commit_unix_milliseconds", "Push gateway Kafka consumer worker last commit timestamp.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_consumer_worker_last_error_backoff_milliseconds", "Push gateway Kafka consumer worker error backoff.", "gauge")
	writeConsumerWorkerPrometheusSamples(builder, "delivery", delivery)
	writeConsumerWorkerPrometheusSamples(builder, "identity", identity)
}

func writeConsumerWorkerPrometheusSamples(builder *strings.Builder, kind string, snapshot *types.ConsumerWorkerSnapshot) {
	if snapshot == nil {
		return
	}
	labels := map[string]string{"consumer": kind}
	writePrometheusSample(builder, "nexusim_push_gateway_consumer_worker_errors_total", labels, snapshot.TotalErrors)
	writePrometheusSample(builder, "nexusim_push_gateway_consumer_worker_consecutive_errors", labels, snapshot.ConsecutiveErrors)
	writePrometheusSample(builder, "nexusim_push_gateway_consumer_worker_last_error_unix_milliseconds", labels, snapshot.LastErrorAtMS)
	writePrometheusSample(builder, "nexusim_push_gateway_consumer_worker_last_success_unix_milliseconds", labels, snapshot.LastSuccessAtMS)
	writePrometheusSample(builder, "nexusim_push_gateway_consumer_worker_last_commit_unix_milliseconds", labels, snapshot.LastCommitAtMS)
	writePrometheusSample(builder, "nexusim_push_gateway_consumer_worker_last_error_backoff_milliseconds", labels, snapshot.LastErrorBackoffMS)
}

func writeWebSocketWriterPrometheus(builder *strings.Builder, snapshot *types.WebSocketWriterSnapshot) {
	writePrometheusHeader(builder, "nexusim_push_gateway_ws_writer_events_total", "Push gateway WebSocket writer event counters.", "counter")
	writePrometheusHeader(builder, "nexusim_push_gateway_ws_writer_last_event_unix_milliseconds", "Push gateway WebSocket writer last event timestamps.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_ws_writer_write_duration_milliseconds", "Push gateway WebSocket writer write duration histogram.", "histogram")
	writePrometheusHeader(builder, "nexusim_push_gateway_ws_writer_write_duration_max_milliseconds", "Push gateway WebSocket writer max observed write duration.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_ws_writer_queue_duration_milliseconds", "Push gateway WebSocket writer outbound queue duration histogram.", "histogram")
	writePrometheusHeader(builder, "nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds", "Push gateway WebSocket writer max observed outbound queue duration.", "gauge")
	if snapshot == nil {
		return
	}
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "outbound_frame_dequeued"}, snapshot.OutboundFrameDequeuedCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "frame_write_attempt"}, snapshot.FrameWriteAttemptCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "frame_write_success"}, snapshot.FrameWriteSuccessCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "frame_write_error"}, snapshot.FrameWriteErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "delivery_notify_write_attempt"}, snapshot.DeliveryNotifyWriteAttemptCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "delivery_notify_write_success"}, snapshot.DeliveryNotifyWriteSuccessCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "delivery_notify_write_error"}, snapshot.DeliveryNotifyWriteErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "resume_hint_write_attempt"}, snapshot.ResumeHintWriteAttemptCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "resume_hint_write_success"}, snapshot.ResumeHintWriteSuccessCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_events_total", map[string]string{"event": "resume_hint_write_error"}, snapshot.ResumeHintWriteErrorCount)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_last_event_unix_milliseconds", map[string]string{"event": "write_success"}, snapshot.LastWriteSuccessAtMS)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_last_event_unix_milliseconds", map[string]string{"event": "write_error"}, snapshot.LastWriteErrorAtMS)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_last_event_unix_milliseconds", map[string]string{"event": "delivery_notify_write_success"}, snapshot.LastDeliveryNotifyWriteAtMS)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_last_event_unix_milliseconds", map[string]string{"event": "delivery_notify_write_error"}, snapshot.LastDeliveryNotifyWriteErrorAtMS)
	writeWebSocketDurationPrometheusSamples(builder, "frame_write", snapshot.FrameWriteDuration)
	writeWebSocketDurationPrometheusSamples(builder, "delivery_notify", snapshot.DeliveryNotifyWriteDuration)
	writeWebSocketQueueDurationPrometheusSamples(builder, "frame", snapshot.FrameQueueDuration)
	writeWebSocketQueueDurationPrometheusSamples(builder, "delivery_notify", snapshot.DeliveryNotifyQueueDuration)
}

func writeWebSocketDurationPrometheusSamples(builder *strings.Builder, operation string, snapshot types.WebSocketWriterDurationSnapshot) {
	labels := map[string]string{"operation": operation}
	for _, bucket := range snapshot.Buckets {
		bucketLabels := map[string]string{"operation": operation, "le": bucket.LE}
		writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket", bucketLabels, bucket.Count)
	}
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum", labels, snapshot.SumMS)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_write_duration_milliseconds_count", labels, snapshot.Count)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_write_duration_max_milliseconds", labels, snapshot.MaxMS)
}

func writeWebSocketQueueDurationPrometheusSamples(builder *strings.Builder, operation string, snapshot types.WebSocketWriterDurationSnapshot) {
	labels := map[string]string{"operation": operation}
	for _, bucket := range snapshot.Buckets {
		bucketLabels := map[string]string{"operation": operation, "le": bucket.LE}
		writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket", bucketLabels, bucket.Count)
	}
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum", labels, snapshot.SumMS)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count", labels, snapshot.Count)
	writePrometheusSample(builder, "nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds", labels, snapshot.MaxMS)
}

func writeTracePrometheus(builder *strings.Builder, snapshot *TraceSnapshot) {
	writePrometheusHeader(builder, "nexusim_push_gateway_otel_traces_enabled", "Push gateway OpenTelemetry trace enabled flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_otel_traces_otlp_endpoint_configured", "Push gateway OpenTelemetry OTLP endpoint configured flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_otel_traces_otlp_insecure", "Push gateway OpenTelemetry OTLP insecure flag.", "gauge")
	writePrometheusHeader(builder, "nexusim_push_gateway_otel_traces_sampling_ratio", "Push gateway OpenTelemetry trace sampling ratio.", "gauge")
	if snapshot == nil {
		return
	}
	exporter := snapshot.Exporter
	if strings.TrimSpace(exporter) == "" {
		exporter = "disabled"
	}
	labels := map[string]string{"exporter": exporter}
	writePrometheusSample(builder, "nexusim_push_gateway_otel_traces_enabled", labels, prometheusBool(snapshot.Enabled))
	writePrometheusSample(builder, "nexusim_push_gateway_otel_traces_otlp_endpoint_configured", labels, prometheusBool(snapshot.OTLPEndpointSet))
	writePrometheusSample(builder, "nexusim_push_gateway_otel_traces_otlp_insecure", labels, prometheusBool(snapshot.OTLPInsecure))
	writePrometheusSample(builder, "nexusim_push_gateway_otel_traces_sampling_ratio", labels, snapshot.SamplingRatio)
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
