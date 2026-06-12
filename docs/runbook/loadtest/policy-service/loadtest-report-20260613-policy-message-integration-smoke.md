# policy-service + message-service Integration Smoke - 2026-06-13

## Scope

This smoke validates the first `policy-service -> message-service` runtime integration:

```text
policy-service grpc
-> message-service NEXUSIM_POLICY_SERVICE_ADDR
-> MessageService.SendMessage
-> message_log / conversation_timeline_events / message_outbox
```

It proves that `message-service` can use the policy RPC adapter for `SendMessage` allow and deny decisions. It does not validate contacts block projection, conversation role policy, tenant policy, risk scoring, Kafka publish, delivery projection, or push notification.

## Environment

- Repository commit: `102cf970f8d07708cf8146064d5a710df049f601`
- Git dirty: `false`
- Command: `.\loadtest\policyintegration\run-local-smoke.ps1`
- Raw result directory: `H:\NexusIM\loadtest-results\policy-message-smoke-20260613-072623`
- `policy-service` mode: `NEXUSIM_POLICY_SERVICE_MODE=grpc`
- `message-service` mode: `NEXUSIM_MESSAGE_SERVICE_MODE=grpc`
- `message-service` policy adapter: `NEXUSIM_POLICY_SERVICE_ADDR=<policy-service addr>`
- `NEXUSIM_POLICY_RPC_TIMEOUT=2s`

The smoke intentionally sets the local message-service mock policy opposite to the remote policy decision. That prevents a false positive if `NEXUSIM_POLICY_SERVICE_ADDR` is not used.

Process log evidence:

```text
message-service using policy-service at 127.0.0.1:50883
message-service gRPC server started on 127.0.0.1:50884
message-service using policy-service at 127.0.0.1:50893
message-service gRPC server started on 127.0.0.1:50894
```

## Results

Both scenarios passed.

### Allow

- Policy marker: `allowed=true`, `permission_version=41`, `classification=POLICY_RPC_ALLOWED`
- Message gRPC result: `OK`
- Message ID: `msg_d8431638-dbc1-4964-9e67-bd2e14e7dc08`
- Conversation seq: `1`

Database evidence:

| Table | Before | After |
| --- | ---: | ---: |
| `message_log` | 0 | 1 |
| `conversation_timeline_events` | 0 | 1 |
| `message_outbox` | 0 | 1 |

Persisted policy fields:

| Field | Value |
| --- | --- |
| `message_log.permission_version` | 41 |
| `message_log.classification` | POLICY_RPC_ALLOWED |
| `conversation_timeline_events.permission_version` | 41 |
| `conversation_timeline_events.classification` | POLICY_RPC_ALLOWED |
| `message_outbox.status` | PENDING |

### Deny

- Policy marker: `allowed=false`, `permission_version=42`, `classification=POLICY_RPC_BLOCKED`
- Policy reason: `blocked by policy integration smoke`
- Message gRPC result: `PermissionDenied`
- MessageError code: `MESSAGE_ERROR_CODE_PERMISSION_DENIED`
- MessageError public message: `permission denied`
- Retryable: `false`

Database evidence:

| Table | Before | After |
| --- | ---: | ---: |
| `message_log` | 0 | 0 |
| `conversation_timeline_events` | 0 | 0 |
| `message_outbox` | 0 | 0 |

The deny case does not write a policy decision to message tables by design; it rejects before message persistence.

## Conclusion

`policy-service` now has both:

- direct public gRPC smoke for `CheckMessageAction`;
- `message-service` integration smoke for `SendMessage` allow / deny.

Keep the conclusion narrow. This proves the first-stage RPC policy boundary is runnable and can gate `SendMessage`. It is still an environment-driven static policy service, not a contacts / role / tenant / risk policy engine, and this smoke does not prove Kafka relay, delivery, or push behavior.
