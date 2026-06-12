# policy-service message action rule-store integration smoke - 2026-06-13

## Scope

This smoke extends the policy-service -> message-service integration check from `SendMessage` to the existing message mutation commands:

```text
EditMessage
RevokeMessage
DeleteMessage
```

Each scenario first uses a seeded `SEND` allow rule to create a baseline message, then uses an exact `EDIT` / `REVOKE` / `DELETE` rule to validate allow or deny behavior through the real `message-service` gRPC path.

This is still a policy rule-store smoke, not a contacts / conversation role / tenant / risk policy engine test.

## Raw Result

- Raw directory: `H:\NexusIM\loadtest-results\policy-message-action-rules-smoke-20260613`
- Runner commit: `4017387 test: harden policy action smoke checks`
- Runner dirty flag: `false`
- Script:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 `
  -RunName policy-message-action-rules-smoke-20260613 `
  -UsePolicyRules `
  -Actions edit,revoke,delete
```

The script applies message and policy PostgreSQL migrations, starts `policy-service` with `NEXUSIM_POLICY_RULES_ENABLED=true`, starts `message-service` with `NEXUSIM_POLICY_SERVICE_ADDR`, seeds exact `SEND` + target-action policy rules, and runs allow / deny scenarios for each target action.

## Result Matrix

| Action | Scenario | gRPC | Message status | Timeline event / history | Action DB delta |
| --- | --- | --- | --- | --- | --- |
| edit | allow | OK | EDITED | `message.edited.v1` / `EDIT` | conversation_seq/timeline/outbox `1 -> 2`, change_history/idempotency `0 -> 1` |
| edit | deny | PermissionDenied | unchanged | none | conversation_seq/timeline/outbox/change_history/idempotency unchanged after base send |
| revoke | allow | OK | REVOKED | `message.revoked.v1` / `REVOKE` | conversation_seq/timeline/outbox `1 -> 2`, change_history/idempotency `0 -> 1` |
| revoke | deny | PermissionDenied | unchanged | none | conversation_seq/timeline/outbox/change_history/idempotency unchanged after base send |
| delete | allow | OK | DELETED | `message.deleted.v1` / `DELETE` | conversation_seq/timeline/outbox `1 -> 2`, change_history/idempotency `0 -> 1` |
| delete | deny | PermissionDenied | unchanged | none | conversation_seq/timeline/outbox/change_history/idempotency unchanged after base send |

All deny responses carried `MESSAGE_ERROR_CODE_PERMISSION_DENIED` and `retryable=false`.

## Conclusion

`EditMessage`, `RevokeMessage`, and `DeleteMessage` now have real-process evidence that message-service uses policy-service decisions for mutation commands, not only for `SendMessage`.

The allow scenarios verify the mutation response, exact `conversation_seq` advance, `message_change_history` type and before/after status, mutation timestamp, timeline event, and outbox row. The deny scenarios are the important safety evidence: after the baseline message exists, a target-action deny rule rejects before mutation state is written. No target mutation timeline event, outbox row, change history row, mutation idempotency row, or sequence advance is created.

## Limits

- The policy rule store is exact-match only.
- The baseline `SEND` rule is seeded only to create a message for mutation testing.
- This smoke does not cover contacts block projection, conversation role projection, tenant policy, risk scoring, policy audit outbox, or policy-service mTLS.
- It does not publish the mutation outbox rows to Kafka or project them into delivery; those paths are already covered by message-service and delivery-service message-change smokes.
