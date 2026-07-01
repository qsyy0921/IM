# Hot Group Metrics Window

Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.

- run_name: hotgroup-fanoutpolicy-clean-400sub-5000msg-coordinator-20260701-084638
- commit: 37b575e
- git_dirty: False
- result_dir: H:\NexusIM\loadtest-results\hotgroup-fanoutpolicy-clean-400sub-5000msg-coordinator-20260701-084638
- prometheus: http://172.31.50.2:19091
- window_start_utc: 2026-07-01T00:41:51.3224202Z
- window_end_utc: 2026-07-01T00:55:00.7170795Z
- step_seconds: 30

## Run Parameters

| field | value |
| --- | ---: |
| group_size | 6000 |
| message_count | 5000 |
| message_rate | 8000 |
| sender_count | 256 |
| subscriber_count | 0 |
| fanout_mode | READ_FANOUT |
| send_p95_ms | 21.055 |
| send_p99_ms | 26.145 |
| pull_p95_ms | 68.047 |
| conversation_signal_count | 0 |

## Prometheus Query Summary

| metric | has data | series | samples | min | max | last | query |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| core_targets_up | True | 1 | 27 | 7 | 7 | 7 | sum(up{job=~"nexusim-message-service\|nexusim-conversation-service\|nexusim-delivery-service\|nexusim-push-gateway"}) |
| message_send_p95_ms | True | 1 | 27 | 17.456 | 17.902 | 17.9 | max(nexusim_message_latency_p95_milliseconds{operation="send_message"}) |
| message_send_p99_ms | True | 1 | 27 | 21.167 | 21.592 | 21.579 | max(nexusim_message_latency_p99_milliseconds{operation="send_message"}) |
| message_seq_alloc_p99_ms | True | 1 | 27 | 1.295 | 1.309 | 1.295 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"}) |
| message_outbox_fetched_per_call_avg | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"}) |
| message_kafka_records_per_call_avg | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"}) |
| conversation_grpc_rps | True | 1 | 27 | 0 | 38.63 | 0 | sum(rate(nexusim_conversation_grpc_method_requests_total[5m])) |
| delivery_outbox_pending | True | 1 | 27 | 0 | 1721 | 0 | max(nexusim_delivery_outbox{state="pending"}) |
| delivery_outbox_pending_ready | True | 1 | 27 | 0 | 1721 | 0 | max(nexusim_delivery_outbox{state="pending_ready"}) |
| delivery_outbox_dlq | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_delivery_outbox{state="dlq"}) |
| delivery_projection_unresolved | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_delivery_projection_failures{state="unresolved_total"}) |
| delivery_timeline_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_timeline_worker_errors_total[5m])) |
| delivery_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[5m])) |
| delivery_grpc_rps | True | 1 | 27 | 0 | 0.137 | 0 | sum(rate(nexusim_delivery_grpc_method_requests_total[5m])) |
| delivery_grpc_errors_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_delivery_grpc_method_errors_total[5m])) |
| push_connected_sessions | True | 1 | 27 | 0 | 100 | 0 | max(nexusim_push_gateway_sessions{state="connected"}) |
| push_slow_evicted_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m])) |
| push_writer_events_5m | True | 1 | 27 | 0 | 1043666.667 | 10833.75 | sum(increase(nexusim_push_gateway_ws_writer_events_total[5m])) |
| push_writer_outbound_dequeued_5m | True | 1 | 27 | 0 | 208733.333 | 2166.75 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m])) |
| push_writer_frame_write_attempt_5m | True | 1 | 27 | 0 | 209133.333 | 2166.75 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m])) |
| push_writer_frame_write_success_5m | True | 1 | 27 | 0 | 209133.333 | 2166.75 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m])) |
| push_writer_frame_write_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m])) |
| push_writer_delivery_notify_attempt_5m | True | 1 | 27 | 0 | 208333.333 | 2166.75 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m])) |
| push_writer_delivery_notify_success_5m | True | 1 | 27 | 0 | 208333.333 | 2166.75 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m])) |
| push_writer_delivery_notify_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m])) |
| push_writer_frame_write_duration_p95_ms_5m | True | 1 | 27 | NaN | NaN | 0.366 | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_frame_write_duration_p99_ms_5m | True | 1 | 27 | NaN | NaN | 0.615 | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p95_ms_5m | True | 1 | 27 | NaN | NaN | 0.366 | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_5m | True | 1 | 27 | NaN | NaN | 0.615 | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_avg_ms_5m | True | 1 | 27 | NaN | NaN | 0.142 | sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_duration_max_ms | True | 1 | 27 | 0 | 4.445 | 4.125 | max(nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"}) |
| push_writer_delivery_notify_queue_p95_ms_5m | True | 1 | 27 | NaN | NaN | 3.109 | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_5m | True | 1 | 27 | NaN | NaN | 4.622 | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_avg_ms_5m | True | 1 | 27 | NaN | NaN | 1.313 | sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_queue_max_ms | True | 1 | 27 | 0 | 6.38 | 5.034 | max(nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds{operation="delivery_notify"}) |
| push_consumer_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m])) |
| push_redis_route_events_5m | True | 1 | 27 | 0 | 212500 | 2210.085 | sum(increase(nexusim_push_gateway_redis_route_events_total[5m])) |
| push_redis_registry_remote_matched_sessions_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m])) |
| push_redis_registry_remote_publish_call_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m])) |
| push_redis_registry_remote_publish_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m])) |
| push_redis_registry_remote_no_subscriber_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m])) |
| push_redis_registry_remote_enqueued_sessions_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m])) |
| push_redis_subscriber_messages_5m | True | 1 | 27 | 0 | 2083.333 | 21.668 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m])) |
| push_redis_subscriber_enqueued_5m | True | 1 | 27 | 0 | 208333.333 | 2166.75 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m])) |
| push_redis_subscriber_evicted_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m])) |
| push_redis_subscriber_errors_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queued_5m | True | 1 | 27 | 0 | 2083.333 | 21.668 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_full_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[5m])) |
| push_redis_subscriber_signal_fanout_worker_errors_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_depth | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_5m | True | 1 | 27 | NaN | NaN | 62.5 | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_5m | True | 1 | 27 | NaN | NaN | 92.5 | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_avg_ms_5m | True | 1 | 27 | NaN | NaN | 14.058 | sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum{operation="conversation_signal"}[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count{operation="conversation_signal"}[5m])) |
| push_redis_subscriber_signal_fanout_duration_max_ms | True | 1 | 27 | 0 | 75.39 | 74.996 | max(nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds{operation="conversation_signal"}) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_5m | True | 1 | 27 | NaN | NaN | 0.095 | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_5m | True | 1 | 27 | NaN | NaN | 0.099 | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_avg_ms_5m | True | 1 | 27 | NaN | NaN | 0.014 | sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count[5m])) |
| push_redis_subscriber_signal_fanout_queue_wait_max_ms | True | 1 | 27 | 0 | 0.132 | 0.132 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds) |
| push_writer_frame_write_success_window | True | 1 | 27 | 0 | 206544.88 | 206544.88 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[789s])) |
| push_writer_frame_write_error_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[789s])) |
| push_writer_delivery_notify_success_window | True | 1 | 27 | 0 | 205721.992 | 205721.992 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[789s])) |
| push_writer_delivery_notify_error_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[789s])) |
| push_writer_delivery_notify_duration_p95_ms_window | True | 1 | 27 | NaN | NaN | 0.465 | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[789s])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_window | True | 1 | 27 | NaN | NaN | 0.893 | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[789s])) by (le)) |
| push_writer_delivery_notify_queue_p95_ms_window | True | 1 | 27 | NaN | NaN | 4.071 | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[789s])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_window | True | 1 | 27 | NaN | NaN | 4.814 | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[789s])) by (le)) |
| push_redis_subscriber_messages_window | True | 1 | 27 | 0 | 2057.22 | 2057.22 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[789s])) |
| push_redis_subscriber_enqueued_window | True | 1 | 27 | 0 | 205721.992 | 205721.992 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[789s])) |
| push_redis_subscriber_evicted_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[789s])) |
| push_redis_subscriber_errors_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[789s])) |
| push_redis_subscriber_signal_fanout_queued_window | True | 1 | 27 | 0 | 2057.22 | 2057.22 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[789s])) |
| push_redis_subscriber_signal_fanout_queue_full_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[789s])) |
| push_redis_subscriber_signal_fanout_worker_errors_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[789s])) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_window | True | 1 | 27 | NaN | NaN | 39.944 | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[789s])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_window | True | 1 | 27 | NaN | NaN | 84.614 | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[789s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_window | True | 1 | 27 | NaN | NaN | 0.095 | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[789s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_window | True | 1 | 27 | NaN | NaN | 0.099 | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[789s])) by (le)) |
| message_pg_pool_conns | True | 1 | 27 | 72 | 72 | 72 | max(nexusim_message_pg_pool_conns) |
| conversation_pg_pool_conns | True | 1 | 27 | 72 | 72 | 72 | max(nexusim_conversation_pg_pool_conns) |
| delivery_pg_pool_conns | True | 1 | 27 | 72 | 72 | 72 | max(nexusim_delivery_pg_pool_conns) |

## Interpretation

- Use this report with the matching hotgroup summary and analysis report.
- Metrics ending in _5m show the moving five-minute pressure window; metrics ending in _window approximate the whole captured run window.
- Metrics with no data mean the exporter or scrape target did not expose that series in this window.
- Do not use this single window as production capacity evidence.
