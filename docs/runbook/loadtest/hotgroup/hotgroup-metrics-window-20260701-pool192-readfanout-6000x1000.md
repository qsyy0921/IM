# Hot Group Metrics Window

Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.

- run_name: hotgroup-pool192-readfanout-6000x1000-256c-058a5ee5-20260701-1620
- commit: 058a5ee
- git_dirty: False
- result_dir: H:\NexusIM\loadtest-results\hotgroup-pool192-readfanout-6000x1000-256c-058a5ee5-20260701-1620
- prometheus: http://172.31.50.2:19091
- window_start_utc: 2026-07-01T08:13:42.5632709Z
- window_end_utc: 2026-07-01T08:24:38.9505989Z
- step_seconds: 30

## Run Parameters

| field | value |
| --- | ---: |
| group_size | 6000 |
| message_count | 1000 |
| message_rate | 8000 |
| sender_count | 256 |
| subscriber_count | 0 |
| fanout_mode | READ_FANOUT |
| send_p95_ms | 2511.305 |
| send_p99_ms | 3000.288 |
| pull_p95_ms | 0 |
| conversation_signal_count | 0 |

## Prometheus Query Summary

| metric | has data | series | samples | min | max | last | query |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| core_targets_up | True | 1 | 22 | 7 | 7 | 7 | sum(up{job=~"nexusim-message-service\|nexusim-conversation-service\|nexusim-delivery-service\|nexusim-push-gateway"}) |
| message_send_p95_ms | True | 1 | 22 | 0 | 2504.433 | 2504.433 | max(nexusim_message_latency_p95_milliseconds{operation="send_message"}) |
| message_send_p99_ms | True | 1 | 22 | 0 | 2988.805 | 2988.805 | max(nexusim_message_latency_p99_milliseconds{operation="send_message"}) |
| message_seq_alloc_p99_ms | True | 1 | 22 | 0 | 2366.488 | 2366.488 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"}) |
| message_repository_append_p99_ms | True | 1 | 22 | 0 | 2801.021 | 2801.021 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append"}) |
| message_repository_pool_acquire_p99_ms | True | 1 | 22 | 0 | 695.913 | 596.926 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire"}) |
| message_repository_tx_begin_p99_ms | True | 1 | 22 | 0 | 5.206 | 5.206 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin"}) |
| message_repository_idempotency_lock_p99_ms | True | 1 | 22 | 0 | 18.694 | 18.694 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock"}) |
| message_repository_find_existing_p99_ms | True | 1 | 22 | 0 | 21.473 | 21.473 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing"}) |
| message_repository_ensure_seq_p99_ms | True | 1 | 22 | 0 | 1180.48 | 1180.48 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq"}) |
| message_repository_allocate_seq_p99_ms | True | 1 | 22 | 0 | 2350.794 | 2350.794 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq"}) |
| message_repository_insert_message_p99_ms | True | 1 | 22 | 0 | 4.246 | 4.246 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message"}) |
| message_repository_insert_timeline_p99_ms | True | 1 | 22 | 0 | 2.851 | 2.851 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline"}) |
| message_repository_insert_outbox_p99_ms | True | 1 | 22 | 0 | 6.24 | 6.24 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox"}) |
| message_repository_commit_p99_ms | True | 1 | 22 | 0 | 5.567 | 5.567 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit"}) |
| message_outbox_fetched_per_call_avg | True | 1 | 22 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"}) |
| message_kafka_records_per_call_avg | True | 1 | 22 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"}) |
| conversation_grpc_rps | True | 1 | 22 | 0 | 25.358 | 12.474 | sum(rate(nexusim_conversation_grpc_method_requests_total[5m])) |
| delivery_outbox_pending | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending"}) |
| delivery_outbox_pending_ready | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending_ready"}) |
| delivery_outbox_dlq | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="dlq"}) |
| delivery_projection_unresolved | False | 0 | 0 |  |  |  | max(nexusim_delivery_projection_failures{state="unresolved_total"}) |
| delivery_timeline_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_timeline_worker_errors_total[5m])) |
| delivery_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[5m])) |
| delivery_grpc_rps | True | 1 | 22 | 0 | 0.063 | 0 | sum(rate(nexusim_delivery_grpc_method_requests_total[5m])) |
| delivery_grpc_errors_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_delivery_grpc_method_errors_total[5m])) |
| push_connected_sessions | True | 1 | 22 | 0 | 0 | 0 | max(nexusim_push_gateway_sessions{state="connected"}) |
| push_slow_evicted_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m])) |
| push_writer_events_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total[5m])) |
| push_writer_outbound_dequeued_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m])) |
| push_writer_frame_write_attempt_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m])) |
| push_writer_frame_write_success_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m])) |
| push_writer_frame_write_error_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m])) |
| push_writer_delivery_notify_attempt_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m])) |
| push_writer_delivery_notify_success_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m])) |
| push_writer_delivery_notify_error_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m])) |
| push_writer_frame_write_duration_p95_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_frame_write_duration_p99_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p95_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_avg_ms_5m | True | 1 | 22 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_duration_max_ms | True | 1 | 22 | 2.525 | 2.525 | 2.525 | max(nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"}) |
| push_writer_delivery_notify_queue_p95_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_avg_ms_5m | True | 1 | 22 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_queue_max_ms | True | 1 | 22 | 2.329 | 2.329 | 2.329 | max(nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds{operation="delivery_notify"}) |
| push_consumer_worker_errors_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m])) |
| push_redis_route_events_5m | True | 1 | 22 | 0 | 2734.388 | 2734.388 | sum(increase(nexusim_push_gateway_redis_route_events_total[5m])) |
| push_redis_registry_remote_matched_sessions_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m])) |
| push_redis_registry_remote_publish_call_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m])) |
| push_redis_registry_remote_publish_error_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m])) |
| push_redis_registry_remote_no_subscriber_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m])) |
| push_redis_registry_conversation_route_cache_hit_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[5m])) |
| push_redis_registry_conversation_route_cache_miss_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[5m])) |
| push_redis_registry_conversation_route_cache_invalidated_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_registry_remote_enqueued_sessions_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_matched_sessions_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_publish_call_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[5m])) |
| push_redis_delivery_consumer_remote_publish_error_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[5m])) |
| push_redis_delivery_consumer_remote_no_subscriber_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_no_subscriber"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_5m | True | 1 | 22 | 0 | 1306.218 | 1306.218 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_5m | True | 1 | 22 | 0 | 132.238 | 132.238 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[5m])) |
| push_redis_subscriber_messages_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m])) |
| push_redis_subscriber_enqueued_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m])) |
| push_redis_subscriber_evicted_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m])) |
| push_redis_subscriber_errors_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queued_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_full_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[5m])) |
| push_redis_subscriber_signal_fanout_worker_errors_5m | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_depth | True | 1 | 22 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_avg_ms_5m | True | 1 | 22 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum{operation="conversation_signal"}[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count{operation="conversation_signal"}[5m])) |
| push_redis_subscriber_signal_fanout_duration_max_ms | True | 1 | 22 | 19.142 | 19.142 | 19.142 | max(nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds{operation="conversation_signal"}) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_5m | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_avg_ms_5m | True | 1 | 22 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count[5m])) |
| push_redis_subscriber_signal_fanout_queue_wait_max_ms | True | 1 | 22 | 0.053 | 0.053 | 0.053 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds) |
| push_writer_frame_write_success_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[656s])) |
| push_writer_frame_write_error_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[656s])) |
| push_writer_delivery_notify_success_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[656s])) |
| push_writer_delivery_notify_error_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[656s])) |
| push_writer_delivery_notify_duration_p95_ms_window | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[656s])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_window | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[656s])) by (le)) |
| push_writer_delivery_notify_queue_p95_ms_window | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[656s])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_window | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[656s])) by (le)) |
| push_redis_subscriber_messages_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[656s])) |
| push_redis_subscriber_enqueued_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[656s])) |
| push_redis_subscriber_evicted_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[656s])) |
| push_redis_subscriber_errors_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[656s])) |
| push_redis_registry_conversation_route_cache_hit_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[656s])) |
| push_redis_registry_conversation_route_cache_miss_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[656s])) |
| push_redis_registry_conversation_route_cache_invalidated_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[656s])) |
| push_redis_delivery_consumer_remote_matched_sessions_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[656s])) |
| push_redis_delivery_consumer_remote_publish_call_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[656s])) |
| push_redis_delivery_consumer_remote_publish_error_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[656s])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_window | True | 1 | 22 | 919.745 | 1850.367 | 926.577 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[656s])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_window | True | 1 | 22 | 93.113 | 189.675 | 93.804 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[656s])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[656s])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[656s])) |
| push_redis_subscriber_signal_fanout_queued_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[656s])) |
| push_redis_subscriber_signal_fanout_queue_full_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[656s])) |
| push_redis_subscriber_signal_fanout_worker_errors_window | True | 1 | 22 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[656s])) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_window | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[656s])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_window | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[656s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_window | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[656s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_window | True | 1 | 22 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[656s])) by (le)) |
| message_pg_pool_conns | True | 1 | 22 | 64 | 192 | 192 | max(nexusim_message_pg_pool_conns) |
| conversation_pg_pool_conns | True | 1 | 22 | 32 | 32 | 32 | max(nexusim_conversation_pg_pool_conns) |
| delivery_pg_pool_conns | True | 1 | 22 | 16 | 16 | 16 | max(nexusim_delivery_pg_pool_conns) |

## Interpretation

- Use this report with the matching hotgroup summary and analysis report.
- Metrics ending in _5m show the moving five-minute pressure window; metrics ending in _window approximate the whole captured run window.
- Metrics with no data mean the exporter or scrape target did not expose that series in this window.
- Do not use this single window as production capacity evidence.
