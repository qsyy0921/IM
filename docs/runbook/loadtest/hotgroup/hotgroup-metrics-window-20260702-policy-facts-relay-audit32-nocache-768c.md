# Hot Group Metrics Window

Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.

- run_name: hotgroup-policy-facts-relay-audit32-nocache-6000x5000-768c-e9c2df2e-20260702-1545
- commit: e9c2df2
- git_dirty: False
- result_dir: H:\NexusIM\loadtest-results\hotgroup-policy-facts-relay-audit32-nocache-6000x5000-768c-e9c2df2e-20260702-1545
- prometheus: http://172.31.50.2:19091
- window_start_utc: 2026-07-02T07:43:29.4551667Z
- window_end_utc: 2026-07-02T07:48:56.3348815Z
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
| send_p95_ms | 601.584 |
| send_p99_ms | 679.805 |
| pull_p95_ms | 1572.215 |
| conversation_signal_count | 0 |

## Prometheus Query Summary

| metric | has data | series | samples | min | max | last | query |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| core_targets_up | True | 1 | 11 | 8 | 8 | 8 | sum(up{job=~"nexusim-message-service\|nexusim-conversation-service\|nexusim-policy-service\|nexusim-delivery-service\|nexusim-push-gateway"}) |
| message_send_p95_ms | True | 1 | 11 | 487.399 | 516.227 | 516.227 | max(nexusim_message_latency_p95_milliseconds{operation="send_message"}) |
| message_send_p99_ms | True | 1 | 11 | 616.699 | 635.536 | 635.536 | max(nexusim_message_latency_p99_milliseconds{operation="send_message"}) |
| message_send_p95_recent_ms | True | 1 | 11 | 343.177 | 353.419 | 353.419 | max(nexusim_message_latency_p95_milliseconds{operation="send_message_recent"}) |
| message_send_p99_recent_ms | True | 1 | 11 | 372.885 | 373.225 | 372.885 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_recent"}) |
| message_command_build_p99_recent_ms | True | 1 | 11 | 0.136 | 0.142 | 0.142 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_command_build_recent"}) |
| message_admission_p99_recent_ms | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_admission_recent"}) |
| message_dependency_read_p99_recent_ms | True | 1 | 11 | 147.71 | 277.755 | 147.71 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_dependency_read_recent"}) |
| message_conversation_context_p99_recent_ms | True | 1 | 11 | 42.735 | 75.519 | 42.735 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_conversation_context_recent"}) |
| message_policy_check_p99_recent_ms | True | 1 | 11 | 131.25 | 213.297 | 131.25 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_policy_check_recent"}) |
| message_seq_floor_p99_recent_ms | True | 1 | 11 | 0.032 | 0.037 | 0.032 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_seq_floor_recent"}) |
| message_sequencer_allocate_p99_recent_ms | True | 1 | 11 | 33.157 | 100.068 | 33.157 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_sequencer_allocate_recent"}) |
| message_repository_append_call_p99_recent_ms | True | 1 | 11 | 249.637 | 290.167 | 290.167 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_repository_append_call_recent"}) |
| message_seq_alloc_p99_ms | True | 1 | 11 | 0.028 | 0.029 | 0.029 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"}) |
| message_seq_alloc_p99_recent_ms | True | 1 | 11 | 0.028 | 0.03 | 0.03 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc_recent"}) |
| message_repository_append_p99_ms | True | 1 | 11 | 239.687 | 278.312 | 278.312 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append"}) |
| message_repository_append_p99_recent_ms | True | 1 | 11 | 249.636 | 290.164 | 290.164 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append_recent"}) |
| message_repository_pool_acquire_p99_ms | True | 1 | 11 | 210.641 | 243.033 | 243.033 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire"}) |
| message_repository_pool_acquire_p99_recent_ms | True | 1 | 11 | 217.238 | 252.653 | 252.653 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire_recent"}) |
| message_repository_tx_begin_p99_ms | True | 1 | 11 | 14.664 | 16.043 | 14.664 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin"}) |
| message_repository_tx_begin_p99_recent_ms | True | 1 | 11 | 10.73 | 20.373 | 10.73 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin_recent"}) |
| message_repository_idempotency_lock_p99_ms | True | 1 | 11 | 8.972 | 9.316 | 9.316 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock"}) |
| message_repository_idempotency_lock_p99_recent_ms | True | 1 | 11 | 7.693 | 8.853 | 7.693 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock_recent"}) |
| message_repository_find_existing_p99_ms | True | 1 | 11 | 11.567 | 13.119 | 13.119 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing"}) |
| message_repository_find_existing_p99_recent_ms | True | 1 | 11 | 9.896 | 10.294 | 10.294 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing_recent"}) |
| message_repository_ensure_seq_p99_ms | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq"}) |
| message_repository_ensure_seq_p99_recent_ms | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq_recent"}) |
| message_repository_allocate_seq_p99_ms | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq"}) |
| message_repository_allocate_seq_p99_recent_ms | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq_recent"}) |
| message_repository_insert_message_p99_ms | True | 1 | 11 | 24.225 | 26.196 | 26.196 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message"}) |
| message_repository_insert_message_p99_recent_ms | True | 1 | 11 | 27.963 | 28.207 | 28.207 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message_recent"}) |
| message_repository_insert_timeline_p99_ms | True | 1 | 11 | 18.992 | 20.919 | 20.919 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline"}) |
| message_repository_insert_timeline_p99_recent_ms | True | 1 | 11 | 20.557 | 25.454 | 25.454 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline_recent"}) |
| message_repository_insert_outbox_p99_ms | True | 1 | 11 | 30.531 | 32.716 | 32.716 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox"}) |
| message_repository_insert_outbox_p99_recent_ms | True | 1 | 11 | 33.899 | 36.211 | 36.211 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox_recent"}) |
| message_repository_commit_p99_ms | True | 1 | 11 | 17.378 | 17.591 | 17.378 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit"}) |
| message_repository_commit_p99_recent_ms | True | 1 | 11 | 16.87 | 18.296 | 16.87 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit_recent"}) |
| message_outbox_fetched_per_call_avg | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"}) |
| message_kafka_records_per_call_avg | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"}) |
| conversation_grpc_rps | True | 1 | 11 | 0 | 38.596 | 38.596 | sum(rate(nexusim_conversation_grpc_method_requests_total[5m])) |
| policy_grpc_rps | True | 1 | 6 | 13.506 | 16.921 | 13.506 | sum(rate(nexusim_policy_grpc_method_requests_total[5m])) |
| policy_grpc_check_avg_ms | True | 1 | 7 | 77 | 187 | 77 | max(nexusim_policy_grpc_latency_avg_milliseconds{method="/nexusim.policy.v1.PolicyService/CheckMessageAction"}) |
| policy_grpc_check_max_ms | True | 1 | 7 | 313 | 313 | 313 | max(nexusim_policy_grpc_latency_max_milliseconds{method="/nexusim.policy.v1.PolicyService/CheckMessageAction"}) |
| policy_decision_send_avg_ms | True | 1 | 7 | 76 | 187 | 76 | max(nexusim_policy_decision_latency_avg_milliseconds{action="SEND"}) |
| policy_decision_send_max_ms | True | 1 | 7 | 313 | 313 | 313 | max(nexusim_policy_decision_latency_max_milliseconds{action="SEND"}) |
| policy_decision_audit_p99_ms | True | 1 | 7 | 113 | 125 | 113 | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_outbox"}) |
| policy_decision_audit_max_ms | True | 1 | 7 | 129 | 129 | 129 | max(nexusim_policy_decision_stage_latency_max_milliseconds{action="SEND",stage="decision_audit_outbox"}) |
| policy_evaluator_revision_lookup_p99_ms | True | 1 | 7 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="policy_revision_lookup"}) |
| policy_evaluator_facts_read_p99_ms | True | 1 | 7 | 16 | 228 | 16 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="policy_facts_read"}) |
| policy_evaluator_decision_cache_get_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="decision_cache_get"}) |
| policy_evaluator_decision_cache_set_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="decision_cache_set"}) |
| policy_evaluator_contact_block_p99_ms | True | 1 | 7 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="contact_block_lookup"}) |
| policy_evaluator_user_restriction_p99_ms | True | 1 | 7 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="user_restriction_lookup"}) |
| policy_evaluator_role_gate_p99_ms | True | 1 | 7 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="role_gate"}) |
| policy_evaluator_rebac_gate_p99_ms | True | 1 | 7 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="rebac_gate"}) |
| policy_evaluator_tenant_quota_p99_ms | True | 1 | 7 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="tenant_quota_lookup"}) |
| policy_evaluator_message_action_rule_p99_ms | True | 1 | 7 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="message_action_rule_lookup"}) |
| policy_evaluator_exact_rule_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="exact_rule_lookup"}) |
| policy_evaluator_tenant_rule_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="tenant_rule_lookup"}) |
| policy_evaluator_stage_errors_5m | True | 1 | 6 | 0 | 0 | 0 | sum(increase(nexusim_policy_evaluator_stage_errors_total[5m])) |
| policy_pg_pool_acquire_total | True | 1 | 11 | 875 | 5621 | 5621 | max(nexusim_policy_pg_pool_acquire_total) |
| policy_pg_pool_acquire_duration_ms_total | True | 1 | 11 | 32 | 127836 | 127836 | max(nexusim_policy_pg_pool_acquire_duration_milliseconds_total) |
| policy_pg_pool_empty_acquire_total | True | 1 | 11 | 2 | 1132 | 1132 | max(nexusim_policy_pg_pool_empty_acquire_total) |
| policy_pg_pool_canceled_acquire_total | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_policy_pg_pool_canceled_acquire_total) |
| policy_pg_pool_conns_acquired | True | 1 | 11 | 0 | 8 | 0 | max(nexusim_policy_pg_pool_conns{state="acquired"}) |
| policy_pg_pool_conns_max | True | 1 | 11 | 32 | 32 | 32 | max(nexusim_policy_pg_pool_conns{state="max"}) |
| policy_audit_pg_pool_acquire_total | True | 1 | 11 | 1 | 5001 | 5001 | max(nexusim_policy_audit_pg_pool_acquire_total) |
| policy_audit_pg_pool_acquire_duration_ms_total | True | 1 | 11 | 12 | 146695 | 146695 | max(nexusim_policy_audit_pg_pool_acquire_duration_milliseconds_total) |
| policy_audit_pg_pool_empty_acquire_total | True | 1 | 11 | 1 | 4255 | 4255 | max(nexusim_policy_audit_pg_pool_empty_acquire_total) |
| policy_audit_pg_pool_canceled_acquire_total | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_canceled_acquire_total) |
| policy_audit_pg_pool_conns_acquired | True | 1 | 11 | 0 | 24 | 0 | max(nexusim_policy_audit_pg_pool_conns{state="acquired"}) |
| policy_audit_pg_pool_conns_max | True | 1 | 11 | 32 | 32 | 32 | max(nexusim_policy_audit_pg_pool_conns{state="max"}) |
| delivery_outbox_pending | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending"}) |
| delivery_outbox_pending_ready | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending_ready"}) |
| delivery_outbox_dlq | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="dlq"}) |
| delivery_projection_unresolved | False | 0 | 0 |  |  |  | max(nexusim_delivery_projection_failures{state="unresolved_total"}) |
| delivery_timeline_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_timeline_worker_errors_total[5m])) |
| delivery_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[5m])) |
| delivery_grpc_rps | True | 1 | 11 | 0 | 0.063 | 0.063 | sum(rate(nexusim_delivery_grpc_method_requests_total[5m])) |
| delivery_grpc_errors_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_delivery_grpc_method_errors_total[5m])) |
| push_connected_sessions | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_push_gateway_sessions{state="connected"}) |
| push_slow_evicted_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m])) |
| push_writer_events_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total[5m])) |
| push_writer_outbound_dequeued_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m])) |
| push_writer_frame_write_attempt_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m])) |
| push_writer_frame_write_success_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m])) |
| push_writer_frame_write_error_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m])) |
| push_writer_delivery_notify_attempt_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m])) |
| push_writer_delivery_notify_success_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m])) |
| push_writer_delivery_notify_error_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m])) |
| push_writer_frame_write_duration_p95_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_frame_write_duration_p99_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p95_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_avg_ms_5m | True | 1 | 11 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_duration_max_ms | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"}) |
| push_writer_delivery_notify_queue_p95_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_avg_ms_5m | True | 1 | 11 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_queue_max_ms | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds{operation="delivery_notify"}) |
| push_consumer_worker_errors_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m])) |
| push_redis_route_events_5m | True | 1 | 11 | 0 | 10000 | 10000 | sum(increase(nexusim_push_gateway_redis_route_events_total[5m])) |
| push_redis_registry_remote_matched_sessions_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m])) |
| push_redis_registry_remote_publish_call_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m])) |
| push_redis_registry_remote_publish_error_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m])) |
| push_redis_registry_remote_no_subscriber_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m])) |
| push_redis_registry_conversation_route_cache_hit_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[5m])) |
| push_redis_registry_conversation_route_cache_miss_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[5m])) |
| push_redis_registry_conversation_route_cache_invalidated_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_registry_remote_enqueued_sessions_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_matched_sessions_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_publish_call_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[5m])) |
| push_redis_delivery_consumer_remote_publish_error_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[5m])) |
| push_redis_delivery_consumer_remote_no_subscriber_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_no_subscriber"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_5m | True | 1 | 11 | 0 | 4671.579 | 4671.579 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_5m | True | 1 | 11 | 0 | 591.579 | 591.579 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[5m])) |
| push_redis_subscriber_messages_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m])) |
| push_redis_subscriber_enqueued_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m])) |
| push_redis_subscriber_evicted_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m])) |
| push_redis_subscriber_errors_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queued_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_full_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[5m])) |
| push_redis_subscriber_signal_fanout_worker_errors_5m | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_depth | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_avg_ms_5m | True | 1 | 11 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum{operation="conversation_signal"}[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count{operation="conversation_signal"}[5m])) |
| push_redis_subscriber_signal_fanout_duration_max_ms | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds{operation="conversation_signal"}) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_5m | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_avg_ms_5m | True | 1 | 11 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count[5m])) |
| push_redis_subscriber_signal_fanout_queue_wait_max_ms | True | 1 | 11 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds) |
| push_writer_frame_write_success_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[327s])) |
| push_writer_frame_write_error_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[327s])) |
| push_writer_delivery_notify_success_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[327s])) |
| push_writer_delivery_notify_error_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[327s])) |
| push_writer_delivery_notify_duration_p95_ms_window | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[327s])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_window | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[327s])) by (le)) |
| push_writer_delivery_notify_queue_p95_ms_window | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[327s])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_window | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[327s])) by (le)) |
| push_redis_subscriber_messages_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[327s])) |
| push_redis_subscriber_enqueued_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[327s])) |
| push_redis_subscriber_evicted_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[327s])) |
| push_redis_subscriber_errors_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[327s])) |
| push_redis_registry_conversation_route_cache_hit_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[327s])) |
| push_redis_registry_conversation_route_cache_miss_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[327s])) |
| push_redis_registry_conversation_route_cache_invalidated_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[327s])) |
| push_redis_delivery_consumer_remote_matched_sessions_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[327s])) |
| push_redis_delivery_consumer_remote_publish_call_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[327s])) |
| push_redis_delivery_consumer_remote_publish_error_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[327s])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_window | True | 1 | 11 | 0 | 4607.067 | 4607.067 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[327s])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_window | True | 1 | 11 | 0 | 583.41 | 583.41 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[327s])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[327s])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[327s])) |
| push_redis_subscriber_signal_fanout_queued_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[327s])) |
| push_redis_subscriber_signal_fanout_queue_full_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[327s])) |
| push_redis_subscriber_signal_fanout_worker_errors_window | True | 1 | 11 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[327s])) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_window | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[327s])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_window | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[327s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_window | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[327s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_window | True | 1 | 11 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[327s])) by (le)) |
| message_pg_pool_conns | True | 1 | 11 | 72 | 72 | 72 | max(nexusim_message_pg_pool_conns) |
| conversation_pg_pool_conns | True | 1 | 11 | 32 | 32 | 32 | max(nexusim_conversation_pg_pool_conns) |
| delivery_pg_pool_conns | True | 1 | 11 | 16 | 16 | 16 | max(nexusim_delivery_pg_pool_conns) |

## Interpretation

- Use this report with the matching hotgroup summary and analysis report.
- Metrics ending in _5m show the moving five-minute pressure window; metrics ending in _window approximate the whole captured run window.
- Metrics with no data mean the exporter or scrape target did not expose that series in this window.
- Do not use this single window as production capacity evidence.
