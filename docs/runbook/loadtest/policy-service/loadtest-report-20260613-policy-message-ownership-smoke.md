# policy-service message ownership integration smoke - 2026-06-13

## Scope

This smoke verifies the first-stage sender-only message ownership gate through the real `message-service -> policy-service` RPC boundary.

It covers:

- `message-service` reading the original sender from its own `message_log`;
- `message-service` forwarding `message_sender_user_id` to `policy-service`;
- `policy-service CheckMessageAction`;
- `EditMessage`, `RevokeMessage`, and `DeleteMessage` public gRPC APIs;
- `policy_decision_audit_outbox` rows for both the baseline send and the target mutation.

It does not cover admin / moderator override, full ReBAC, compliance delete, user-private delete, or capacity.

## Command

```powershell
. .\tools\go-env.ps1
go build -o bin\policy-message-loadtest.exe ./loadtest/policyintegration
.\loadtest\policyintegration\run-local-smoke.ps1 `
  -RunName policy-message-ownership-smoke-20260613-final2 `
  -Actions edit,revoke,delete `
  -UseOwnershipGate `
  -SkipBuild
```

Raw result directory:

```text
H:\NexusIM\loadtest-results\policy-message-ownership-smoke-20260613-final2
```

## Environment

```text
commit=e638913
git_dirty=false
```

The runner starts:

- `policy-service` in `grpc` mode with `NEXUSIM_POLICY_RULES_ENABLED=true`;
- `message-service` in `grpc` mode with `NEXUSIM_POLICY_SERVICE_ADDR` set;
- local static policy recovery as allow with `classification=POLICY_OWNERSHIP_FALLTHROUGH_ALLOWED`;
- no exact or tenant policy rules.

This setup makes non-sender denies prove the ownership gate: if the ownership gate did not run, the static default would allow the mutation.

## Results

| action | scenario | actor | gRPC | audit allowed | audit classification | seq after target | history rows after target |
| --- | --- | --- | --- | --- | --- | ---: | ---: |
| edit | allow | sender | OK | true | POLICY_OWNERSHIP_FALLTHROUGH_ALLOWED | 2 | 1 |
| edit | deny | non-sender | PermissionDenied | false | MESSAGE_OWNERSHIP_DENIED | 1 | 0 |
| revoke | allow | sender | OK | true | POLICY_OWNERSHIP_FALLTHROUGH_ALLOWED | 2 | 1 |
| revoke | deny | non-sender | PermissionDenied | false | MESSAGE_OWNERSHIP_DENIED | 1 | 0 |
| delete | allow | sender | OK | true | POLICY_OWNERSHIP_FALLTHROUGH_ALLOWED | 2 | 1 |
| delete | deny | non-sender | PermissionDenied | false | MESSAGE_OWNERSHIP_DENIED | 1 | 0 |

All six scenarios recorded:

```text
policy_audit_rows=2
message_id_present=true
message_key_present=true
```

For deny scenarios:

```text
change_user_id=policy-message-non-sender
message_error_code=MESSAGE_ERROR_CODE_PERMISSION_DENIED
message_error_retryable=false
policy_audit_reason_code=MESSAGE_OWNERSHIP_DENIED
conversation_seq remains 1
message_change_history remains 0
message_log / timeline / outbox counts remain unchanged after the target action
```

For allow scenarios:

```text
change_user_id=policy-message-user
conversation_seq advances from 1 to 2
message_change_history rows = 1
timeline event type matches message.edited.v1 / message.revoked.v1 / message.deleted.v1
```

## Interpretation

The allow scenarios prove that the original sender can still mutate their own message when policy falls through to allow.

The deny scenarios prove that a different actor is rejected by `policy-service` before rule/static default. The latest audit row for each target mutation is `MESSAGE_OWNERSHIP_DENIED`, has `message_id_present=true`, and has a non-empty stable `message_key`. This prevents the result from being confused with message-service's final transactional sender check.

## Limits

This is a targeted real-process smoke, not a capacity result. It uses message-service's static conversation context and policy-service's static allow recovery. It proves sender-only ownership enforcement for message mutation actions, not a complete moderation or authorization model.

Remaining future work:

- admin / moderator mutation override;
- full ReBAC and tenant policy DSL;
- retention / compliance delete semantics;
- external audit sink and broader policy repair workflow.
