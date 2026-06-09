# conversation-service ListConversationMembers ROLE_CHANGED Smoke

## 目标

验证 `ROLE_CHANGED` 后，普通 `ListConversationMembers` 不只是保留目标成员，还能返回更新后的当前角色：

```text
seed owner + active member
-> owner calls CreateMemberChange(ROLE_CHANGED, target_role=ADMIN)
-> PostgreSQL conversation_members.role = ADMIN
-> owner calls ListConversationMembers
-> target user remains ACTIVE and role is ADMIN
```

本轮不是容量压测，也不验证完整群管理。它只证明第一版 roster API 会反映当前 ACTIVE 成员的最新角色，不承担成员历史、审计列表或 owner transfer。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `7150944` |
| git_dirty | `false` |
| PostgreSQL | `nexusim-postgres` |
| conversation-service | 本机进程，`127.0.0.1:13496` |
| 原始结果 | `H:\NexusIM\loadtest-results\conversation-members-role-smoke-20260610-050635\memberchange-summary.json` |

## 执行方式

先 seed 一个 active group conversation：

```text
tenant_id = tenant-roster-role-smoke
conversation_id = conv-roster-role-smoke
owner = owner-1 ACTIVE OWNER
target = roster-user-role ACTIVE MEMBER
```

然后运行：

```powershell
bin\memberchange-loadtest.exe `
  --target 127.0.0.1:13496 `
  --vus 1 `
  --duration 3s `
  --request-count 1 `
  --request-timeout 3s `
  --tenant-id tenant-roster-role-smoke `
  --conversation-id conv-roster-role-smoke `
  --operator-user-id owner-1 `
  --list-user-id owner-1 `
  --target-user-id roster-user-role `
  --change-type role-changed `
  --target-role admin `
  --idempotency-prefix roster-role-20260610-050635 `
  --expected-member-version 0 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir H:\NexusIM\loadtest-results\conversation-members-role-smoke-20260610-050635
```

## 关键结果

| 指标 | 结果 |
| --- | --- |
| request_count | `1` |
| success_count | `1` |
| error_count | `0` |
| success_rate | `1.0` |
| p95 / p99 | `14.045ms / 14.045ms` |
| operator_user_id | `owner-1` |
| list_user_id | `owner-1` |
| change_type | `ROLE_CHANGED` |
| target_role | `ADMIN` |
| target_user_id | `roster-user-role` |
| saga_count | `1` |
| saga_done_count | `0` |
| timeline_count | `1` |
| outbox_total_count | `1` |
| outbox_pending_count | `1` |
| conversation_seq_current | `1` |
| sample_get_status | `MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED` |
| member_list_count | `2` |
| member_list_sample_users | `owner-1`, `roster-user-role` |
| member_list_target_present | `true` |
| member_list_target_role | `MEMBER_ROLE_ADMIN` |
| member_list_target_status | `MEMBER_STATUS_ACTIVE` |
| member_list_target_member_version | `6` |
| member_list_target_permission_version | `8` |

数据库复核：

```text
owner-1,OWNER,ACTIVE,,5,7
roster-user-role,ADMIN,ACTIVE,,6,8
```

目标成员仍为 ACTIVE，并且角色从 `MEMBER` 更新为 `ADMIN`。

## 结论

- `CreateMemberChange(ROLE_CHANGED)` 后，`ListConversationMembers` 能返回目标成员的最新角色。
- 普通 roster API 的边界保持清晰：只反映当前 ACTIVE roster 和当前 role；历史角色变化、审计和 owner transfer 后续单独设计。
- 该验证继续遵守低耦合原则：其它服务不跨表读取 `conversation_members`，需要成员列表时走 conversation-service API。

## 限制

- 本轮只验证 `MEMBER -> ADMIN`。
- 未覆盖 `ADMIN -> MEMBER`、非法 owner transfer、非 owner 操作和完整权限负例。
- 未启动 outbox relay / member-change-worker，所以 summary 里 `outbox_pending_count=1`、`saga_done_count=0` 是预期现象。
- 不验证 admin-only 成员历史查询、邀请审批、BANNED / ban 流程或 DLQ repair。
