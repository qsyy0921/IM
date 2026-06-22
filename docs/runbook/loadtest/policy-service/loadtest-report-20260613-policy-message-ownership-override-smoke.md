# policy-service message ownership override smoke

Date: 2026-06-13

Raw result directory:

```text
H:\NexusIM\loadtest-results\policy-message-ownership-override-smoke-20260613-r2
```

## Scope

This smoke verifies the first-stage non-sender message mutation override:

```text
policy_message_ownership_override_rules
-> policy_conversation_members_projection
-> policy-service CheckMessageAction ownership_override=true
-> message-service mutation repository typed ownership override
```

It is not a capacity test and not a full ReBAC / moderator-role implementation. The current role model still has only `OWNER`, `ADMIN`, and `MEMBER`; this smoke uses `ADMIN` as the first-stage moderation override role.

The run was executed before the final commit, so the raw summary records `git_dirty=true`. That is expected for this development smoke; source-level checks are run separately after the files are committed.

## Result

All six scenarios passed.

| Action | Scenario | Projected Role | gRPC Result | DB Effect | Audit Classification |
| --- | --- | --- | --- | --- | --- |
| edit | allow | ADMIN / ACTIVE | OK | seq 1 -> 2, change history +1, timeline/outbox +1 | MESSAGE_OWNERSHIP_ROLE_OVERRIDE |
| edit | deny | MEMBER / ACTIVE | PermissionDenied | seq stays 1, no change history, no mutation timeline/outbox | MESSAGE_OWNERSHIP_DENIED |
| revoke | allow | ADMIN / ACTIVE | OK | seq 1 -> 2, change history +1, timeline/outbox +1 | MESSAGE_OWNERSHIP_ROLE_OVERRIDE |
| revoke | deny | MEMBER / ACTIVE | PermissionDenied | seq stays 1, no change history, no mutation timeline/outbox | MESSAGE_OWNERSHIP_DENIED |
| delete | allow | ADMIN / ACTIVE | OK | seq 1 -> 2, change history +1, timeline/outbox +1 | MESSAGE_OWNERSHIP_ROLE_OVERRIDE |
| delete | deny | MEMBER / ACTIVE | PermissionDenied | seq stays 1, no change history, no mutation timeline/outbox | MESSAGE_OWNERSHIP_DENIED |

Key invariant checked by the runner:

- base `SendMessage` uses a tenant `SEND` allow seed and writes `POLICY_SEND_SEED`;
- target `EDIT` / `REVOKE` / `DELETE` is issued by a non-sender user;
- allow scenarios seed `policy_message_ownership_override_rules(min_role=ADMIN)` and an `ADMIN / ACTIVE` projection at the same `conversation_permission_version`;
- deny scenarios keep the same override rule but seed a `MEMBER / ACTIVE` projection;
- allow scenarios write the expected mutation facts;
- deny scenarios leave `conversation_seq`, `message_change_history`, `conversation_timeline_events`, `message_outbox`, and the original message row unchanged after the target action;
- latest policy audit row carries `message_id_present=true` and `message_key_present=true`.

## Design Notes

The override is a typed proof, not a classification-string convention.

policy-service returns `ownership_override=true` only for explicit ownership override allow decisions. message-service rejects that flag for `SEND` and for denied decisions. The mutation repositories then allow a non-sender transaction only when `PermissionDecision.OwnershipOverride` is true.

This keeps the low-coupling boundary:

- message-service still owns message facts and reads the original sender from `message_log`;
- policy-service owns policy rules, projected conversation roles, and audit;
- exact rules, tenant rules, and static default cannot silently bypass sender ownership;
- missing or stale conversation member projection fails closed before an override can be issued.

## Remaining Work

- Separate `MODERATOR` role is not implemented.
- Full ReBAC, owner-transfer moderation semantics, compliance delete, retention policy, and user-private delete remain future work.
- This smoke does not cover stale projection at process level; focused PostgreSQL tests cover stale projection fail-closed for the override checker.
