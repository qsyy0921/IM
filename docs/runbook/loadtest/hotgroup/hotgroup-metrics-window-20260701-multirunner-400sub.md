# Hot Group Metrics Window

Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.

- run_name: hotgroup-multirunner-400sub-shard0-20260701-013557
- commit: 9e7d4f9
- git_dirty: False
- result_dir: H:\NexusIM\loadtest-results\hotgroup-multirunner-400sub-shard0-20260701-013557
- prometheus: http://172.31.50.2:19091
- window_start_utc: 2026-06-30T17:30:59.7704037Z
- window_end_utc: 2026-06-30T17:44:21.5228779Z
- step_seconds: 30

## Run Parameters

| field | value |
| --- | ---: |
| group_size | 6000 |
| message_count | 1000 |
| message_rate | 8000 |
| sender_count | 256 |
| subscriber_count | 100 |
| fanout_mode |  |
| send_p95_ms | 0 |
| send_p99_ms | 0 |
| pull_p95_ms | 0 |
| conversation_signal_count | 100000 |

## Prometheus Query Summary

| metric | has data | series | samples | min | max | last | query |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| core_targets_up | True | 1 | 27 | 4 | 4 | 4 | sum(up{job=~"nexusim-message-service\|nexusim-conversation-service\|nexusim-delivery-service\|nexusim-push-gateway"}) |
| message_send_p95_ms | True | 1 | 27 | 17.539 | 17.566 | 17.566 | max(nexusim_message_latency_p95_milliseconds{operation="send_message"}) |
| message_send_p99_ms | True | 1 | 27 | 21.591 | 21.592 | 21.591 | max(nexusim_message_latency_p99_milliseconds{operation="send_message"}) |
| message_seq_alloc_p99_ms | True | 1 | 27 | 1.336 | 1.338 | 1.338 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"}) |
| message_outbox_fetched_per_call_avg | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"}) |
| message_kafka_records_per_call_avg | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"}) |
| conversation_grpc_rps | True | 1 | 27 | 0 | 25.053 | 0 | sum(rate(nexusim_conversation_grpc_method_requests_total[5m])) |
| delivery_outbox_pending | True | 1 | 27 | 0 | 117 | 0 | max(nexusim_delivery_outbox{state="pending"}) |
| delivery_outbox_pending_ready | True | 1 | 27 | 0 | 117 | 0 | max(nexusim_delivery_outbox{state="pending_ready"}) |
| delivery_outbox_dlq | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_delivery_outbox{state="dlq"}) |
| delivery_projection_unresolved | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_delivery_projection_failures{state="unresolved_total"}) |
| delivery_timeline_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_timeline_worker_errors_total[5m])) |
| delivery_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[5m])) |
| delivery_grpc_rps | True | 1 | 27 | 0 | 0.131 | 0 | sum(rate(nexusim_delivery_grpc_method_requests_total[5m])) |
| delivery_grpc_errors_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_delivery_grpc_method_errors_total[5m])) |
| push_connected_sessions | True | 1 | 27 | 0 | 400 | 0 | max(nexusim_push_gateway_sessions{state="connected"}) |
| push_slow_evicted_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m])) |
| push_writer_events_5m | True | 1 | 27 | 0 | 2107368.421 | 319748.867 | sum(increase(nexusim_push_gateway_ws_writer_events_total[5m])) |
| push_writer_outbound_dequeued_5m | True | 1 | 27 | 0 | 421473.684 | 63949.773 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m])) |
| push_writer_frame_write_attempt_5m | True | 1 | 27 | 0 | 421894.737 | 63949.773 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m])) |
| push_writer_frame_write_success_5m | True | 1 | 27 | 0 | 421894.737 | 63949.773 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m])) |
| push_writer_frame_write_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m])) |
| push_writer_delivery_notify_attempt_5m | True | 1 | 27 | 0 | 421052.632 | 63949.773 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m])) |
| push_writer_delivery_notify_success_5m | True | 1 | 27 | 0 | 421052.632 | 63949.773 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m])) |
| push_writer_delivery_notify_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m])) |
| push_consumer_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m])) |
| push_redis_route_events_5m | True | 1 | 27 | 0 | 422105.263 | 64109.648 | sum(increase(nexusim_push_gateway_redis_route_events_total[5m])) |
| push_redis_registry_remote_matched_sessions_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m])) |
| push_redis_registry_remote_publish_call_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m])) |
| push_redis_registry_remote_publish_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m])) |
| push_redis_registry_remote_no_subscriber_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m])) |
| push_redis_registry_remote_enqueued_sessions_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m])) |
| push_redis_subscriber_messages_5m | True | 1 | 27 | 0 | 1052.632 | 159.874 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m])) |
| push_redis_subscriber_enqueued_5m | True | 1 | 27 | 0 | 421052.632 | 63949.773 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m])) |
| push_redis_subscriber_evicted_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m])) |
| push_redis_subscriber_errors_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m])) |
| push_writer_frame_write_success_window | True | 1 | 27 | 0 | 413089.756 | 413089.756 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[802s])) |
| push_writer_frame_write_error_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[802s])) |
| push_writer_delivery_notify_success_window | True | 1 | 27 | 0 | 412265.225 | 412265.225 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[802s])) |
| push_writer_delivery_notify_error_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[802s])) |
| push_redis_subscriber_messages_window | True | 1 | 27 | 0 | 1030.663 | 1030.663 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[802s])) |
| push_redis_subscriber_enqueued_window | True | 1 | 27 | 0 | 412265.225 | 412265.225 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[802s])) |
| push_redis_subscriber_evicted_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[802s])) |
| push_redis_subscriber_errors_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[802s])) |
| message_pg_pool_conns | True | 1 | 27 | 72 | 72 | 72 | max(nexusim_message_pg_pool_conns) |
| conversation_pg_pool_conns | True | 1 | 27 | 72 | 72 | 72 | max(nexusim_conversation_pg_pool_conns) |
| delivery_pg_pool_conns | True | 1 | 27 | 72 | 72 | 72 | max(nexusim_delivery_pg_pool_conns) |

## Interpretation

- Use this report with the matching hotgroup summary and analysis report.
- Metrics ending in _5m show the moving five-minute pressure window; metrics ending in _window approximate the whole captured run window.
- Metrics with no data mean the exporter or scrape target did not expose that series in this window.
- Do not use this single window as production capacity evidence.
