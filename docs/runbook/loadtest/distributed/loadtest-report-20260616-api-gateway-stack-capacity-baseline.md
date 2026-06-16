# Api Gateway Stack Capacity Baseline - 2026-06-16

## Scope

This is a local short baseline for the api-gateway stack path. It is not a production capacity test or SLO claim.

Covered path:

```text
api-gateway GatewayService facade
-> conversation / message / delivery / receipt gRPC over mTLS
-> policy-service decision and audit outbox
-> message timeline outbox relay
-> delivery timeline projection and delivery outbox relay
-> push-gateway WebSocket notify and delivery ACK
-> receipt delivery consumer / outbox relay / conversation summary
```

Security mode:

- Entry gRPC TLS and client cert verification enabled.
- Downstream service mTLS enabled.
- Gateway auth mode: `hmac`.
- Legacy descriptors disabled: `NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=false`.

## Raw Result

Raw output is stored outside the Git workspace:

```text
H:\NexusIM\loadtest-results\capacity-baseline-api-gateway-stack-clean-20260616
```

Summary file:

```text
H:\NexusIM\loadtest-results\capacity-baseline-api-gateway-stack-clean-20260616\e2e-demo-summary.json
```

Metrics snapshot:

```text
H:\NexusIM\loadtest-results\capacity-baseline-api-gateway-stack-clean-20260616\api-gateway-debug-metrics.json
```

## Result

The smoke passed on clean commit `0f2b5957b970be94ca4700b47349885991d73713`.

```text
git_dirty = false
success = true
gateway_facade = true
gateway_auth_mode = hmac
duration_seconds = 2.1295866
operations_per_second = 3.2870229367521375
user_facing_operation_count = 7
websocket_frame_count = 3
items_pulled = 1
max_conversation_seq = 2
unread_before_read = 1
unread_after_read = 0
postgres_user_inbox_count = 1
postgres_summary_count = 1
policy_audit_kafka_events = 1
```

Functional evidence:

```text
server.hello succeeded
CreateMemberChange JOIN boundary_seq = 1
SendMessage conversation_seq = 2
delivery.notify received for seq = 2
PullInbox item_count = 1, max_seq = 2
delivery.ack.ok last_received_seq = 2
MarkRead last_read_seq = 2
ListConversations unread_count changed 1 -> 0
policy.message_action_decision.v1 Kafka readback event_count = 1
```

## Api Gateway Metrics Snapshot

The debug metrics snapshot recorded:

```text
grpc.total_requests = 7
grpc.total_errors = 1
grpc.facade_requests = 7
runtime.register_legacy_descriptors = false
rate_limit.enabled = false
trace.enabled = false
```

The single gRPC error was one transient `MarkRead` `FailedPrecondition` during the runner's wait loop while receipt projection caught up. The final business result succeeded: `MarkRead` reached seq `2` and conversation unread changed from `1` to `0`.

## Environment Note

The first run hit a local port conflict on default api-gateway debug port `11904`, held by Docker / WSL port forwarding on this machine. `loadtest/demo/run-local-secure-demo.ps1` now accepts explicit gRPC and debug port parameters while preserving existing default ports.

The clean run used this alternate local port set:

```text
conversation = 13096
message = 13095
delivery = 13097
push = 13098
receipt = 13099
policy = 13100
policy_debug = 13101
push_debug = 13102
api_gateway = 13103
api_gateway_debug = 13104
```

## Limitations

- This is a short local stack baseline, not a throughput or saturation test.
- It uses one tenant, one conversation, one sender, one receiver device, and one message.
- It does not prove target-environment legacy quiet-window observation, production Alertmanager routing, production collector retention, or DB-backed quota control-plane integration.
- It does not replace longer capacity curves or resource saturation testing.
