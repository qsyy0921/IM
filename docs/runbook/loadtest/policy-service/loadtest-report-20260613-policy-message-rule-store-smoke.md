# policy-service message rule-store integration smoke - 2026-06-13

## Scope

This smoke validates the first PostgreSQL-backed policy rule slice:

```text
policy_message_action_rules
-> policy-service CheckMessageAction
-> message-service SendMessage
```

It is not a contacts / conversation / tenant / risk policy engine test. The rule store is exact-match only in this slice: `tenant_id + user_id + conversation_id + action`.

## Raw Result

- Raw directory: `H:\NexusIM\loadtest-results\policy-message-rules-smoke-20260613`
- Implementation commit: `3666901 feat: add policy message rule store`
- Runner dirty flag: `false`
- Script:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 `
  -RunName policy-message-rules-smoke-20260613 `
  -UsePolicyRules
```

The script applies message and policy PostgreSQL migrations, starts `policy-service` with `NEXUSIM_POLICY_RULES_ENABLED=true`, starts `message-service` with `NEXUSIM_POLICY_SERVICE_ADDR`, seeds exact policy rules, and runs allow / deny SendMessage scenarios.

## Evidence

### Allow Rule

- Seeded rule: `SEND`, `allowed=true`, `permission_version=41`, `classification=POLICY_RPC_ALLOWED`.
- `SendMessage` returned `OK`.
- `message_id`: `msg_3a95461c-090b-4ecc-a7bb-fe982bdf156f`
- `conversation_seq`: `1`
- Database delta:
  - `message_log`: `0 -> 1`
  - `conversation_timeline_events`: `0 -> 1`
  - `message_outbox`: `0 -> 1`
- Persisted message / timeline metadata:
  - `permission_version=41`
  - `classification=POLICY_RPC_ALLOWED`

### Deny Rule

- Seeded rule: `SEND`, `allowed=false`, `permission_version=42`, `classification=POLICY_RPC_BLOCKED`, `reason=blocked by policy integration smoke`.
- `SendMessage` returned `PermissionDenied`.
- `MessageError.code`: `MESSAGE_ERROR_CODE_PERMISSION_DENIED`
- `MessageError.retryable`: `false`
- Database delta:
  - `message_log`: `0 -> 0`
  - `conversation_timeline_events`: `0 -> 0`
  - `message_outbox`: `0 -> 0`

## Conclusion

The exact-match policy rule store works through the real service path. A matching allow rule is persisted into message-service metadata; a matching deny rule rejects before message writes and outbox enqueue.

The important safety boundary is also covered by tests: PostgreSQL lookup errors return dependency unavailable and do not silently fall back to the static policy. Only a clean miss falls back to static policy.

## Limits

- Only exact rule matching is implemented.
- No wildcard / priority rule DSL.
- No contacts block, conversation role, tenant policy, risk scoring or policy audit outbox.
- `permission_version` must still be aligned with the conversation permission version for allow decisions.
