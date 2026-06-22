# policy-service message role gate integration smoke - 2026-06-13

## Scope

This smoke verifies that the conversation role gate is enforced through the real `message-service -> policy-service` RPC boundary for `SendMessage`.

It covers:

- seeded `policy_conversation_role_action_rules`;
- seeded `policy_conversation_members_projection`;
- `policy-service CheckMessageAction`;
- `message-service SendMessage` public gRPC API;
- `policy_decision_audit_outbox` decision rows.

It does not cover Kafka projection from `conversation.timeline.events`; that is covered by `loadtest-report-20260613-policy-conversation-role-smoke.md`.

## Command

```powershell
. .\tools\go-env.ps1
.\loadtest\policyintegration\run-local-smoke.ps1 -UseConversationRoleGate -Actions send -RunName policy-message-role-gate-smoke-20260613-clean
```

Raw result directory:

```text
H:\NexusIM\loadtest-results\policy-message-role-gate-smoke-20260613-clean
```

## Environment

```text
commit=773ad4c
git_dirty=false
```

The runner starts:

- `policy-service` in `grpc` mode with `NEXUSIM_POLICY_RULES_ENABLED=true`;
- `message-service` in `grpc` mode with `NEXUSIM_POLICY_SERVICE_ADDR` set;
- static conversation mock with `NEXUSIM_MOCK_PERMISSION_VERSION` aligned to the scenario;
- local static policy recovery with opposite sentinel values so a missed RPC path cannot pass silently.

## Results

### Allow

Seeded projection:

```text
role=ADMIN
status=ACTIVE
permission_version=41
```

Observed:

```text
grpc_code=OK
message_log=1
conversation_timeline_events=1
message_outbox=1
conversation_seq=1
message_permission_version=41
message_classification=POLICY_ROLE_GATE_TENANT_ALLOW
policy_audit_rows=1
policy_audit_allowed=true
policy_audit_classification=POLICY_ROLE_GATE_TENANT_ALLOW
policy_audit_reason_code=
```

### Deny

Seeded projection:

```text
role=MEMBER
status=ACTIVE
permission_version=42
```

Observed:

```text
grpc_code=PermissionDenied
message_error_code=MESSAGE_ERROR_CODE_PERMISSION_DENIED
message_log=0
conversation_timeline_events=0
message_outbox=0
conversation_seq=0
policy_audit_rows=1
policy_audit_allowed=false
policy_audit_classification=CONVERSATION_ROLE_DENIED
policy_audit_reason_code=CONVERSATION_ROLE_DENIED
```

## Interpretation

The allow scenario proves that an active `ADMIN` projection passes the conversation role gate and then falls through to the tenant-level allow rule. The accepted message carries the expected `permission_version` and classification into `message_log` and `conversation_timeline_events`.

The deny scenario proves that an active `MEMBER` projection is rejected by the role gate before message-service starts its write transaction. No message, timeline event, outbox row or sequence allocation is created.

The audit assertions are part of the runner. They prove that the deny is produced by policy-service as `CONVERSATION_ROLE_DENIED`, not inferred from the public `permission denied` message exposed by message-service.

## Limits

This is a targeted integration smoke, not a capacity result. It uses seeded member projection rows. The separate role projection smoke covers `conversation.timeline.events -> policy_conversation_members_projection -> CheckMessageAction`.

Remaining future work:

- full ReBAC and admin / moderator mutation override;
- sender-only ownership is covered separately by `loadtest-report-20260613-policy-message-ownership-smoke.md`;
- tenant policy DSL / quota / risk policy;
- broader audit retention and external sink;
- broader poison-payload repair classification.
