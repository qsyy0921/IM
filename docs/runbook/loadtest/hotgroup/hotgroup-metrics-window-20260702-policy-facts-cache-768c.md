# Hot Group Metrics Window

Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.

- run_name: hotgroup-policy-facts-cache-6000x5000-768c-e680fb61-20260702-1455
- commit: e680fb6
- git_dirty: False
- result_dir: H:\NexusIM\loadtest-results\hotgroup-policy-facts-cache-6000x5000-768c-e680fb61-20260702-1455
- prometheus: http://172.31.50.2:19091
- window_start_utc: 2026-07-02T06:50:35.8434300Z
- window_end_utc: 2026-07-02T07:04:04.3161427Z
- step_seconds: 30

## Run Parameters

| field | value |
| --- | ---: |
| group_size | 6000 |
| message_count | 5000 |
| message_rate | 16000 |
| sender_count | 768 |
| subscriber_count | 0 |
| fanout_mode | READ_FANOUT |
| send_p95_ms | 645.709 |
| send_p99_ms | 716.758 |
| pull_p95_ms | 1622.663 |
| conversation_signal_count | 0 |

## Prometheus Query Summary

| metric | has data | series | samples | min | max | last | query |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| core_targets_up | True | 1 | 27 | 8 | 8 | 8 | sum(up{job=~"nexusim-message-service\|nexusim-conversation-service\|nexusim-policy-service\|nexusim-delivery-service\|nexusim-push-gateway"}) |
| message_send_p95_ms | True | 1 | 27 | 0 | 583.343 | 583.343 | max(nexusim_message_latency_p95_milliseconds{operation="send_message"}) |
| message_send_p99_ms | True | 1 | 27 | 0 | 639.812 | 639.812 | max(nexusim_message_latency_p99_milliseconds{operation="send_message"}) |
| message_send_p95_recent_ms | True | 1 | 27 | 0 | 341.762 | 341.762 | max(nexusim_message_latency_p95_milliseconds{operation="send_message_recent"}) |
| message_send_p99_recent_ms | True | 1 | 27 | 0 | 366.077 | 366.077 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_recent"}) |
| message_command_build_p99_recent_ms | True | 1 | 27 | 0 | 0.145 | 0.145 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_command_build_recent"}) |
| message_admission_p99_recent_ms | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_admission_recent"}) |
| message_dependency_read_p99_recent_ms | True | 1 | 27 | 0 | 328.018 | 328.018 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_dependency_read_recent"}) |
| message_conversation_context_p99_recent_ms | True | 1 | 27 | 0 | 27.791 | 27.791 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_conversation_context_recent"}) |
| message_policy_check_p99_recent_ms | True | 1 | 27 | 0 | 312.913 | 312.913 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_policy_check_recent"}) |
| message_seq_floor_p99_recent_ms | True | 1 | 27 | 0 | 0.025 | 0.025 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_seq_floor_recent"}) |
| message_sequencer_allocate_p99_recent_ms | True | 1 | 27 | 0 | 29.171 | 29.171 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_sequencer_allocate_recent"}) |
| message_repository_append_call_p99_recent_ms | True | 1 | 27 | 0 | 52.52 | 52.52 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_repository_append_call_recent"}) |
| message_seq_alloc_p99_ms | True | 1 | 27 | 0 | 0.028 | 0.028 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"}) |
| message_seq_alloc_p99_recent_ms | True | 1 | 27 | 0 | 0.028 | 0.028 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc_recent"}) |
| message_repository_append_p99_ms | True | 1 | 27 | 0 | 155.395 | 155.395 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append"}) |
| message_repository_append_p99_recent_ms | True | 1 | 27 | 0 | 52.508 | 52.508 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append_recent"}) |
| message_repository_pool_acquire_p99_ms | True | 1 | 27 | 0 | 101.864 | 101.864 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire"}) |
| message_repository_pool_acquire_p99_recent_ms | True | 1 | 27 | 0 | 24.976 | 24.976 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire_recent"}) |
| message_repository_tx_begin_p99_ms | True | 1 | 27 | 0 | 5.824 | 5.824 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin"}) |
| message_repository_tx_begin_p99_recent_ms | True | 1 | 27 | 0 | 5.724 | 5.724 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin_recent"}) |
| message_repository_idempotency_lock_p99_ms | True | 1 | 27 | 0 | 8.202 | 8.202 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock"}) |
| message_repository_idempotency_lock_p99_recent_ms | True | 1 | 27 | 0 | 6.065 | 6.065 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock_recent"}) |
| message_repository_find_existing_p99_ms | True | 1 | 27 | 0 | 15.951 | 15.951 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing"}) |
| message_repository_find_existing_p99_recent_ms | True | 1 | 27 | 0 | 6.709 | 6.709 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing_recent"}) |
| message_repository_ensure_seq_p99_ms | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq"}) |
| message_repository_ensure_seq_p99_recent_ms | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq_recent"}) |
| message_repository_allocate_seq_p99_ms | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq"}) |
| message_repository_allocate_seq_p99_recent_ms | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq_recent"}) |
| message_repository_insert_message_p99_ms | True | 1 | 27 | 0 | 21.881 | 21.881 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message"}) |
| message_repository_insert_message_p99_recent_ms | True | 1 | 27 | 0 | 21.463 | 21.463 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message_recent"}) |
| message_repository_insert_timeline_p99_ms | True | 1 | 27 | 0 | 17.04 | 17.04 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline"}) |
| message_repository_insert_timeline_p99_recent_ms | True | 1 | 27 | 0 | 13.199 | 13.199 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline_recent"}) |
| message_repository_insert_outbox_p99_ms | True | 1 | 27 | 0 | 25.341 | 25.341 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox"}) |
| message_repository_insert_outbox_p99_recent_ms | True | 1 | 27 | 0 | 19.773 | 19.773 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox_recent"}) |
| message_repository_commit_p99_ms | True | 1 | 27 | 0 | 14.375 | 14.375 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit"}) |
| message_repository_commit_p99_recent_ms | True | 1 | 27 | 0 | 12.533 | 12.533 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit_recent"}) |
| message_outbox_fetched_per_call_avg | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"}) |
| message_kafka_records_per_call_avg | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"}) |
| conversation_grpc_rps | True | 1 | 16 | 0 | 19.99 | 0 | sum(rate(nexusim_conversation_grpc_method_requests_total[5m])) |
| policy_grpc_rps | True | 1 | 14 | 0 | 0 | 0 | sum(rate(nexusim_policy_grpc_method_requests_total[5m])) |
| policy_grpc_check_avg_ms | True | 1 | 14 | 261 | 261 | 261 | max(nexusim_policy_grpc_latency_avg_milliseconds{method="/nexusim.policy.v1.PolicyService/CheckMessageAction"}) |
| policy_grpc_check_max_ms | True | 1 | 14 | 439 | 439 | 439 | max(nexusim_policy_grpc_latency_max_milliseconds{method="/nexusim.policy.v1.PolicyService/CheckMessageAction"}) |
| policy_decision_send_avg_ms | True | 1 | 14 | 261 | 261 | 261 | max(nexusim_policy_decision_latency_avg_milliseconds{action="SEND"}) |
| policy_decision_send_max_ms | True | 1 | 14 | 439 | 439 | 439 | max(nexusim_policy_decision_latency_max_milliseconds{action="SEND"}) |
| policy_decision_audit_p99_ms | True | 1 | 14 | 284 | 284 | 284 | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_outbox"}) |
| policy_decision_audit_max_ms | True | 1 | 14 | 293 | 293 | 293 | max(nexusim_policy_decision_stage_latency_max_milliseconds{action="SEND",stage="decision_audit_outbox"}) |
| policy_evaluator_revision_lookup_p99_ms | True | 1 | 14 | 9 | 9 | 9 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="policy_revision_lookup"}) |
| policy_evaluator_facts_read_p99_ms | True | 1 | 14 | 13 | 13 | 13 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="policy_facts_read"}) |
| policy_evaluator_decision_cache_get_p99_ms | True | 1 | 14 | 4 | 4 | 4 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="decision_cache_get"}) |
| policy_evaluator_decision_cache_set_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="decision_cache_set"}) |
| policy_evaluator_contact_block_p99_ms | True | 1 | 14 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="contact_block_lookup"}) |
| policy_evaluator_user_restriction_p99_ms | True | 1 | 14 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="user_restriction_lookup"}) |
| policy_evaluator_role_gate_p99_ms | True | 1 | 14 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="role_gate"}) |
| policy_evaluator_rebac_gate_p99_ms | True | 1 | 14 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="rebac_gate"}) |
| policy_evaluator_tenant_quota_p99_ms | True | 1 | 14 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="tenant_quota_lookup"}) |
| policy_evaluator_message_action_rule_p99_ms | True | 1 | 14 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="message_action_rule_lookup"}) |
| policy_evaluator_exact_rule_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="exact_rule_lookup"}) |
| policy_evaluator_tenant_rule_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="tenant_rule_lookup"}) |
| policy_evaluator_stage_errors_5m | True | 1 | 14 | 0 | 0 | 0 | sum(increase(nexusim_policy_evaluator_stage_errors_total[5m])) |
| policy_pg_pool_acquire_total | True | 1 | 27 | 20 | 76320 | 11041 | max(nexusim_policy_pg_pool_acquire_total) |
| policy_pg_pool_acquire_duration_ms_total | True | 1 | 27 | 17 | 150927 | 150927 | max(nexusim_policy_pg_pool_acquire_duration_milliseconds_total) |
| policy_pg_pool_empty_acquire_total | True | 1 | 27 | 1 | 3191 | 3191 | max(nexusim_policy_pg_pool_empty_acquire_total) |
| policy_pg_pool_canceled_acquire_total | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_policy_pg_pool_canceled_acquire_total) |
| policy_pg_pool_conns_acquired | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_policy_pg_pool_conns{state="acquired"}) |
| policy_pg_pool_conns_max | True | 1 | 27 | 32 | 128 | 32 | max(nexusim_policy_pg_pool_conns{state="max"}) |
| policy_audit_pg_pool_acquire_total | True | 1 | 25 | 0 | 5001 | 5001 | max(nexusim_policy_audit_pg_pool_acquire_total) |
| policy_audit_pg_pool_acquire_duration_ms_total | True | 1 | 25 | 0 | 1062277 | 1062277 | max(nexusim_policy_audit_pg_pool_acquire_duration_milliseconds_total) |
| policy_audit_pg_pool_empty_acquire_total | True | 1 | 25 | 0 | 4901 | 4901 | max(nexusim_policy_audit_pg_pool_empty_acquire_total) |
| policy_audit_pg_pool_canceled_acquire_total | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_canceled_acquire_total) |
| policy_audit_pg_pool_conns_acquired | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_conns{state="acquired"}) |
| policy_audit_pg_pool_conns_max | True | 1 | 25 | 16 | 16 | 16 | max(nexusim_policy_audit_pg_pool_conns{state="max"}) |
| delivery_outbox_pending | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending"}) |
| delivery_outbox_pending_ready | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending_ready"}) |
| delivery_outbox_dlq | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="dlq"}) |
| delivery_projection_unresolved | False | 0 | 0 |  |  |  | max(nexusim_delivery_projection_failures{state="unresolved_total"}) |
| delivery_timeline_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_timeline_worker_errors_total[5m])) |
| delivery_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[5m])) |
| delivery_grpc_rps | True | 1 | 13 | 0 | 0.046 | 0 | sum(rate(nexusim_delivery_grpc_method_requests_total[5m])) |
| delivery_grpc_errors_5m | True | 1 | 13 | 0 | 0 | 0 | sum(increase(nexusim_delivery_grpc_method_errors_total[5m])) |
| push_connected_sessions | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_push_gateway_sessions{state="connected"}) |
| push_slow_evicted_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m])) |
| push_writer_events_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total[5m])) |
| push_writer_outbound_dequeued_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m])) |
| push_writer_frame_write_attempt_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m])) |
| push_writer_frame_write_success_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m])) |
| push_writer_frame_write_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m])) |
| push_writer_delivery_notify_attempt_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m])) |
| push_writer_delivery_notify_success_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m])) |
| push_writer_delivery_notify_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m])) |
| push_writer_frame_write_duration_p95_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_frame_write_duration_p99_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p95_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_avg_ms_5m | True | 1 | 27 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_duration_max_ms | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"}) |
| push_writer_delivery_notify_queue_p95_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_avg_ms_5m | True | 1 | 27 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_queue_max_ms | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds{operation="delivery_notify"}) |
| push_consumer_worker_errors_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m])) |
| push_redis_route_events_5m | True | 1 | 27 | 0 | 9812.7 | 1813.684 | sum(increase(nexusim_push_gateway_redis_route_events_total[5m])) |
| push_redis_registry_remote_matched_sessions_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m])) |
| push_redis_registry_remote_publish_call_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m])) |
| push_redis_registry_remote_publish_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m])) |
| push_redis_registry_remote_no_subscriber_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m])) |
| push_redis_registry_conversation_route_cache_hit_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[5m])) |
| push_redis_registry_conversation_route_cache_miss_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[5m])) |
| push_redis_registry_conversation_route_cache_invalidated_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_registry_remote_enqueued_sessions_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_matched_sessions_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_publish_call_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[5m])) |
| push_redis_delivery_consumer_remote_publish_error_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[5m])) |
| push_redis_delivery_consumer_remote_no_subscriber_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_no_subscriber"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_5m | True | 1 | 27 | 0 | 4595.442 | 863.158 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_5m | True | 1 | 27 | 0 | 569.137 | 91.579 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[5m])) |
| push_redis_subscriber_messages_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m])) |
| push_redis_subscriber_enqueued_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m])) |
| push_redis_subscriber_evicted_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m])) |
| push_redis_subscriber_errors_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queued_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_full_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[5m])) |
| push_redis_subscriber_signal_fanout_worker_errors_5m | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_depth | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_avg_ms_5m | True | 1 | 27 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum{operation="conversation_signal"}[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count{operation="conversation_signal"}[5m])) |
| push_redis_subscriber_signal_fanout_duration_max_ms | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds{operation="conversation_signal"}) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_5m | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_avg_ms_5m | True | 1 | 27 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count[5m])) |
| push_redis_subscriber_signal_fanout_queue_wait_max_ms | True | 1 | 27 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds) |
| push_writer_frame_write_success_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[809s])) |
| push_writer_frame_write_error_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[809s])) |
| push_writer_delivery_notify_success_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[809s])) |
| push_writer_delivery_notify_error_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[809s])) |
| push_writer_delivery_notify_duration_p95_ms_window | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[809s])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_window | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[809s])) by (le)) |
| push_writer_delivery_notify_queue_p95_ms_window | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[809s])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_window | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[809s])) by (le)) |
| push_redis_subscriber_messages_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[809s])) |
| push_redis_subscriber_enqueued_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[809s])) |
| push_redis_subscriber_evicted_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[809s])) |
| push_redis_subscriber_errors_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[809s])) |
| push_redis_registry_conversation_route_cache_hit_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[809s])) |
| push_redis_registry_conversation_route_cache_miss_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[809s])) |
| push_redis_registry_conversation_route_cache_invalidated_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[809s])) |
| push_redis_delivery_consumer_remote_matched_sessions_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[809s])) |
| push_redis_delivery_consumer_remote_publish_call_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[809s])) |
| push_redis_delivery_consumer_remote_publish_error_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[809s])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_window | True | 1 | 27 | 0 | 4501.498 | 4501.498 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[809s])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_window | True | 1 | 27 | 0 | 557.502 | 557.502 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[809s])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[809s])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[809s])) |
| push_redis_subscriber_signal_fanout_queued_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[809s])) |
| push_redis_subscriber_signal_fanout_queue_full_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[809s])) |
| push_redis_subscriber_signal_fanout_worker_errors_window | True | 1 | 27 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[809s])) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_window | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[809s])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_window | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[809s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_window | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[809s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_window | True | 1 | 27 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[809s])) by (le)) |
| message_pg_pool_conns | True | 1 | 27 | 72 | 72 | 72 | max(nexusim_message_pg_pool_conns) |
| conversation_pg_pool_conns | True | 1 | 27 | 32 | 32 | 32 | max(nexusim_conversation_pg_pool_conns) |
| delivery_pg_pool_conns | True | 1 | 27 | 16 | 16 | 16 | max(nexusim_delivery_pg_pool_conns) |

## Interpretation

- Use this report with the matching hotgroup summary and analysis report.
- Metrics ending in _5m show the moving five-minute pressure window; metrics ending in _window approximate the whole captured run window.
- Metrics with no data mean the exporter or scrape target did not expose that series in this window.
- Do not use this single window as production capacity evidence.
