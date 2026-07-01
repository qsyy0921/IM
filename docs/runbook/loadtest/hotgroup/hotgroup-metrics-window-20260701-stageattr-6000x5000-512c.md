# Hot Group Metrics Window

Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.

- run_name: hotgroup-stageattr-6000x5000-512c-b9612a91-20260701-1740
- commit: b9612a9
- git_dirty: True
- result_dir: H:\NexusIM\loadtest-results\hotgroup-stageattr-6000x5000-512c-b9612a91-20260701-1740
- prometheus: http://172.31.50.2:19091
- window_start_utc: 2026-07-01T09:34:08.5998886Z
- window_end_utc: 2026-07-01T09:47:55.7015451Z
- step_seconds: 30

## Run Parameters

| field | value |
| --- | ---: |
| group_size | 6000 |
| message_count | 5000 |
| message_rate | 12000 |
| sender_count | 512 |
| subscriber_count | 0 |
| fanout_mode | READ_FANOUT |
| send_p95_ms | 462.274 |
| send_p99_ms | 546.306 |
| pull_p95_ms | 972.499 |
| conversation_signal_count | 0 |

## Prometheus Query Summary

| metric | has data | series | samples | min | max | last | query |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| core_targets_up | True | 1 | 28 | 7 | 7 | 7 | sum(up{job=~"nexusim-message-service\|nexusim-conversation-service\|nexusim-delivery-service\|nexusim-push-gateway"}) |
| message_send_p95_ms | True | 1 | 28 | 0 | 426.766 | 426.766 | max(nexusim_message_latency_p95_milliseconds{operation="send_message"}) |
| message_send_p99_ms | True | 1 | 28 | 0 | 478.528 | 478.528 | max(nexusim_message_latency_p99_milliseconds{operation="send_message"}) |
| message_send_p95_recent_ms | True | 1 | 28 | 0 | 233.888 | 233.888 | max(nexusim_message_latency_p95_milliseconds{operation="send_message_recent"}) |
| message_send_p99_recent_ms | True | 1 | 28 | 0 | 247.194 | 247.194 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_recent"}) |
| message_command_build_p99_recent_ms | True | 1 | 28 | 0 | 0.158 | 0.158 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_command_build_recent"}) |
| message_admission_p99_recent_ms | True | 1 | 28 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_admission_recent"}) |
| message_dependency_read_p99_recent_ms | True | 1 | 28 | 0 | 210.849 | 210.849 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_dependency_read_recent"}) |
| message_conversation_context_p99_recent_ms | True | 1 | 28 | 0 | 37.986 | 37.986 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_conversation_context_recent"}) |
| message_policy_check_p99_recent_ms | True | 1 | 28 | 0 | 190.723 | 190.723 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_policy_check_recent"}) |
| message_seq_floor_p99_recent_ms | True | 1 | 28 | 0 | 0.046 | 0.046 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_seq_floor_recent"}) |
| message_sequencer_allocate_p99_recent_ms | True | 1 | 28 | 0 | 31.605 | 31.605 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_sequencer_allocate_recent"}) |
| message_repository_append_call_p99_recent_ms | True | 1 | 28 | 0 | 120.568 | 120.568 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_repository_append_call_recent"}) |
| message_seq_alloc_p99_ms | True | 1 | 28 | 0 | 0.026 | 0.026 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"}) |
| message_seq_alloc_p99_recent_ms | True | 1 | 28 | 0 | 0.026 | 0.026 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc_recent"}) |
| message_repository_append_p99_ms | True | 1 | 28 | 0 | 157.565 | 157.565 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append"}) |
| message_repository_append_p99_recent_ms | True | 1 | 28 | 0 | 120.566 | 120.566 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append_recent"}) |
| message_repository_pool_acquire_p99_ms | True | 1 | 28 | 0 | 121.166 | 121.166 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire"}) |
| message_repository_pool_acquire_p99_recent_ms | True | 1 | 28 | 0 | 86.471 | 86.471 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire_recent"}) |
| message_repository_tx_begin_p99_ms | True | 1 | 28 | 0 | 14.853 | 14.853 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin"}) |
| message_repository_tx_begin_p99_recent_ms | True | 1 | 28 | 0 | 13.114 | 13.114 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin_recent"}) |
| message_repository_idempotency_lock_p99_ms | True | 1 | 28 | 0 | 9.448 | 9.448 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock"}) |
| message_repository_idempotency_lock_p99_recent_ms | True | 1 | 28 | 0 | 8.058 | 8.058 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock_recent"}) |
| message_repository_find_existing_p99_ms | True | 1 | 28 | 0 | 16.096 | 16.096 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing"}) |
| message_repository_find_existing_p99_recent_ms | True | 1 | 28 | 0 | 8.611 | 8.611 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing_recent"}) |
| message_repository_ensure_seq_p99_ms | True | 1 | 28 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq"}) |
| message_repository_ensure_seq_p99_recent_ms | True | 1 | 28 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq_recent"}) |
| message_repository_allocate_seq_p99_ms | True | 1 | 28 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq"}) |
| message_repository_allocate_seq_p99_recent_ms | True | 1 | 28 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq_recent"}) |
| message_repository_insert_message_p99_ms | True | 1 | 28 | 0 | 20.628 | 20.628 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message"}) |
| message_repository_insert_message_p99_recent_ms | True | 1 | 28 | 0 | 20.51 | 20.51 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message_recent"}) |
| message_repository_insert_timeline_p99_ms | True | 1 | 28 | 0 | 18.559 | 18.559 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline"}) |
| message_repository_insert_timeline_p99_recent_ms | True | 1 | 28 | 0 | 14.475 | 14.475 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline_recent"}) |
| message_repository_insert_outbox_p99_ms | True | 1 | 28 | 0 | 22.725 | 22.725 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox"}) |
| message_repository_insert_outbox_p99_recent_ms | True | 1 | 28 | 0 | 19.676 | 19.676 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox_recent"}) |
| message_repository_commit_p99_ms | True | 1 | 28 | 0 | 12.417 | 12.417 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit"}) |
| message_repository_commit_p99_recent_ms | True | 1 | 28 | 0 | 11.4 | 11.4 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit_recent"}) |
| message_outbox_fetched_per_call_avg | True | 1 | 28 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"}) |
| message_kafka_records_per_call_avg | True | 1 | 28 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"}) |
| conversation_grpc_rps | True | 1 | 28 | 0 | 38.596 | 0 | sum(rate(nexusim_conversation_grpc_method_requests_total[5m])) |
| delivery_outbox_pending | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending"}) |
| delivery_outbox_pending_ready | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending_ready"}) |
| delivery_outbox_dlq | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="dlq"}) |
| delivery_projection_unresolved | False | 0 | 0 |  |  |  | max(nexusim_delivery_projection_failures{state="unresolved_total"}) |
| delivery_timeline_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_timeline_worker_errors_total[5m])) |
| delivery_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[5m])) |
| delivery_grpc_rps | True | 1 | 28 | 0 | 0.063 | 0 | sum(rate(nexusim_delivery_grpc_method_requests_total[5m])) |
| delivery_grpc_errors_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_delivery_grpc_method_errors_total[5m])) |
| push_connected_sessions | True | 1 | 28 | 0 | 0 | 0 | max(nexusim_push_gateway_sessions{state="connected"}) |
| push_slow_evicted_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m])) |
| push_writer_events_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total[5m])) |
| push_writer_outbound_dequeued_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m])) |
| push_writer_frame_write_attempt_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m])) |
| push_writer_frame_write_success_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m])) |
| push_writer_frame_write_error_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m])) |
| push_writer_delivery_notify_attempt_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m])) |
| push_writer_delivery_notify_success_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m])) |
| push_writer_delivery_notify_error_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m])) |
| push_writer_frame_write_duration_p95_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_frame_write_duration_p99_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p95_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_avg_ms_5m | True | 1 | 28 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_duration_max_ms | True | 1 | 28 | 2.525 | 2.525 | 2.525 | max(nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"}) |
| push_writer_delivery_notify_queue_p95_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_avg_ms_5m | True | 1 | 28 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_queue_max_ms | True | 1 | 28 | 2.329 | 2.329 | 2.329 | max(nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds{operation="delivery_notify"}) |
| push_consumer_worker_errors_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m])) |
| push_redis_route_events_5m | True | 1 | 28 | 0 | 10000 | 1153.684 | sum(increase(nexusim_push_gateway_redis_route_events_total[5m])) |
| push_redis_registry_remote_matched_sessions_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m])) |
| push_redis_registry_remote_publish_call_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m])) |
| push_redis_registry_remote_publish_error_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m])) |
| push_redis_registry_remote_no_subscriber_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m])) |
| push_redis_registry_conversation_route_cache_hit_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[5m])) |
| push_redis_registry_conversation_route_cache_miss_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[5m])) |
| push_redis_registry_conversation_route_cache_invalidated_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_registry_remote_enqueued_sessions_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_matched_sessions_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_publish_call_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[5m])) |
| push_redis_delivery_consumer_remote_publish_error_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[5m])) |
| push_redis_delivery_consumer_remote_no_subscriber_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_no_subscriber"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_5m | True | 1 | 28 | 0 | 4575.789 | 548.421 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_5m | True | 1 | 28 | 0 | 687.368 | 58.947 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[5m])) |
| push_redis_subscriber_messages_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m])) |
| push_redis_subscriber_enqueued_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m])) |
| push_redis_subscriber_evicted_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m])) |
| push_redis_subscriber_errors_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queued_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_full_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[5m])) |
| push_redis_subscriber_signal_fanout_worker_errors_5m | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_depth | True | 1 | 28 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_avg_ms_5m | True | 1 | 28 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum{operation="conversation_signal"}[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count{operation="conversation_signal"}[5m])) |
| push_redis_subscriber_signal_fanout_duration_max_ms | True | 1 | 28 | 19.142 | 19.142 | 19.142 | max(nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds{operation="conversation_signal"}) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_5m | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_avg_ms_5m | True | 1 | 28 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count[5m])) |
| push_redis_subscriber_signal_fanout_queue_wait_max_ms | True | 1 | 28 | 0.053 | 0.053 | 0.053 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds) |
| push_writer_frame_write_success_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[827s])) |
| push_writer_frame_write_error_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[827s])) |
| push_writer_delivery_notify_success_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[827s])) |
| push_writer_delivery_notify_error_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[827s])) |
| push_writer_delivery_notify_duration_p95_ms_window | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[827s])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_window | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[827s])) by (le)) |
| push_writer_delivery_notify_queue_p95_ms_window | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[827s])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_window | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[827s])) by (le)) |
| push_redis_subscriber_messages_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[827s])) |
| push_redis_subscriber_enqueued_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[827s])) |
| push_redis_subscriber_evicted_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[827s])) |
| push_redis_subscriber_errors_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[827s])) |
| push_redis_registry_conversation_route_cache_hit_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[827s])) |
| push_redis_registry_conversation_route_cache_miss_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[827s])) |
| push_redis_registry_conversation_route_cache_invalidated_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[827s])) |
| push_redis_delivery_consumer_remote_matched_sessions_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[827s])) |
| push_redis_delivery_consumer_remote_publish_call_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[827s])) |
| push_redis_delivery_consumer_remote_publish_error_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[827s])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_window | True | 1 | 28 | 0 | 4438.233 | 4438.233 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[827s])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_window | True | 1 | 28 | 0 | 666.705 | 666.705 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[827s])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[827s])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[827s])) |
| push_redis_subscriber_signal_fanout_queued_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[827s])) |
| push_redis_subscriber_signal_fanout_queue_full_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[827s])) |
| push_redis_subscriber_signal_fanout_worker_errors_window | True | 1 | 28 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[827s])) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_window | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[827s])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_window | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[827s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_window | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[827s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_window | True | 1 | 28 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[827s])) by (le)) |
| message_pg_pool_conns | True | 1 | 28 | 72 | 72 | 72 | max(nexusim_message_pg_pool_conns) |
| conversation_pg_pool_conns | True | 1 | 28 | 32 | 32 | 32 | max(nexusim_conversation_pg_pool_conns) |
| delivery_pg_pool_conns | True | 1 | 28 | 16 | 16 | 16 | max(nexusim_delivery_pg_pool_conns) |

## Interpretation

- Use this report with the matching hotgroup summary and analysis report.
- Metrics ending in _5m show the moving five-minute pressure window; metrics ending in _window approximate the whole captured run window.
- Metrics with no data mean the exporter or scrape target did not expose that series in this window.
- Do not use this single window as production capacity evidence.
