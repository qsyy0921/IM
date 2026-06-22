# policy-service tenant message action rule smoke - 2026-06-13

## Scope

This smoke verifies the first-stage tenant action default table:

```text
policy_tenant_message_action_rules
tenant_id + action
```

The test runs through the real `policy-service -> message-service` gRPC path. It proves that tenant-scoped action defaults can allow or deny `SEND`, `EDIT`, `REVOKE`, and `DELETE` without seeding an exact `user_id + conversation_id + action` rule.

This is still not a full tenant policy engine. It does not cover tenant policy DSL, quotas, risk scoring, conversation role policy, group policy or content moderation.

## Raw Result

- Raw directory: `H:\NexusIM\loadtest-results\policy-message-tenant-rule-actions-smoke-20260613-clean`
- Runner commit: `13fcc46 feat: add tenant message policy rules`
- Runner dirty flag: `false`
- Script:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 `
  -RunName policy-message-tenant-rule-actions-smoke-20260613-clean `
  -UseTenantPolicyRules `
  -Actions send,edit,revoke,delete
```

The script applies message and policy PostgreSQL migrations, starts `policy-service` with `NEXUSIM_POLICY_RULES_ENABLED=true`, starts `message-service` with `NEXUSIM_POLICY_SERVICE_ADDR`, seeds tenant action rules, and runs allow / deny scenarios for each action. The local static policy is deliberately configured to the opposite decision so a tenant rule miss is visible.

For mutation actions, the runner also seeds a tenant-level `SEND / POLICY_SEND_SEED` allow rule so the baseline message can be created before the `EDIT`, `REVOKE`, or `DELETE` decision is tested.

## Result Matrix

| Action | Scenario | gRPC | Tenant rules seeded | Persisted classification / denial evidence |
| --- | --- | --- | --- | --- |
| send | allow | OK | `SEND:POLICY_RPC_SEND_ALLOWED` | `message_log.classification=POLICY_RPC_SEND_ALLOWED` |
| send | deny | PermissionDenied | `SEND:POLICY_RPC_SEND_BLOCKED` | `MESSAGE_ERROR_CODE_PERMISSION_DENIED`, no message rows written |
| edit | allow | OK | `SEND:POLICY_SEND_SEED`, `EDIT:POLICY_RPC_EDIT_ALLOWED` | base message `POLICY_SEND_SEED`, change timeline `POLICY_RPC_EDIT_ALLOWED` |
| edit | deny | PermissionDenied | `SEND:POLICY_SEND_SEED`, `EDIT:POLICY_RPC_EDIT_BLOCKED` | base message unchanged, no edit timeline/change-history delta |
| revoke | allow | OK | `SEND:POLICY_SEND_SEED`, `REVOKE:POLICY_RPC_REVOKE_ALLOWED` | base message `POLICY_SEND_SEED`, change timeline `POLICY_RPC_REVOKE_ALLOWED` |
| revoke | deny | PermissionDenied | `SEND:POLICY_SEND_SEED`, `REVOKE:POLICY_RPC_REVOKE_BLOCKED` | base message unchanged, no revoke timeline/change-history delta |
| delete | allow | OK | `SEND:POLICY_SEND_SEED`, `DELETE:POLICY_RPC_DELETE_ALLOWED` | base message `POLICY_SEND_SEED`, change timeline `POLICY_RPC_DELETE_ALLOWED` |
| delete | deny | PermissionDenied | `SEND:POLICY_SEND_SEED`, `DELETE:POLICY_RPC_DELETE_BLOCKED` | base message unchanged, no delete timeline/change-history delta |

All eight scenarios reported:

```text
success=true
commit=13fcc46
git_dirty=false
tenant_policy_rule_seeded=true
```

## Exact + Tenant Summary Check

A second short smoke verified that runner summaries distinguish tenant and exact rules when both stores are seeded:

```powershell
.\loadtest\policyintegration\run-local-smoke.ps1 `
  -RunName policy-message-exact-tenant-summary-smoke-20260613-clean `
  -UsePolicyRules `
  -UseTenantPolicyRules `
  -Actions send `
  -SkipBuild
```

Raw directory:

```text
H:\NexusIM\loadtest-results\policy-message-exact-tenant-summary-smoke-20260613-clean
```

Both allow and deny summaries contained two rules:

```text
tenant SEND rule with empty user_id/conversation_id
exact SEND rule with user_id/conversation_id
```

This check prevents the summary from making an exact-rule-over-tenant-rule run look tenant-only.

## Implementation Notes

Decision priority is:

```text
direct contact block
-> exact policy_message_action_rules row
-> tenant policy_tenant_message_action_rules row
-> static default
```

Exact rules intentionally override tenant defaults. The evaluator also treats a missing tenant-rule table as a clean tenant-rule miss, so a deployment that has not applied migration `000007_policy_tenant_message_action_rules.sql` yet does not break the older exact-rule / static-recovery path. Other PostgreSQL errors still fail closed as policy unavailable.

## Conclusion

policy-service now has a first-stage tenant action default rule store. The real-process smoke proves tenant rules can drive both allow and deny decisions for send and mutation actions through message-service, and deny decisions occur before mutation state is written.

This is enough to talk about tenant-level policy defaults in an interview. It is not enough to claim a complete tenant policy engine, role policy engine, risk system, quota system, or production policy DSL.
