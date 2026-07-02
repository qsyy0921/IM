# Hot Group Metrics Window

Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.

- run_name: hotgroup-runtimeprofile-audit64-pg192-loadpg-6000x5000-768c-fed098de-20260702-173719
- commit: fed098d
- git_dirty: True
- result_dir: H:\NexusIM\loadtest-results\hotgroup-runtimeprofile-audit64-pg192-loadpg-6000x5000-768c-fed098de-20260702-173719
- prometheus: http://172.31.50.2:19091
- window_start_utc: 2026-07-02T09:32:20.2821178Z
- window_end_utc: 2026-07-02T09:44:11.8943343Z
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
| send_p95_ms | 485.805 |
| send_p99_ms | 582.221 |
| pull_p95_ms | 0 |
| conversation_signal_count | 0 |

## Prometheus Query Summary

| metric | has data | series | samples | min | max | last | query |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| core_targets_up | True | 1 | 24 | 8 | 8 | 8 | sum(up{job=~"nexusim-message-service\|nexusim-conversation-service\|nexusim-policy-service\|nexusim-delivery-service\|nexusim-push-gateway"}) |
| message_send_p95_ms | True | 1 | 24 | 618.305 | 699.306 | 618.305 | max(nexusim_message_latency_p95_milliseconds{operation="send_message"}) |
| message_send_p99_ms | True | 1 | 24 | 726.947 | 742.327 | 726.947 | max(nexusim_message_latency_p99_milliseconds{operation="send_message"}) |
| message_send_p95_recent_ms | True | 1 | 24 | 361.923 | 400.252 | 400.252 | max(nexusim_message_latency_p95_milliseconds{operation="send_message_recent"}) |
| message_send_p99_recent_ms | True | 1 | 24 | 427.557 | 445.09 | 445.09 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_recent"}) |
| message_command_build_p99_recent_ms | True | 1 | 24 | 0.112 | 0.125 | 0.112 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_command_build_recent"}) |
| message_admission_p99_recent_ms | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_admission_recent"}) |
| message_dependency_read_p99_recent_ms | True | 1 | 24 | 309.973 | 343.293 | 343.293 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_dependency_read_recent"}) |
| message_conversation_context_p99_recent_ms | True | 1 | 24 | 41.901 | 47.262 | 41.901 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_conversation_context_recent"}) |
| message_policy_check_p99_recent_ms | True | 1 | 24 | 288.236 | 318.97 | 318.97 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_policy_check_recent"}) |
| message_seq_floor_p99_recent_ms | True | 1 | 24 | 0.028 | 0.03 | 0.03 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_seq_floor_recent"}) |
| message_sequencer_allocate_p99_recent_ms | True | 1 | 24 | 39.459 | 51.322 | 51.322 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_sequencer_allocate_recent"}) |
| message_repository_append_call_p99_recent_ms | True | 1 | 24 | 145.201 | 145.447 | 145.201 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_repository_append_call_recent"}) |
| message_seq_alloc_p99_ms | True | 1 | 24 | 0.029 | 0.029 | 0.029 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"}) |
| message_seq_alloc_p99_recent_ms | True | 1 | 24 | 0.029 | 0.029 | 0.029 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc_recent"}) |
| message_repository_append_p99_ms | True | 1 | 24 | 262.656 | 298.791 | 262.656 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append"}) |
| message_repository_append_p99_recent_ms | True | 1 | 24 | 145.195 | 145.445 | 145.195 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append_recent"}) |
| message_repository_pool_acquire_p99_ms | True | 1 | 24 | 116.249 | 171.178 | 116.249 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire"}) |
| message_repository_pool_acquire_p99_recent_ms | True | 1 | 24 | 5.067 | 22.178 | 22.178 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire_recent"}) |
| message_repository_tx_begin_p99_ms | True | 1 | 24 | 9.182 | 10.043 | 9.182 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin"}) |
| message_repository_tx_begin_p99_recent_ms | True | 1 | 24 | 8.487 | 9.018 | 9.018 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin_recent"}) |
| message_repository_idempotency_lock_p99_ms | True | 1 | 24 | 13.092 | 19.134 | 13.092 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock"}) |
| message_repository_idempotency_lock_p99_recent_ms | True | 1 | 24 | 7.913 | 8.72 | 8.72 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock_recent"}) |
| message_repository_find_existing_p99_ms | True | 1 | 24 | 26.803 | 38.418 | 26.803 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing"}) |
| message_repository_find_existing_p99_recent_ms | True | 1 | 24 | 9.627 | 9.814 | 9.627 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing_recent"}) |
| message_repository_ensure_seq_p99_ms | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq"}) |
| message_repository_ensure_seq_p99_recent_ms | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq_recent"}) |
| message_repository_allocate_seq_p99_ms | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq"}) |
| message_repository_allocate_seq_p99_recent_ms | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq_recent"}) |
| message_repository_insert_message_p99_ms | True | 1 | 24 | 48.123 | 57.139 | 57.139 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message"}) |
| message_repository_insert_message_p99_recent_ms | True | 1 | 24 | 38.685 | 68.567 | 68.567 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message_recent"}) |
| message_repository_insert_timeline_p99_ms | True | 1 | 24 | 55.781 | 58.906 | 55.781 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline"}) |
| message_repository_insert_timeline_p99_recent_ms | True | 1 | 24 | 56.094 | 75.959 | 56.094 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline_recent"}) |
| message_repository_insert_outbox_p99_ms | True | 1 | 24 | 68.534 | 75.958 | 75.958 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox"}) |
| message_repository_insert_outbox_p99_recent_ms | True | 1 | 24 | 62.784 | 83.22 | 83.22 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox_recent"}) |
| message_repository_commit_p99_ms | True | 1 | 24 | 20.315 | 22.158 | 20.315 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit"}) |
| message_repository_commit_p99_recent_ms | True | 1 | 24 | 16.348 | 17.905 | 17.905 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit_recent"}) |
| message_outbox_fetched_per_call_avg | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"}) |
| message_kafka_records_per_call_avg | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"}) |
| conversation_grpc_rps | True | 1 | 24 | 0 | 39.941 | 0 | sum(rate(nexusim_conversation_grpc_method_requests_total[5m])) |
| policy_grpc_rps | True | 1 | 24 | 0 | 0 | 0 | sum(rate(nexusim_policy_grpc_method_requests_total[5m])) |
| policy_grpc_check_avg_ms | True | 1 | 21 | 234 | 237 | 237 | max(nexusim_policy_grpc_latency_avg_milliseconds{method="/nexusim.policy.v1.PolicyService/CheckMessageAction"}) |
| policy_grpc_check_max_ms | True | 1 | 21 | 409 | 485 | 409 | max(nexusim_policy_grpc_latency_max_milliseconds{method="/nexusim.policy.v1.PolicyService/CheckMessageAction"}) |
| policy_decision_send_avg_ms | True | 1 | 21 | 233 | 237 | 237 | max(nexusim_policy_decision_latency_avg_milliseconds{action="SEND"}) |
| policy_decision_send_max_ms | True | 1 | 21 | 409 | 485 | 409 | max(nexusim_policy_decision_latency_max_milliseconds{action="SEND"}) |
| policy_decision_audit_p99_ms | True | 1 | 21 | 265 | 289 | 289 | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_outbox"}) |
| policy_decision_audit_max_ms | True | 1 | 21 | 286 | 329 | 329 | max(nexusim_policy_decision_stage_latency_max_milliseconds{action="SEND",stage="decision_audit_outbox"}) |
| policy_evaluator_revision_lookup_p99_ms | True | 1 | 21 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="policy_revision_lookup"}) |
| policy_evaluator_facts_read_p99_ms | True | 1 | 21 | 17 | 34 | 34 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="policy_facts_read"}) |
| policy_evaluator_decision_cache_get_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="decision_cache_get"}) |
| policy_evaluator_decision_cache_set_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="decision_cache_set"}) |
| policy_evaluator_contact_block_p99_ms | True | 1 | 21 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="contact_block_lookup"}) |
| policy_evaluator_user_restriction_p99_ms | True | 1 | 21 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="user_restriction_lookup"}) |
| policy_evaluator_role_gate_p99_ms | True | 1 | 21 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="role_gate"}) |
| policy_evaluator_rebac_gate_p99_ms | True | 1 | 21 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="rebac_gate"}) |
| policy_evaluator_tenant_quota_p99_ms | True | 1 | 21 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="tenant_quota_lookup"}) |
| policy_evaluator_message_action_rule_p99_ms | True | 1 | 21 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="message_action_rule_lookup"}) |
| policy_evaluator_exact_rule_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="exact_rule_lookup"}) |
| policy_evaluator_tenant_rule_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="tenant_rule_lookup"}) |
| policy_evaluator_stage_errors_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_policy_evaluator_stage_errors_total[5m])) |
| policy_pg_pool_acquire_total | True | 1 | 24 | 142 | 5560 | 5321 | max(nexusim_policy_pg_pool_acquire_total) |
| policy_pg_pool_acquire_duration_ms_total | True | 1 | 24 | 19 | 155394 | 155394 | max(nexusim_policy_pg_pool_acquire_duration_milliseconds_total) |
| policy_pg_pool_empty_acquire_total | True | 1 | 24 | 1 | 1469 | 1469 | max(nexusim_policy_pg_pool_empty_acquire_total) |
| policy_pg_pool_canceled_acquire_total | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_policy_pg_pool_canceled_acquire_total) |
| policy_pg_pool_conns_acquired | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_policy_pg_pool_conns{state="acquired"}) |
| policy_pg_pool_conns_max | True | 1 | 24 | 32 | 32 | 32 | max(nexusim_policy_pg_pool_conns{state="max"}) |
| policy_audit_pg_pool_acquire_total | True | 1 | 24 | 0 | 5001 | 5001 | max(nexusim_policy_audit_pg_pool_acquire_total) |
| policy_audit_pg_pool_acquire_duration_ms_total | True | 1 | 24 | 0 | 928158 | 849904 | max(nexusim_policy_audit_pg_pool_acquire_duration_milliseconds_total) |
| policy_audit_pg_pool_empty_acquire_total | True | 1 | 24 | 0 | 4898 | 4898 | max(nexusim_policy_audit_pg_pool_empty_acquire_total) |
| policy_audit_pg_pool_canceled_acquire_total | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_canceled_acquire_total) |
| policy_audit_pg_pool_conns_acquired | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_conns{state="acquired"}) |
| policy_audit_pg_pool_conns_max | True | 1 | 24 | 32 | 64 | 64 | max(nexusim_policy_audit_pg_pool_conns{state="max"}) |
| delivery_outbox_pending | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending"}) |
| delivery_outbox_pending_ready | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending_ready"}) |
| delivery_outbox_dlq | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="dlq"}) |
| delivery_projection_unresolved | False | 0 | 0 |  |  |  | max(nexusim_delivery_projection_failures{state="unresolved_total"}) |
| delivery_timeline_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_timeline_worker_errors_total[5m])) |
| delivery_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[5m])) |
| delivery_grpc_rps | True | 1 | 24 | 0 | 0.063 | 0 | sum(rate(nexusim_delivery_grpc_method_requests_total[5m])) |
| delivery_grpc_errors_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_delivery_grpc_method_errors_total[5m])) |
| push_connected_sessions | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_push_gateway_sessions{state="connected"}) |
| push_slow_evicted_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m])) |
| push_writer_events_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total[5m])) |
| push_writer_outbound_dequeued_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m])) |
| push_writer_frame_write_attempt_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m])) |
| push_writer_frame_write_success_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m])) |
| push_writer_frame_write_error_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m])) |
| push_writer_delivery_notify_attempt_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m])) |
| push_writer_delivery_notify_success_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m])) |
| push_writer_delivery_notify_error_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m])) |
| push_writer_frame_write_duration_p95_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_frame_write_duration_p99_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p95_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_avg_ms_5m | True | 1 | 24 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_duration_max_ms | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"}) |
| push_writer_delivery_notify_queue_p95_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_avg_ms_5m | True | 1 | 24 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_queue_max_ms | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds{operation="delivery_notify"}) |
| push_consumer_worker_errors_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m])) |
| push_redis_route_events_5m | True | 1 | 24 | 1637.895 | 10000 | 5734.85 | sum(increase(nexusim_push_gateway_redis_route_events_total[5m])) |
| push_redis_registry_remote_matched_sessions_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m])) |
| push_redis_registry_remote_publish_call_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m])) |
| push_redis_registry_remote_publish_error_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m])) |
| push_redis_registry_remote_no_subscriber_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m])) |
| push_redis_registry_conversation_route_cache_hit_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[5m])) |
| push_redis_registry_conversation_route_cache_miss_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[5m])) |
| push_redis_registry_conversation_route_cache_invalidated_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_registry_remote_enqueued_sessions_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_matched_sessions_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_publish_call_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[5m])) |
| push_redis_delivery_consumer_remote_publish_error_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[5m])) |
| push_redis_delivery_consumer_remote_no_subscriber_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_no_subscriber"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_5m | True | 1 | 24 | 780 | 4720 | 2709.396 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_5m | True | 1 | 24 | 82.105 | 543.158 | 309.007 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[5m])) |
| push_redis_subscriber_messages_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m])) |
| push_redis_subscriber_enqueued_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m])) |
| push_redis_subscriber_evicted_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m])) |
| push_redis_subscriber_errors_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queued_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_full_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[5m])) |
| push_redis_subscriber_signal_fanout_worker_errors_5m | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_depth | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_avg_ms_5m | True | 1 | 24 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum{operation="conversation_signal"}[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count{operation="conversation_signal"}[5m])) |
| push_redis_subscriber_signal_fanout_duration_max_ms | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds{operation="conversation_signal"}) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_5m | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_avg_ms_5m | True | 1 | 24 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count[5m])) |
| push_redis_subscriber_signal_fanout_queue_wait_max_ms | True | 1 | 24 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds) |
| push_writer_frame_write_success_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[711s])) |
| push_writer_frame_write_error_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[711s])) |
| push_writer_delivery_notify_success_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[711s])) |
| push_writer_delivery_notify_error_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[711s])) |
| push_writer_delivery_notify_duration_p95_ms_window | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[711s])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_window | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[711s])) by (le)) |
| push_writer_delivery_notify_queue_p95_ms_window | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[711s])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_window | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[711s])) by (le)) |
| push_redis_subscriber_messages_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[711s])) |
| push_redis_subscriber_enqueued_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[711s])) |
| push_redis_subscriber_evicted_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[711s])) |
| push_redis_subscriber_errors_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[711s])) |
| push_redis_registry_conversation_route_cache_hit_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[711s])) |
| push_redis_registry_conversation_route_cache_miss_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[711s])) |
| push_redis_registry_conversation_route_cache_invalidated_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[711s])) |
| push_redis_delivery_consumer_remote_matched_sessions_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[711s])) |
| push_redis_delivery_consumer_remote_publish_call_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[711s])) |
| push_redis_delivery_consumer_remote_publish_error_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[711s])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_window | True | 1 | 24 | 973.761 | 8058.576 | 7110.38 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[711s])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_window | True | 1 | 24 | 103.043 | 913.229 | 813.15 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[711s])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[711s])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[711s])) |
| push_redis_subscriber_signal_fanout_queued_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[711s])) |
| push_redis_subscriber_signal_fanout_queue_full_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[711s])) |
| push_redis_subscriber_signal_fanout_worker_errors_window | True | 1 | 24 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[711s])) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_window | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[711s])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_window | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[711s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_window | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[711s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_window | True | 1 | 24 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[711s])) by (le)) |
| message_pg_pool_conns | True | 1 | 24 | 192 | 192 | 192 | max(nexusim_message_pg_pool_conns) |
| conversation_pg_pool_conns | True | 1 | 24 | 32 | 32 | 32 | max(nexusim_conversation_pg_pool_conns) |
| delivery_pg_pool_conns | True | 1 | 24 | 16 | 16 | 16 | max(nexusim_delivery_pg_pool_conns) |

## Interpretation

- Use this report with the matching hotgroup summary and analysis report.
- Metrics ending in _5m show the moving five-minute pressure window; metrics ending in _window approximate the whole captured run window.
- Metrics with no data mean the exporter or scrape target did not expose that series in this window.
- Do not use this single window as production capacity evidence.
