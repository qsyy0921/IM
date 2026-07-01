# Hot Group Metrics Window

Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.

- run_name: hotgroup-readseq-clean-6000x1000-256c-6a4673b6-20260701-1635
- commit: 6a4673b
- git_dirty: False
- result_dir: H:\NexusIM\loadtest-results\hotgroup-readseq-clean-6000x1000-256c-6a4673b6-20260701-1635
- prometheus: http://172.31.50.2:19091
- window_start_utc: 2026-07-01T08:29:40.9724348Z
- window_end_utc: 2026-07-01T08:41:01.6729843Z
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
| send_p95_ms | 208.507 |
| send_p99_ms | 220.367 |
| pull_p95_ms | 132.531 |
| conversation_signal_count | 0 |

## Run-Local vs Prometheus Gauge Caveat

本轮 run-local summary 显示 SendMessage p95 / p99 为 `208.507ms / 220.367ms`。
下方 Prometheus `nexusim_message_latency_p*_milliseconds` 仍保留上一轮失败 run 的
2-3s 历史 gauge 值，因此不能直接把这些 gauge 当成本轮 run-local 延迟。
后续需要改成 run-window delta histogram、复压前重置相关服务，或在报告中只使用
`hotgroup-summary.json` 的 run-local latency。

## Prometheus Query Summary

| metric | has data | series | samples | min | max | last | query |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| core_targets_up | True | 1 | 23 | 7 | 7 | 7 | sum(up{job=~"nexusim-message-service\|nexusim-conversation-service\|nexusim-delivery-service\|nexusim-push-gateway"}) |
| message_send_p95_ms | True | 1 | 23 | 2104.62 | 2504.433 | 2104.62 | max(nexusim_message_latency_p95_milliseconds{operation="send_message"}) |
| message_send_p99_ms | True | 1 | 23 | 2983.872 | 2988.805 | 2983.872 | max(nexusim_message_latency_p99_milliseconds{operation="send_message"}) |
| message_seq_alloc_p99_ms | True | 1 | 23 | 2228.181 | 2366.488 | 2228.181 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"}) |
| message_repository_append_p99_ms | True | 1 | 23 | 2741.621 | 2801.021 | 2741.621 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append"}) |
| message_repository_pool_acquire_p99_ms | True | 1 | 23 | 591.725 | 596.926 | 591.725 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire"}) |
| message_repository_tx_begin_p99_ms | True | 1 | 23 | 5.206 | 5.869 | 5.869 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin"}) |
| message_repository_idempotency_lock_p99_ms | True | 1 | 23 | 15.183 | 18.694 | 15.183 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock"}) |
| message_repository_find_existing_p99_ms | True | 1 | 23 | 17.538 | 21.473 | 17.538 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing"}) |
| message_repository_ensure_seq_p99_ms | True | 1 | 23 | 1180.48 | 1180.48 | 1180.48 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq"}) |
| message_repository_allocate_seq_p99_ms | True | 1 | 23 | 2350.794 | 2350.794 | 2350.794 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq"}) |
| message_repository_insert_message_p99_ms | True | 1 | 23 | 4.246 | 15.998 | 15.998 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message"}) |
| message_repository_insert_timeline_p99_ms | True | 1 | 23 | 2.851 | 15.624 | 15.624 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline"}) |
| message_repository_insert_outbox_p99_ms | True | 1 | 23 | 6.24 | 23.658 | 23.658 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox"}) |
| message_repository_commit_p99_ms | True | 1 | 23 | 5.567 | 12.514 | 12.514 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit"}) |
| message_outbox_fetched_per_call_avg | True | 1 | 23 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"}) |
| message_kafka_records_per_call_avg | True | 1 | 23 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"}) |
| conversation_grpc_rps | True | 1 | 23 | 0 | 24.561 | 0 | sum(rate(nexusim_conversation_grpc_method_requests_total[5m])) |
| delivery_outbox_pending | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending"}) |
| delivery_outbox_pending_ready | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending_ready"}) |
| delivery_outbox_dlq | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="dlq"}) |
| delivery_projection_unresolved | False | 0 | 0 |  |  |  | max(nexusim_delivery_projection_failures{state="unresolved_total"}) |
| delivery_timeline_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_timeline_worker_errors_total[5m])) |
| delivery_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[5m])) |
| delivery_grpc_rps | True | 1 | 23 | 0 | 0.066 | 0 | sum(rate(nexusim_delivery_grpc_method_requests_total[5m])) |
| delivery_grpc_errors_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_delivery_grpc_method_errors_total[5m])) |
| push_connected_sessions | True | 1 | 23 | 0 | 0 | 0 | max(nexusim_push_gateway_sessions{state="connected"}) |
| push_slow_evicted_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m])) |
| push_writer_events_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total[5m])) |
| push_writer_outbound_dequeued_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m])) |
| push_writer_frame_write_attempt_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m])) |
| push_writer_frame_write_success_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m])) |
| push_writer_frame_write_error_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m])) |
| push_writer_delivery_notify_attempt_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m])) |
| push_writer_delivery_notify_success_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m])) |
| push_writer_delivery_notify_error_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m])) |
| push_writer_frame_write_duration_p95_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_frame_write_duration_p99_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p95_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_avg_ms_5m | True | 1 | 23 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_duration_max_ms | True | 1 | 23 | 2.525 | 2.525 | 2.525 | max(nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"}) |
| push_writer_delivery_notify_queue_p95_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_avg_ms_5m | True | 1 | 23 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_queue_max_ms | True | 1 | 23 | 2.329 | 2.329 | 2.329 | max(nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds{operation="delivery_notify"}) |
| push_consumer_worker_errors_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m])) |
| push_redis_route_events_5m | True | 1 | 23 | 0 | 2000 | 1662.898 | sum(increase(nexusim_push_gateway_redis_route_events_total[5m])) |
| push_redis_registry_remote_matched_sessions_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m])) |
| push_redis_registry_remote_publish_call_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m])) |
| push_redis_registry_remote_publish_error_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m])) |
| push_redis_registry_remote_no_subscriber_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m])) |
| push_redis_registry_conversation_route_cache_hit_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[5m])) |
| push_redis_registry_conversation_route_cache_miss_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[5m])) |
| push_redis_registry_conversation_route_cache_invalidated_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_registry_remote_enqueued_sessions_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_matched_sessions_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_publish_call_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[5m])) |
| push_redis_delivery_consumer_remote_publish_error_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[5m])) |
| push_redis_delivery_consumer_remote_no_subscriber_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_no_subscriber"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_5m | True | 1 | 23 | 0 | 953.684 | 793.538 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_5m | True | 1 | 23 | 0 | 98.947 | 82.054 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[5m])) |
| push_redis_subscriber_messages_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m])) |
| push_redis_subscriber_enqueued_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m])) |
| push_redis_subscriber_evicted_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m])) |
| push_redis_subscriber_errors_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queued_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_full_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[5m])) |
| push_redis_subscriber_signal_fanout_worker_errors_5m | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_depth | True | 1 | 23 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_avg_ms_5m | True | 1 | 23 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum{operation="conversation_signal"}[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count{operation="conversation_signal"}[5m])) |
| push_redis_subscriber_signal_fanout_duration_max_ms | True | 1 | 23 | 19.142 | 19.142 | 19.142 | max(nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds{operation="conversation_signal"}) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_5m | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_avg_ms_5m | True | 1 | 23 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count[5m])) |
| push_redis_subscriber_signal_fanout_queue_wait_max_ms | True | 1 | 23 | 0.053 | 0.053 | 0.053 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds) |
| push_writer_frame_write_success_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[681s])) |
| push_writer_frame_write_error_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[681s])) |
| push_writer_delivery_notify_success_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[681s])) |
| push_writer_delivery_notify_error_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[681s])) |
| push_writer_delivery_notify_duration_p95_ms_window | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[681s])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_window | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[681s])) by (le)) |
| push_writer_delivery_notify_queue_p95_ms_window | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[681s])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_window | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[681s])) by (le)) |
| push_redis_subscriber_messages_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[681s])) |
| push_redis_subscriber_enqueued_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[681s])) |
| push_redis_subscriber_evicted_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[681s])) |
| push_redis_subscriber_errors_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[681s])) |
| push_redis_registry_conversation_route_cache_hit_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[681s])) |
| push_redis_registry_conversation_route_cache_miss_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[681s])) |
| push_redis_registry_conversation_route_cache_invalidated_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[681s])) |
| push_redis_delivery_consumer_remote_matched_sessions_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[681s])) |
| push_redis_delivery_consumer_remote_publish_call_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[681s])) |
| push_redis_delivery_consumer_remote_publish_error_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[681s])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_window | True | 1 | 23 | 0 | 934.827 | 927.866 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[681s])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_window | True | 1 | 23 | 0 | 96.991 | 96.269 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[681s])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[681s])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[681s])) |
| push_redis_subscriber_signal_fanout_queued_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[681s])) |
| push_redis_subscriber_signal_fanout_queue_full_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[681s])) |
| push_redis_subscriber_signal_fanout_worker_errors_window | True | 1 | 23 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[681s])) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_window | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[681s])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_window | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[681s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_window | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[681s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_window | True | 1 | 23 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[681s])) by (le)) |
| message_pg_pool_conns | True | 1 | 23 | 192 | 192 | 192 | max(nexusim_message_pg_pool_conns) |
| conversation_pg_pool_conns | True | 1 | 23 | 32 | 32 | 32 | max(nexusim_conversation_pg_pool_conns) |
| delivery_pg_pool_conns | True | 1 | 23 | 16 | 16 | 16 | max(nexusim_delivery_pg_pool_conns) |

## Interpretation

- Use this report with the matching hotgroup summary and analysis report.
- Metrics ending in _5m show the moving five-minute pressure window; metrics ending in _window approximate the whole captured run window.
- Metrics with no data mean the exporter or scrape target did not expose that series in this window.
- Do not use this single window as production capacity evidence.
