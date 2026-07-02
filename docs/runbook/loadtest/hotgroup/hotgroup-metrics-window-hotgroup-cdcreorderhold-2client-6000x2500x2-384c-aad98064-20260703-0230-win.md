# Hot Group Metrics Window

Scope: low-sensitive Prometheus query summary for one hotgroup run. This is not a production SLO.

- run_name: hotgroup-cdcreorderhold-2client-6000x2500x2-384c-aad98064-20260703-0230-win
- commit: aad9806
- git_dirty: False
- result_dir: H:\NexusIM\loadtest-results\hotgroup-cdcreorderhold-2client-6000x2500x2-384c-aad98064-20260703-0230-win
- prometheus: http://172.31.50.2:19091
- window_start_utc: 2026-07-02T18:25:06.8945815Z
- window_end_utc: 2026-07-02T18:37:33.7819766Z
- step_seconds: 30

## Run Parameters

| field | value |
| --- | ---: |
| group_size | 6000 |
| message_count | 2500 |
| message_rate | 8000 |
| sender_count | 384 |
| subscriber_count | 0 |
| fanout_mode | READ_FANOUT |
| send_p95_ms | 638.253 |
| send_p99_ms | 817.526 |
| pull_p95_ms | 60.544 |
| conversation_signal_count | 0 |

## Prometheus Query Summary

| metric | has data | series | samples | min | max | last | query |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| core_targets_up | True | 1 | 25 | 8 | 8 | 8 | sum(up{job=~"nexusim-message-service\|nexusim-conversation-service\|nexusim-policy-service\|nexusim-delivery-service\|nexusim-push-gateway"}) |
| message_send_p95_ms | True | 1 | 25 | 0 | 631.407 | 631.407 | max(nexusim_message_latency_p95_milliseconds{operation="send_message"}) |
| message_send_p99_ms | True | 1 | 25 | 0 | 816.636 | 816.636 | max(nexusim_message_latency_p99_milliseconds{operation="send_message"}) |
| message_send_p95_recent_ms | True | 1 | 25 | 0 | 654.679 | 654.679 | max(nexusim_message_latency_p95_milliseconds{operation="send_message_recent"}) |
| message_send_p99_recent_ms | True | 1 | 25 | 0 | 846.446 | 846.446 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_recent"}) |
| message_command_build_p99_recent_ms | True | 1 | 25 | 0 | 0.251 | 0.127 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_command_build_recent"}) |
| message_admission_p99_recent_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_admission_recent"}) |
| message_dependency_read_p99_recent_ms | True | 1 | 25 | 0 | 833.628 | 833.628 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_dependency_read_recent"}) |
| message_conversation_context_p99_recent_ms | True | 1 | 25 | 0 | 138.687 | 138.687 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_conversation_context_recent"}) |
| message_policy_check_p99_recent_ms | True | 1 | 25 | 0 | 769.226 | 769.226 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_policy_check_recent"}) |
| message_seq_floor_p99_recent_ms | True | 1 | 25 | 0 | 0.046 | 0.027 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_seq_floor_recent"}) |
| message_sequencer_allocate_p99_recent_ms | True | 1 | 25 | 0 | 152.202 | 33.507 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_sequencer_allocate_recent"}) |
| message_repository_append_call_p99_recent_ms | True | 1 | 25 | 0 | 173.032 | 29.727 | max(nexusim_message_latency_p99_milliseconds{operation="send_message_repository_append_call_recent"}) |
| message_seq_alloc_p99_ms | True | 1 | 25 | 0 | 0.034 | 0.028 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc"}) |
| message_seq_alloc_p99_recent_ms | True | 1 | 25 | 0 | 0.032 | 0.028 | max(nexusim_message_latency_p99_milliseconds{operation="conversation_seq_alloc_recent"}) |
| message_repository_append_p99_ms | True | 1 | 25 | 0 | 332.331 | 332.331 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append"}) |
| message_repository_append_p99_recent_ms | True | 1 | 25 | 0 | 173.026 | 29.726 | max(nexusim_message_latency_p99_milliseconds{operation="repository_append_recent"}) |
| message_repository_pool_acquire_p99_ms | True | 1 | 25 | 0 | 228.068 | 228.068 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire"}) |
| message_repository_pool_acquire_p99_recent_ms | True | 1 | 25 | 0 | 12.716 | 0.217 | max(nexusim_message_latency_p99_milliseconds{operation="repository_pool_acquire_recent"}) |
| message_repository_tx_begin_p99_ms | True | 1 | 25 | 0 | 20.165 | 12.094 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin"}) |
| message_repository_tx_begin_p99_recent_ms | True | 1 | 25 | 0 | 19.428 | 6.079 | max(nexusim_message_latency_p99_milliseconds{operation="repository_tx_begin_recent"}) |
| message_repository_idempotency_lock_p99_ms | True | 1 | 25 | 0 | 33.269 | 20.634 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock"}) |
| message_repository_idempotency_lock_p99_recent_ms | True | 1 | 25 | 0 | 17.538 | 6.537 | max(nexusim_message_latency_p99_milliseconds{operation="repository_idempotency_lock_recent"}) |
| message_repository_find_existing_p99_ms | True | 1 | 25 | 0 | 39.921 | 32.537 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing"}) |
| message_repository_find_existing_p99_recent_ms | True | 1 | 25 | 0 | 21.83 | 6.895 | max(nexusim_message_latency_p99_milliseconds{operation="repository_find_existing_recent"}) |
| message_repository_ensure_seq_p99_ms | True | 1 | 25 | 0 | 2.23 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq"}) |
| message_repository_ensure_seq_p99_recent_ms | True | 1 | 25 | 0 | 2.23 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_ensure_seq_recent"}) |
| message_repository_allocate_seq_p99_ms | True | 1 | 25 | 0 | 1.305 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq"}) |
| message_repository_allocate_seq_p99_recent_ms | True | 1 | 25 | 0 | 1.305 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_allocate_seq_recent"}) |
| message_repository_insert_message_p99_ms | True | 1 | 25 | 0 | 127.402 | 31.666 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message"}) |
| message_repository_insert_message_p99_recent_ms | True | 1 | 25 | 0 | 129.456 | 15.652 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_message_recent"}) |
| message_repository_insert_timeline_p99_ms | True | 1 | 25 | 0 | 48.238 | 41.718 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline"}) |
| message_repository_insert_timeline_p99_recent_ms | True | 1 | 25 | 0 | 48.336 | 9.333 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_timeline_recent"}) |
| message_repository_insert_outbox_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox"}) |
| message_repository_insert_outbox_p99_recent_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="repository_insert_outbox_recent"}) |
| message_repository_commit_p99_ms | True | 1 | 25 | 0 | 38.675 | 18.15 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit"}) |
| message_repository_commit_p99_recent_ms | True | 1 | 25 | 0 | 32.609 | 9.194 | max(nexusim_message_latency_p99_milliseconds{operation="repository_commit_recent"}) |
| message_outbox_fetched_per_call_avg | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="outbox_fetched_per_call_recent"}) |
| message_kafka_records_per_call_avg | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_value_avg{operation="kafka_publish_records_per_call_recent"}) |
| message_outbox_process_ready_active_recent_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="outbox_process_ready_active_recent"}) |
| message_outbox_process_ready_active_recent_max_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_max_milliseconds{operation="outbox_process_ready_active_recent"}) |
| message_outbox_fetch_ready_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="outbox_fetch_ready"}) |
| message_outbox_mark_published_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="outbox_mark_published"}) |
| message_outbox_commit_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="outbox_commit"}) |
| message_kafka_publish_call_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_p99_milliseconds{operation="kafka_publish_call"}) |
| message_kafka_publish_call_max_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_latency_max_milliseconds{operation="kafka_publish_call"}) |
| message_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_message_outbox_relay_errors_total[5m])) |
| message_outbox_relay_consecutive_errors | False | 0 | 0 |  |  |  | max(nexusim_message_outbox_relay_consecutive_errors) |
| conversation_grpc_rps | True | 1 | 25 | 0 | 17.806 | 0 | sum(rate(nexusim_conversation_grpc_method_requests_total[5m])) |
| policy_grpc_rps | True | 1 | 25 | 0 | 17.614 | 0 | sum(rate(nexusim_policy_grpc_method_requests_total[5m])) |
| policy_grpc_check_avg_ms | True | 1 | 25 | 100 | 128 | 128 | max(nexusim_policy_grpc_latency_avg_milliseconds{method="/nexusim.policy.v1.PolicyService/CheckMessageAction"}) |
| policy_grpc_check_max_ms | True | 1 | 25 | 860 | 1031 | 1031 | max(nexusim_policy_grpc_latency_max_milliseconds{method="/nexusim.policy.v1.PolicyService/CheckMessageAction"}) |
| policy_decision_send_avg_ms | True | 1 | 25 | 96 | 122 | 122 | max(nexusim_policy_decision_latency_avg_milliseconds{action="SEND"}) |
| policy_decision_send_max_ms | True | 1 | 25 | 860 | 969 | 969 | max(nexusim_policy_decision_latency_max_milliseconds{action="SEND"}) |
| policy_decision_audit_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_outbox"}) |
| policy_decision_audit_max_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_max_milliseconds{action="SEND",stage="decision_audit_outbox"}) |
| policy_decision_audit_kafka_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_kafka"}) |
| policy_decision_audit_kafka_max_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_max_milliseconds{action="SEND",stage="decision_audit_kafka"}) |
| policy_decision_audit_kafka_build_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_kafka_build"}) |
| policy_decision_audit_kafka_marshal_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_kafka_marshal"}) |
| policy_decision_audit_kafka_publish_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_kafka_publish"}) |
| policy_decision_audit_kafka_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_policy_decision_stage_errors_total{action="SEND",stage=~"decision_audit_kafka\|decision_audit_kafka_build\|decision_audit_kafka_marshal\|decision_audit_kafka_publish"}[5m])) |
| policy_decision_audit_kafka_async_p99_ms | True | 1 | 25 | 200 | 261 | 261 | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_kafka_async"}) |
| policy_decision_audit_kafka_async_max_ms | True | 1 | 25 | 324 | 426 | 426 | max(nexusim_policy_decision_stage_latency_max_milliseconds{action="SEND",stage="decision_audit_kafka_async"}) |
| policy_decision_audit_kafka_async_enqueue_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_kafka_async_enqueue"}) |
| policy_decision_audit_kafka_async_publish_p99_ms | True | 1 | 25 | 11 | 472 | 11 | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_kafka_async_publish"}) |
| policy_decision_audit_kafka_async_dlq_publish_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_kafka_async_dlq_publish"}) |
| policy_decision_audit_kafka_async_errors_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_policy_decision_stage_errors_total{action="SEND",stage=~"decision_audit_kafka_async\|decision_audit_kafka_async_build\|decision_audit_kafka_async_marshal\|decision_audit_kafka_async_enqueue\|decision_audit_kafka_async_publish\|decision_audit_kafka_async_retry\|decision_audit_kafka_async_dlq_publish"}[5m])) |
| policy_decision_audit_pool_acquire_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_pool_acquire"}) |
| policy_decision_audit_pool_acquire_max_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_max_milliseconds{action="SEND",stage="decision_audit_pool_acquire"}) |
| policy_decision_audit_insert_exec_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_p99_milliseconds{action="SEND",stage="decision_audit_insert_exec"}) |
| policy_decision_audit_insert_exec_max_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_decision_stage_latency_max_milliseconds{action="SEND",stage="decision_audit_insert_exec"}) |
| policy_decision_audit_split_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_policy_decision_stage_errors_total{action="SEND",stage=~"decision_audit_pool_acquire\|decision_audit_insert_exec"}[5m])) |
| policy_evaluator_revision_lookup_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="policy_revision_lookup"}) |
| policy_evaluator_facts_read_p99_ms | True | 1 | 25 | 12 | 87 | 12 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="policy_facts_read"}) |
| policy_evaluator_decision_cache_get_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="decision_cache_get"}) |
| policy_evaluator_decision_cache_set_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="decision_cache_set"}) |
| policy_evaluator_contact_block_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="contact_block_lookup"}) |
| policy_evaluator_user_restriction_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="user_restriction_lookup"}) |
| policy_evaluator_role_gate_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="role_gate"}) |
| policy_evaluator_rebac_gate_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="rebac_gate"}) |
| policy_evaluator_tenant_quota_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="tenant_quota_lookup"}) |
| policy_evaluator_message_action_rule_p99_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="message_action_rule_lookup"}) |
| policy_evaluator_exact_rule_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="exact_rule_lookup"}) |
| policy_evaluator_tenant_rule_p99_ms | False | 0 | 0 |  |  |  | max(nexusim_policy_evaluator_stage_latency_p99_milliseconds{action="SEND",stage="tenant_rule_lookup"}) |
| policy_evaluator_stage_errors_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_policy_evaluator_stage_errors_total[5m])) |
| policy_pg_pool_acquire_total | True | 1 | 25 | 35215 | 40915 | 40915 | max(nexusim_policy_pg_pool_acquire_total) |
| policy_pg_pool_acquire_duration_ms_total | True | 1 | 25 | 200691 | 203923 | 203923 | max(nexusim_policy_pg_pool_acquire_duration_milliseconds_total) |
| policy_pg_pool_empty_acquire_total | True | 1 | 25 | 4750 | 5207 | 5207 | max(nexusim_policy_pg_pool_empty_acquire_total) |
| policy_pg_pool_canceled_acquire_total | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_pg_pool_canceled_acquire_total) |
| policy_pg_pool_conns_acquired | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_pg_pool_conns{state="acquired"}) |
| policy_pg_pool_conns_max | True | 1 | 25 | 32 | 32 | 32 | max(nexusim_policy_pg_pool_conns{state="max"}) |
| policy_audit_pg_pool_acquire_total | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_acquire_total) |
| policy_audit_pg_pool_acquire_duration_ms_total | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_acquire_duration_milliseconds_total) |
| policy_audit_pg_pool_empty_acquire_total | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_empty_acquire_total) |
| policy_audit_pg_pool_canceled_acquire_total | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_canceled_acquire_total) |
| policy_audit_pg_pool_conns_acquired | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_audit_pg_pool_conns{state="acquired"}) |
| policy_audit_pg_pool_conns_max | True | 1 | 25 | 32 | 32 | 32 | max(nexusim_policy_audit_pg_pool_conns{state="max"}) |
| policy_audit_outbox_pending | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_audit_outbox{state="pending"}) |
| policy_audit_outbox_published | True | 1 | 25 | 253524 | 253524 | 253524 | max(nexusim_policy_audit_outbox{state="published"}) |
| policy_audit_outbox_dlq | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_policy_audit_outbox{state="dlq"}) |
| policy_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_policy_outbox_relay_errors_total[5m])) |
| policy_outbox_relay_consecutive_errors | False | 0 | 0 |  |  |  | max(nexusim_policy_outbox_relay_consecutive_errors) |
| delivery_outbox_pending | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending"}) |
| delivery_outbox_pending_ready | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="pending_ready"}) |
| delivery_outbox_dlq | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox{state="dlq"}) |
| delivery_projection_unresolved | False | 0 | 0 |  |  |  | max(nexusim_delivery_projection_failures{state="unresolved_total"}) |
| delivery_timeline_worker_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_timeline_worker_errors_total[5m])) |
| delivery_timeline_worker_consecutive_errors | False | 0 | 0 |  |  |  | max(nexusim_delivery_timeline_worker_consecutive_errors) |
| delivery_timeline_worker_last_commit_age_seconds | False | 0 | 0 |  |  |  | max(time() - (nexusim_delivery_timeline_worker_last_commit_unix_milliseconds / 1000)) |
| delivery_outbox_relay_errors_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[5m])) |
| delivery_outbox_relay_consecutive_errors | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox_relay_consecutive_errors) |
| delivery_outbox_relay_workers | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox_relay_workers) |
| delivery_outbox_relay_last_run_duration_ms | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox_relay_last_run_duration_milliseconds) |
| delivery_outbox_relay_last_publish_duration_ms | False | 0 | 0 |  |  |  | max(nexusim_delivery_outbox_relay_last_publish_duration_milliseconds) |
| delivery_outbox_relay_fetched_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_fetched_total[5m])) |
| delivery_outbox_relay_published_5m | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_published_total[5m])) |
| delivery_grpc_rps | True | 1 | 25 | 0 | 0.189 | 0 | sum(rate(nexusim_delivery_grpc_method_requests_total[5m])) |
| delivery_grpc_errors_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_delivery_grpc_method_errors_total[5m])) |
| push_connected_sessions | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_push_gateway_sessions{state="connected"}) |
| push_slow_evicted_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_session_events_total{event="slow_evicted"}[5m])) |
| push_writer_events_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total[5m])) |
| push_writer_outbound_dequeued_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"}[5m])) |
| push_writer_frame_write_attempt_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_attempt"}[5m])) |
| push_writer_frame_write_success_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[5m])) |
| push_writer_frame_write_error_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[5m])) |
| push_writer_delivery_notify_attempt_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_attempt"}[5m])) |
| push_writer_delivery_notify_success_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[5m])) |
| push_writer_delivery_notify_error_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[5m])) |
| push_writer_frame_write_duration_p95_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_frame_write_duration_p99_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="frame_write"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p95_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_duration_avg_ms_5m | True | 1 | 25 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_duration_max_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"}) |
| push_writer_delivery_notify_queue_p95_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[5m])) by (le)) |
| push_writer_delivery_notify_queue_avg_ms_5m | True | 1 | 25 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_sum{operation="delivery_notify"}[5m])) / sum(rate(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_count{operation="delivery_notify"}[5m])) |
| push_writer_delivery_notify_queue_max_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_push_gateway_ws_writer_queue_duration_max_milliseconds{operation="delivery_notify"}) |
| push_consumer_worker_errors_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_consumer_worker_errors_total[5m])) |
| push_redis_route_events_5m | True | 1 | 25 | 1931.203 | 10048.286 | 1931.203 | sum(increase(nexusim_push_gateway_redis_route_events_total[5m])) |
| push_redis_registry_remote_matched_sessions_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_matched_sessions"}[5m])) |
| push_redis_registry_remote_publish_call_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_call"}[5m])) |
| push_redis_registry_remote_publish_error_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_publish_error"}[5m])) |
| push_redis_registry_remote_no_subscriber_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_no_subscriber"}[5m])) |
| push_redis_registry_conversation_route_cache_hit_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[5m])) |
| push_redis_registry_conversation_route_cache_miss_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[5m])) |
| push_redis_registry_conversation_route_cache_invalidated_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_registry_remote_enqueued_sessions_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="remote_enqueued_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_matched_sessions_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[5m])) |
| push_redis_delivery_consumer_remote_publish_call_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[5m])) |
| push_redis_delivery_consumer_remote_publish_error_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[5m])) |
| push_redis_delivery_consumer_remote_no_subscriber_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_no_subscriber"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_5m | True | 1 | 25 | 917.945 | 4725.867 | 917.945 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_5m | True | 1 | 25 | 98.716 | 562.704 | 98.716 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[5m])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[5m])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[5m])) |
| push_redis_subscriber_messages_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[5m])) |
| push_redis_subscriber_enqueued_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[5m])) |
| push_redis_subscriber_evicted_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[5m])) |
| push_redis_subscriber_errors_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queued_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_full_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[5m])) |
| push_redis_subscriber_signal_fanout_worker_errors_5m | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[5m])) |
| push_redis_subscriber_signal_fanout_queue_depth | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_depth) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_avg_ms_5m | True | 1 | 25 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_sum{operation="conversation_signal"}[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_count{operation="conversation_signal"}[5m])) |
| push_redis_subscriber_signal_fanout_duration_max_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_fanout_duration_max_milliseconds{operation="conversation_signal"}) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.95, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_5m | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.99, sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[5m])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_avg_ms_5m | True | 1 | 25 | NaN | NaN | NaN | sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_sum[5m])) / sum(rate(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_count[5m])) |
| push_redis_subscriber_signal_fanout_queue_wait_max_ms | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_max_milliseconds) |
| push_writer_frame_write_success_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_success"}[747s])) |
| push_writer_frame_write_error_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="frame_write_error"}[747s])) |
| push_writer_delivery_notify_success_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"}[747s])) |
| push_writer_delivery_notify_error_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_error"}[747s])) |
| push_writer_delivery_notify_duration_p95_ms_window | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[747s])) by (le)) |
| push_writer_delivery_notify_duration_p99_ms_window | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{operation="delivery_notify"}[747s])) by (le)) |
| push_writer_delivery_notify_queue_p95_ms_window | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[747s])) by (le)) |
| push_writer_delivery_notify_queue_p99_ms_window | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_ws_writer_queue_duration_milliseconds_bucket{operation="delivery_notify"}[747s])) by (le)) |
| message_outbox_process_ready_active_samples_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_message_latency_samples_total{operation="outbox_process_ready_active"}[747s])) |
| message_kafka_publish_call_samples_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_message_latency_samples_total{operation="kafka_publish_call"}[747s])) |
| message_outbox_relay_errors_window | False | 0 | 0 |  |  |  | sum(increase(nexusim_message_outbox_relay_errors_total[747s])) |
| policy_outbox_relay_errors_window | False | 0 | 0 |  |  |  | sum(increase(nexusim_policy_outbox_relay_errors_total[747s])) |
| delivery_outbox_relay_errors_window | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_errors_total[747s])) |
| delivery_outbox_relay_fetched_window | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_fetched_total[747s])) |
| delivery_outbox_relay_published_window | False | 0 | 0 |  |  |  | sum(increase(nexusim_delivery_outbox_relay_published_total[747s])) |
| push_redis_subscriber_messages_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_message"}[747s])) |
| push_redis_subscriber_enqueued_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_enqueued"}[747s])) |
| push_redis_subscriber_evicted_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_evicted"}[747s])) |
| push_redis_subscriber_errors_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_error"}[747s])) |
| push_redis_registry_conversation_route_cache_hit_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_hit"}[747s])) |
| push_redis_registry_conversation_route_cache_miss_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_miss"}[747s])) |
| push_redis_registry_conversation_route_cache_invalidated_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="registry",event="conversation_route_cache_invalidated"}[747s])) |
| push_redis_delivery_consumer_remote_matched_sessions_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_matched_sessions"}[747s])) |
| push_redis_delivery_consumer_remote_publish_call_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_call"}[747s])) |
| push_redis_delivery_consumer_remote_publish_error_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_publish_error"}[747s])) |
| push_redis_delivery_consumer_conversation_route_cache_hit_window | True | 1 | 25 | 2425.971 | 9081.894 | 7091.783 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_hit"}[747s])) |
| push_redis_delivery_consumer_conversation_route_cache_miss_window | True | 1 | 25 | 295.751 | 1081.371 | 843.001 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_miss"}[747s])) |
| push_redis_delivery_consumer_conversation_route_cache_invalidated_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="conversation_route_cache_invalidated"}[747s])) |
| push_redis_delivery_consumer_remote_enqueued_sessions_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="delivery-consumer",event="remote_enqueued_sessions"}[747s])) |
| push_redis_subscriber_signal_fanout_queued_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queued"}[747s])) |
| push_redis_subscriber_signal_fanout_queue_full_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_queue_full"}[747s])) |
| push_redis_subscriber_signal_fanout_worker_errors_window | True | 1 | 25 | 0 | 0 | 0 | sum(increase(nexusim_push_gateway_redis_route_events_total{role="subscriber",event="subscriber_signal_fanout_worker_error"}[747s])) |
| push_redis_subscriber_signal_fanout_duration_p95_ms_window | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[747s])) by (le)) |
| push_redis_subscriber_signal_fanout_duration_p99_ms_window | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_fanout_duration_milliseconds_bucket{operation="conversation_signal"}[747s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p95_ms_window | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.95, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[747s])) by (le)) |
| push_redis_subscriber_signal_fanout_queue_wait_p99_ms_window | True | 1 | 25 | NaN | NaN | NaN | histogram_quantile(0.99, sum(increase(nexusim_push_gateway_redis_subscriber_signal_fanout_queue_wait_duration_milliseconds_bucket[747s])) by (le)) |
| message_pg_pool_acquire_total | True | 1 | 25 | 0 | 5021 | 5001 | max(nexusim_message_pg_pool_acquire_total) |
| message_pg_pool_acquire_duration_ms_total | True | 1 | 25 | 0 | 43091 | 43091 | max(nexusim_message_pg_pool_acquire_duration_milliseconds_total) |
| message_pg_pool_empty_acquire_total | True | 1 | 25 | 0 | 425 | 425 | max(nexusim_message_pg_pool_empty_acquire_total) |
| message_pg_pool_canceled_acquire_total | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_pg_pool_canceled_acquire_total) |
| message_pg_pool_conns_acquired | True | 1 | 25 | 0 | 0 | 0 | max(nexusim_message_pg_pool_conns{state="acquired"}) |
| message_pg_pool_conns_max | True | 1 | 25 | 192 | 192 | 192 | max(nexusim_message_pg_pool_conns{state="max"}) |
| message_pg_pool_conns | True | 1 | 25 | 192 | 192 | 192 | max(nexusim_message_pg_pool_conns) |
| conversation_pg_pool_conns | True | 1 | 25 | 32 | 32 | 32 | max(nexusim_conversation_pg_pool_conns) |
| delivery_pg_pool_conns | True | 1 | 25 | 16 | 16 | 16 | max(nexusim_delivery_pg_pool_conns) |

## Interpretation

- Use this report with the matching hotgroup summary and analysis report.
- Metrics ending in _5m show the moving five-minute pressure window; metrics ending in _window approximate the whole captured run window.
- Metrics with no data mean the exporter or scrape target did not expose that series in this window.
- Do not use this single window as production capacity evidence.
