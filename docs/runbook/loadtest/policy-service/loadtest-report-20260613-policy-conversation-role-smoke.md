# policy-service Conversation Role Gate Smoke - 2026-06-13

## Scope

This smoke verifies the first-stage conversation role gate with real Kafka and PostgreSQL:

```text
conversation.timeline.events
-> policy-service timeline-consumer
-> policy_conversation_members_projection
-> policy-service CheckMessageAction(SEND)
```

It is not a capacity test and does not prove full ReBAC, message ownership policy, tenant DSL, quota or risk policy.

## Command

```powershell
. .\tools\go-env.ps1
.\loadtest\policyroles\run-local-smoke.ps1 -RunName policy-conversation-role-smoke-20260613-clean
```

Raw result directory:

```text
H:\NexusIM\loadtest-results\policy-conversation-role-smoke-20260613-clean
```

## Environment

- Commit: `e86de33`
- Full commit: `e86de3336754682b5328f14c712c67c27e503626`
- `git_dirty`: `false`
- PostgreSQL: `nexusim-postgres` on localhost `5432`
- Kafka: `nexusim-kafka` on localhost `9092`
- Topic: `conversation.timeline.events.policy-conversation-role-smoke-20260613-clean`
- Consumer group: `nexusim-policy-timeline-policy-conversation-role-smoke-20260613-clean`

## Result

The smoke passed.

Key facts from `policy-role-summary.json`:

```json
{
  "success": true,
  "joined_projection": {
    "role": "ADMIN",
    "status": "ACTIVE",
    "member_version": 7,
    "permission_version": 7
  },
  "allowed_decision": {
    "allowed": true,
    "permission_version": 101,
    "conversation_permission_version": 7,
    "classification": "POLICY_TENANT_ALLOW"
  },
  "role_changed_projection": {
    "role": "MEMBER",
    "status": "ACTIVE",
    "member_version": 8,
    "permission_version": 8
  },
  "role_denied_decision": {
    "allowed": false,
    "permission_version": 8,
    "conversation_permission_version": 8,
    "classification": "CONVERSATION_ROLE_DENIED",
    "reason": "conversation role policy denied"
  },
  "stale_decision": {
    "conversation_permission_version": 7,
    "error_code": "Unavailable",
    "error_message": "policy unavailable"
  },
  "left_projection": {
    "role": "MEMBER",
    "status": "LEFT",
    "member_version": 9,
    "permission_version": 9
  },
  "inactive_denied_decision": {
    "allowed": false,
    "permission_version": 9,
    "conversation_permission_version": 9,
    "classification": "CONVERSATION_ROLE_DENIED",
    "reason": "conversation role policy denied"
  },
  "checkpoint_offset_value": 3
}
```

## Interpretation

- The `ADMIN / ACTIVE / permission_version=7` member event was projected and then allowed through the role gate; final allow came from the seeded tenant action rule.
- The `MEMBER / ACTIVE / permission_version=8` role change was projected and denied by the role gate because the smoke rule requires at least `ADMIN`.
- A request carrying stale `conversation_permission_version=7` after the projection reached version `8` returned `Unavailable / policy unavailable`, proving fail-closed freshness behavior.
- The `MEMBER / LEFT / permission_version=9` event was projected and denied even though the role exists, proving inactive member status is not treated as authorized.
- The Kafka checkpoint reached `3`, matching the three timeline events written by the smoke.

## Remaining Work

- Wire this role-gate scenario into a full message-service integration smoke so `SendMessage` denial is observed through the public message API.
- Add message ownership policy for `EDIT / REVOKE / DELETE`; the current role gate is action-level and does not decide ownership.
- Add richer ReBAC / tenant policy DSL / quota / risk policy only after the first-stage role and contact gates remain stable.
