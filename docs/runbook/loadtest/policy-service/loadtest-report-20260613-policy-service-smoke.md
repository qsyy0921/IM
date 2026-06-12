# policy-service Direct gRPC Smoke - 2026-06-13

## Scope

This smoke validates the first-stage `policy-service` process and public gRPC contract:

```text
policy-service grpc
-> CheckMessageAction(SEND / EDIT / REVOKE / DELETE)
-> allow / deny response echo + permission_version + classification + reason
```

It does not validate contacts block projection, conversation role policy, tenant policy, risk scoring, policy audit outbox, or message-service database transactions.

## Environment

- Repository commit: `815a04f79a9a15c6728f573e906a4087646aba16`
- Git dirty: `false`
- Command: `.\loadtest\policy\run-local-smoke.ps1`
- Raw result directory: `H:\NexusIM\loadtest-results\policy-service-smoke-20260613-071709`
- Service mode: `NEXUSIM_POLICY_SERVICE_MODE=grpc`
- Runner: `loadtest/policy`

## Results

Both scenarios passed.

### Allow

- Target: `127.0.0.1:64949`
- Expected: `allowed=true`, `permission_version=31`, `classification=CONTACT_ALLOWED`
- Actions checked: `SEND`, `EDIT`, `REVOKE`, `DELETE`
- All responses echoed `tenant_id`, `user_id`, `conversation_id`, `action`, and `message_id` where applicable.

Observed action results:

| Action | Allowed | Permission Version | Classification | Reason |
| --- | --- | ---: | --- | --- |
| SEND | true | 31 | CONTACT_ALLOWED |  |
| EDIT | true | 31 | CONTACT_ALLOWED |  |
| REVOKE | true | 31 | CONTACT_ALLOWED |  |
| DELETE | true | 31 | CONTACT_ALLOWED |  |

### Deny

- Target: `127.0.0.1:64953`
- Expected: `allowed=false`, `permission_version=32`, `classification=CONTACT_BLOCKED`
- Expected reason: `blocked by policy smoke`
- Actions checked: `SEND`, `EDIT`, `REVOKE`, `DELETE`
- All responses echoed `tenant_id`, `user_id`, `conversation_id`, `action`, and `message_id` where applicable.

Observed action results:

| Action | Allowed | Permission Version | Classification | Reason |
| --- | --- | ---: | --- | --- |
| SEND | false | 32 | CONTACT_BLOCKED | blocked by policy smoke |
| EDIT | false | 32 | CONTACT_BLOCKED | blocked by policy smoke |
| REVOKE | false | 32 | CONTACT_BLOCKED | blocked by policy smoke |
| DELETE | false | 32 | CONTACT_BLOCKED | blocked by policy smoke |

## Conclusion

`policy-service` has a runnable first-stage gRPC boundary for message action decisions. The smoke proves direct process-level allow/deny behavior and response contract validation for all four message actions.

Keep the wording narrow: this is not a capacity test and not a complete policy engine. The next heavier integration should run `message-service` with `NEXUSIM_POLICY_SERVICE_ADDR` and keep `permission_version` aligned with conversation context to avoid expected dependency-version mismatch.
