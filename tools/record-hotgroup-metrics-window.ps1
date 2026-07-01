param(
    [Parameter(Mandatory = $true)]
    [string]$ResultDir,
    [string]$PrometheusBaseUrl = "http://172.31.50.2:19091",
    [int]$PaddingMinutes = 5,
    [int]$StepSeconds = 30,
    [string]$OutputPath = "",
    [string]$MarkdownPath = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultDir -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "ResultDir"

function Get-PropertyValue {
    param(
        [object]$Object,
        [string]$Name
    )

    if ($null -eq $Object) {
        return $null
    }

    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        return $null
    }

    return $property.Value
}

function Convert-ToDoubleOrNull {
    param([object]$Value)

    if ($null -eq $Value) {
        return $null
    }

    try {
        return [double]::Parse([string]$Value, [System.Globalization.CultureInfo]::InvariantCulture)
    }
    catch {
        return $null
    }
}

function Format-Number {
    param([object]$Value)

    if ($null -eq $Value) {
        return ""
    }

    return ([double]$Value).ToString("0.###", [System.Globalization.CultureInfo]::InvariantCulture)
}

function Escape-MarkdownCell {
    param([object]$Value)

    if ($null -eq $Value) {
        return ""
    }

    return ([string]$Value).Replace("|", "\|").Replace([string][char]0x60, "'")
}

function Invoke-PrometheusRangeQuery {
    param(
        [string]$BaseUrl,
        [string]$Query,
        [int64]$Start,
        [int64]$End,
        [int]$Step
    )

    $escapedQuery = [uri]::EscapeDataString($Query)
    $uri = "$BaseUrl/api/v1/query_range?query=$escapedQuery&start=$Start&end=$End&step=$Step"
    $response = Invoke-RestMethod -Uri $uri -TimeoutSec 20
    if ($response.status -ne "success") {
        throw "Prometheus query failed for [$Query]: $($response.status)"
    }
    return $response.data.result
}

function Summarize-PrometheusResult {
    param(
        [string]$Name,
        [string]$Query,
        [object[]]$Series
    )

    $sampleCount = 0
    $seriesCount = @($Series).Count
    $max = $null
    $min = $null
    $last = $null
    foreach ($serie in @($Series)) {
        foreach ($point in @($serie.values)) {
            if ($point.Count -lt 2) {
                continue
            }
            $value = Convert-ToDoubleOrNull $point[1]
            if ($null -eq $value) {
                continue
            }
            $sampleCount++
            if ($null -eq $max -or $value -gt $max) {
                $max = $value
            }
            if ($null -eq $min -or $value -lt $min) {
                $min = $value
            }
            $last = $value
        }
    }

    return [pscustomobject]@{
        name = $Name
        query = $Query
        has_data = $sampleCount -gt 0
        series_count = $seriesCount
        sample_count = $sampleCount
        min = $min
        max = $max
        last = $last
    }
}

function New-HotGroupMetricQueries {
    param([int]$WindowSeconds)

    if ($WindowSeconds -lt 1) {
        $WindowSeconds = 1
    }

    return @(
        [pscustomobject]@{ name = "core_targets_up"; query = 'sum(up{job=~"nexusim-message-service|nexusim-conversation-service|nexusim-delivery-service|nexusim-push-gateway"})' },
        [pscustomobject]@{ name = "message_send_p95_ms"; query = 'max(nexusim_message_latency_p95_milliseconds{operation="send_message"})' },
        [pscustomobject]@{ name = "message_send_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="send_message"})' },
        [pscustomobject]@{ name = "message_send_p95_recent_ms"; query = 'max(nexusim_message_latency_p95_milliseconds{operation="send_message_recent"})' },
        [pscustomobject]@{ name = "message_send_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="send_message_recent"})' },
        [pscustomobject]@{ name = "message_seq_alloc_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"})' },
        [pscustomobject]@{ name = "message_seq_alloc_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc_recent"})' },
        [pscustomobject]@{ name = "message_repository_append_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_append"})' },
        [pscustomobject]@{ name = "message_repository_append_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_append_recent"})' },
        [pscustomobject]@{ name = "message_repository_pool_acquire_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire"})' },
        [pscustomobject]@{ name = "message_repository_pool_acquire_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire_recent"})' },
        [pscustomobject]@{ name = "message_repository_tx_begin_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin"})' },
        [pscustomobject]@{ name = "message_repository_tx_begin_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin_recent"})' },
        [pscustomobject]@{ name = "message_repository_idempotency_lock_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock"})' },
        [pscustomobject]@{ name = "message_repository_idempotency_lock_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock_recent"})' },
        [pscustomobject]@{ name = "message_repository_find_existing_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing"})' },
        [pscustomobject]@{ name = "message_repository_find_existing_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing_recent"})' },
        [pscustomobject]@{ name = "message_repository_ensure_seq_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq"})' },
        [pscustomobject]@{ name = "message_repository_ensure_seq_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq_recent"})' },
        [pscustomobject]@{ name = "message_repository_allocate_seq_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq"})' },
        [pscustomobject]@{ name = "message_repository_allocate_seq_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq_recent"})' },
        [pscustomobject]@{ name = "message_repository_insert_message_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message"})' },
        [pscustomobject]@{ name = "message_repository_insert_message_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message_recent"})' },
        [pscustomobject]@{ name = "message_repository_insert_timeline_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline"})' },
        [pscustomobject]@{ name = "message_repository_insert_timeline_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline_recent"})' },
        [pscustomobject]@{ name = "message_repository_insert_outbox_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox"})' },
        [pscustomobject]@{ name = "message_repository_insert_outbox_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox_recent"})' },
        [pscustomobject]@{ name = "message_repository_commit_p99_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_commit"})' },
        [pscustomobject]@{ name = "message_repository_commit_p99_recent_ms"; query = 'max(nexusim_message_latency_p99_milliseconds{operation="repository_commit_recent"})' },
        [pscustomobject]@{ name = "message_outbox_fetched_per_call_avg"; query = 'max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"})' },
        [pscustomobject]@{ name = "message_kafka_records_per_call_avg"; query = 'max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"})' },
        [pscustomobject]@{ name = "conversation_grpc_rps"; query = 'sum(rate(nexusim_conversation_grpc_method_requests_total[5m]))' },
        [pscustomobject]@{ name = "delivery_outbox_pending"; query = 'max(nexusim_delivery_outbox{state="pending"})' },
        [pscustomobject]@{ name = "delivery_outbox_pending_ready"; query = 'max(nexusim_delivery_outbox{state="pending_ready"})' },
        [pscustomobject]@{ name = "delivery_outbox_dlq"; query = 'max(nexusim_delivery_outbox{state="dlq"})' },
        [pscustomobject]@{ name = "delivery_projection_unresolved"; query = 'max(nexusim_delivery_projection_failures{state="unresolved_total"})' },
        [pscustomobject]@{ name = "delivery_timeline_worker_errors_5m"; query = 'sum(increase(nexusim_delivery_timeline_worker_errors_total[5m]))' },
        [pscustomobject]@{ name = "delivery_outbox_relay_errors_5m"; query = 'sum(increase(nexusim_delivery_outbox_relay_errors_total[5m]))' },
        [pscustomobject]@{ name = "delivery_grpc_rps"; query = 'sum(rate(nexusim_delivery_grpc_method_requests_total[5m]))' },
        [pscustomobject]@{ name = "delivery_grpc_errors_5m"; query = 'sum(increase(nexusim_delivery_grpc_method_errors_total[5m]))' },
        [pscustomobject]@{ name = "push_connected_sessions"; query = 'max(nexusim_push_gateway_sessions{state="connected"})' },
        [pscustomobject]@{ name = "push_slow_evicted_5m"; query = 'sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_events_5m"; query = 'sum(increase(nexusim_push_gateway_ws_writer_events_total[5m]))' },
        [pscustomobject]@{ name = "push_writer_outbound_dequeued_5m"; query = 'sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_frame_write_attempt_5m"; query = 'sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_frame_write_success_5m"; query = 'sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_frame_write_error_5m"; query = 'sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_attempt_5m"; query = 'sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_success_5m"; query = 'sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_error_5m"; query = 'sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_frame_write_duration_p95_ms_5m"; query = 'histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le))' },
        [pscustomobject]@{ name = "push_writer_frame_write_duration_p99_ms_5m"; query = 'histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_duration_p95_ms_5m"; query = 'histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_duration_p99_ms_5m"; query = 'histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_duration_avg_ms_5m"; query = 'sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_duration_max_ms"; query = 'max(nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"})' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_queue_p95_ms_5m"; query = 'histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_queue_p99_ms_5m"; query = 'histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_queue_avg_ms_5m"; query = 'sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count{operation="delivery_notify"}[5m]))' },
        [pscustomobject]@{ name = "push_writer_delivery_notify_queue_max_ms"; query = 'max(nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds{operation="delivery_notify"})' },
        [pscustomobject]@{ name = "push_consumer_worker_errors_5m"; query = 'sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m]))' },
        [pscustomobject]@{ name = "push_redis_route_events_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total[5m]))' },
        [pscustomobject]@{ name = "push_redis_registry_remote_matched_sessions_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_registry_remote_publish_call_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_registry_remote_publish_error_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_registry_remote_no_subscriber_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_registry_conversation_route_cache_hit_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_registry_conversation_route_cache_miss_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_registry_conversation_route_cache_invalidated_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_registry_remote_enqueued_sessions_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_remote_matched_sessions_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_remote_publish_call_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_remote_publish_error_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_remote_no_subscriber_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_no_subscriber"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_conversation_route_cache_hit_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_conversation_route_cache_miss_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_conversation_route_cache_invalidated_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_remote_enqueued_sessions_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_messages_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_enqueued_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_evicted_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_errors_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queued_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queue_full_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_worker_errors_5m"; query = 'sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queue_depth"; query = 'max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth)' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_duration_p95_ms_5m"; query = 'histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_duration_p99_ms_5m"; query = 'histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_duration_avg_ms_5m"; query = 'sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum{operation="conversation_signal"}[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count{operation="conversation_signal"}[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_duration_max_ms"; query = 'max(nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds{operation="conversation_signal"})' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queue_wait_p95_ms_5m"; query = 'histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queue_wait_p99_ms_5m"; query = 'histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queue_wait_avg_ms_5m"; query = 'sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count[5m]))' },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queue_wait_max_ms"; query = 'max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds)' },
        [pscustomobject]@{ name = "push_writer_frame_write_success_window"; query = "sum(increase(nexusim_push_gateway_ws_writer_events_total{event=`"frame_write_success`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_writer_frame_write_error_window"; query = "sum(increase(nexusim_push_gateway_ws_writer_events_total{event=`"frame_write_error`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_writer_delivery_notify_success_window"; query = "sum(increase(nexusim_push_gateway_ws_writer_events_total{event=`"delivery_notify_write_success`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_writer_delivery_notify_error_window"; query = "sum(increase(nexusim_push_gateway_ws_writer_events_total{event=`"delivery_notify_write_error`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_writer_delivery_notify_duration_p95_ms_window"; query = "histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation=`"delivery_notify`"}[$($WindowSeconds)s])) by (le))" },
        [pscustomobject]@{ name = "push_writer_delivery_notify_duration_p99_ms_window"; query = "histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation=`"delivery_notify`"}[$($WindowSeconds)s])) by (le))" },
        [pscustomobject]@{ name = "push_writer_delivery_notify_queue_p95_ms_window"; query = "histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation=`"delivery_notify`"}[$($WindowSeconds)s])) by (le))" },
        [pscustomobject]@{ name = "push_writer_delivery_notify_queue_p99_ms_window"; query = "histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation=`"delivery_notify`"}[$($WindowSeconds)s])) by (le))" },
        [pscustomobject]@{ name = "push_redis_subscriber_messages_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"subscriber`",event=`"subscriber_message`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_subscriber_enqueued_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"subscriber`",event=`"subscriber_enqueued`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_subscriber_evicted_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"subscriber`",event=`"subscriber_evicted`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_subscriber_errors_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"subscriber`",event=`"subscriber_error`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_registry_conversation_route_cache_hit_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"registry`",event=`"conversation_route_cache_hit`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_registry_conversation_route_cache_miss_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"registry`",event=`"conversation_route_cache_miss`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_registry_conversation_route_cache_invalidated_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"registry`",event=`"conversation_route_cache_invalidated`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_remote_matched_sessions_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"delivery-consumer`",event=`"remote_matched_sessions`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_remote_publish_call_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"delivery-consumer`",event=`"remote_publish_call`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_remote_publish_error_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"delivery-consumer`",event=`"remote_publish_error`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_conversation_route_cache_hit_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"delivery-consumer`",event=`"conversation_route_cache_hit`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_conversation_route_cache_miss_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"delivery-consumer`",event=`"conversation_route_cache_miss`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_conversation_route_cache_invalidated_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"delivery-consumer`",event=`"conversation_route_cache_invalidated`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_delivery_consumer_remote_enqueued_sessions_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"delivery-consumer`",event=`"remote_enqueued_sessions`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queued_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"subscriber`",event=`"subscriber_signal_fanout_queued`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queue_full_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"subscriber`",event=`"subscriber_signal_fanout_queue_full`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_worker_errors_window"; query = "sum(increase(nexusim_push_gateway_redis_route_events_total{role=`"subscriber`",event=`"subscriber_signal_fanout_worker_error`"}[$($WindowSeconds)s]))" },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_duration_p95_ms_window"; query = "histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation=`"conversation_signal`"}[$($WindowSeconds)s])) by (le))" },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_duration_p99_ms_window"; query = "histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation=`"conversation_signal`"}[$($WindowSeconds)s])) by (le))" },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queue_wait_p95_ms_window"; query = "histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[$($WindowSeconds)s])) by (le))" },
        [pscustomobject]@{ name = "push_redis_subscriber_signal_fanout_queue_wait_p99_ms_window"; query = "histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[$($WindowSeconds)s])) by (le))" },
        [pscustomobject]@{ name = "message_pg_pool_conns"; query = 'max(nexusim_message_pg_pool_conns)' },
        [pscustomobject]@{ name = "conversation_pg_pool_conns"; query = 'max(nexusim_conversation_pg_pool_conns)' },
        [pscustomobject]@{ name = "delivery_pg_pool_conns"; query = 'max(nexusim_delivery_pg_pool_conns)' }
    )
}

function Write-MetricsMarkdown {
    param(
        [string]$Path,
        [object]$Summary,
        [object]$Window,
        [object[]]$Metrics
    )

    $builder = New-Object System.Text.StringBuilder
    [void]$builder.AppendLine("# Hot Group Metrics Window")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("- run_name: $($Summary.run_name)")
    [void]$builder.AppendLine("- commit: $($Summary.commit)")
    [void]$builder.AppendLine("- git_dirty: $($Summary.git_dirty)")
    [void]$builder.AppendLine("- result_dir: $ResultDir")
    [void]$builder.AppendLine("- prometheus: $PrometheusBaseUrl")
    [void]$builder.AppendLine("- window_start_utc: $($Window.start_utc)")
    [void]$builder.AppendLine("- window_end_utc: $($Window.end_utc)")
    [void]$builder.AppendLine("- step_seconds: $StepSeconds")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("## Run Parameters")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("| field | value |")
    [void]$builder.AppendLine("| --- | ---: |")
    [void]$builder.AppendLine("| group_size | $($Summary.group_size) |")
    [void]$builder.AppendLine("| message_count | $($Summary.message_count) |")
    [void]$builder.AppendLine("| message_rate | $($Summary.message_rate) |")
    [void]$builder.AppendLine("| sender_count | $($Summary.sender_count) |")
    [void]$builder.AppendLine("| subscriber_count | $($Summary.push.subscriber_count) |")
    [void]$builder.AppendLine("| fanout_mode | $($Summary.actual_fanout_mode) |")
    [void]$builder.AppendLine("| send_p95_ms | $($Summary.send.latency_p95_ms) |")
    [void]$builder.AppendLine("| send_p99_ms | $($Summary.send.latency_p99_ms) |")
    [void]$builder.AppendLine("| pull_p95_ms | $($Summary.receiver.pull_latency_p95_ms) |")
    [void]$builder.AppendLine("| conversation_signal_count | $($Summary.push.conversation_signal_count) |")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("## Prometheus Query Summary")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("| metric | has data | series | samples | min | max | last | query |")
    [void]$builder.AppendLine("| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |")
    foreach ($metric in $Metrics) {
        [void]$builder.AppendLine((
            "| {0} | {1} | {2} | {3} | {4} | {5} | {6} | {7} |" -f
            (Escape-MarkdownCell $metric.name),
            $metric.has_data,
            $metric.series_count,
            $metric.sample_count,
            (Format-Number $metric.min),
            (Format-Number $metric.max),
            (Format-Number $metric.last),
            (Escape-MarkdownCell $metric.query)
        ))
    }
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("## Interpretation")
    [void]$builder.AppendLine("")
    [void]$builder.AppendLine("- Use this report with the matching hotgroup summary and analysis report.")
    if ([string]$Summary.run_name -like "*coordinator*" -and [int64]$Summary.push.subscriber_count -eq 0) {
        [void]$builder.AppendLine("- Multi-runner coordinator summaries usually show `subscriber_count=0`; use the matching multirunner analysis report for total subscriber count and signal count.")
    }
    [void]$builder.AppendLine("- Metrics ending in `_5m` show the moving five-minute pressure window; metrics ending in `_window` approximate the whole captured run window.")
    [void]$builder.AppendLine("- Metrics with no data mean the exporter or scrape target did not expose that series in this window.")
    [void]$builder.AppendLine("- Do not use this single window as production capacity evidence.")

    $parent = Split-Path -Parent $Path
    if ($parent -and -not (Test-Path -LiteralPath $parent -PathType Container)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    Set-Content -LiteralPath $Path -Value ($builder.ToString().TrimEnd()) -Encoding UTF8
}

if (-not (Test-Path -LiteralPath $ResultDir -PathType Container)) {
    throw "ResultDir does not exist: $ResultDir"
}

$summaryPath = Join-Path $ResultDir "hotgroup-summary.json"
if (-not (Test-Path -LiteralPath $summaryPath -PathType Leaf)) {
    throw "Missing hotgroup summary: $summaryPath"
}

$summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
$runName = [string](Get-PropertyValue -Object $summary -Name "run_name")
if ($runName.Trim().Length -eq 0) {
    $runName = Split-Path -Leaf $ResultDir
}

$startedAt = [DateTimeOffset]::Parse([string](Get-PropertyValue -Object $summary -Name "started_at"))
$finishedAt = [DateTimeOffset]::Parse([string](Get-PropertyValue -Object $summary -Name "finished_at"))
$windowStart = $startedAt.AddMinutes(-1 * $PaddingMinutes)
$windowEnd = $finishedAt.AddMinutes($PaddingMinutes)

if ($OutputPath.Trim().Length -eq 0) {
    $OutputPath = Join-Path $ResultDir "hotgroup-prometheus-window.json"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path (Split-Path -Parent $PSScriptRoot) ("docs\runbook\loadtest\hotgroup\hotgroup-metrics-window-" + $runName + ".md")
}

$startUnix = $windowStart.ToUnixTimeSeconds()
$endUnix = $windowEnd.ToUnixTimeSeconds()
$windowSeconds = [Math]::Max(1, [int]($endUnix - $startUnix))
$metrics = New-Object System.Collections.Generic.List[object]
foreach ($definition in New-HotGroupMetricQueries -WindowSeconds $windowSeconds) {
    try {
        $series = Invoke-PrometheusRangeQuery -BaseUrl $PrometheusBaseUrl -Query $definition.query -Start $startUnix -End $endUnix -Step $StepSeconds
        $metrics.Add((Summarize-PrometheusResult -Name $definition.name -Query $definition.query -Series @($series)))
    }
    catch {
        $metrics.Add([pscustomobject]@{
            name = $definition.name
            query = $definition.query
            has_data = $false
            series_count = 0
            sample_count = 0
            min = $null
            max = $null
            last = $null
            error = $_.Exception.Message
        })
    }
}

$window = [pscustomobject]@{
    start_utc = $windowStart.UtcDateTime.ToString("o")
    end_utc = $windowEnd.UtcDateTime.ToString("o")
    start_unix = $startUnix
    end_unix = $endUnix
    padding_minutes = $PaddingMinutes
}

$payload = [pscustomobject]@{
    schema_version = 1
    run_name = $runName
    commit = [string](Get-PropertyValue -Object $summary -Name "commit")
    git_dirty = [bool](Get-PropertyValue -Object $summary -Name "git_dirty")
    result_dir = $ResultDir
    summary_path = $summaryPath
    prometheus_base_url = $PrometheusBaseUrl
    window = $window
    metrics = @($metrics.ToArray())
}

$payload | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $OutputPath -Encoding UTF8
Write-MetricsMarkdown -Path $MarkdownPath -Summary $summary -Window $window -Metrics @($metrics.ToArray())

Write-Host "Wrote hotgroup Prometheus window JSON: $OutputPath"
Write-Host "Wrote hotgroup Prometheus window report: $MarkdownPath"
