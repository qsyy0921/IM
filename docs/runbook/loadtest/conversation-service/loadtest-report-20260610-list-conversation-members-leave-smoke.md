# conversation-service ListConversationMembers LEAVE Smoke

## 目标

验证用户本人 `LEAVE` 后，普通 `ListConversationMembers` 只返回当前 ACTIVE roster，不再返回已离开的目标成员：

```text
seed owner + active member
-> target user calls CreateMemberChange(LEAVE)
-> PostgreSQL conversation_members.status = LEFT
-> owner calls ListConversationMembers
-> target user is absent from ACTIVE roster
```

本轮不是容量压测，也不验证完整群管理。它只证明第一版普通成员列表按当前成员状态过滤，不承担成员历史、审计列表或 admin-only 历史查询。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `14ffedc` |
| git_dirty | `false` |
| PostgreSQL | `nexusim-postgres` |
| conversation-service | 本机进程，`127.0.0.1:13496` |
| 原始结果 | `H:\NexusIM\loadtest-results\conversation-members-leave-smoke-20260610-050022\memberchange-summary.json` |

## 执行方式

先 seed 一个 active group conversation：

```text
tenant_id = tenant-roster-leave-smoke
conversation_id = conv-roster-leave-smoke
owner = owner-1 ACTIVE OWNER
target = roster-user-leave ACTIVE MEMBER
```

然后运行：

```powershell
bin\memberchange-loadtest.exe `
  --target 127.0.0.1:13496 `
  --vus 1 `
  --duration 3s `
  --request-count 1 `
  --request-timeout 3s `
  --tenant-id tenant-roster-leave-smoke `
  --conversation-id conv-roster-leave-smoke `
  --operator-user-id roster-user-leave `
  --list-user-id owner-1 `
  --target-user-id roster-user-leave `
  --change-type leave `
  --idempotency-prefix roster-leave-20260610-050022 `
  --expected-member-version 0 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir H:\NexusIM\loadtest-results\conversation-members-leave-smoke-20260610-050022
```

这里 `operator-user-id` 是退群用户本人，符合第一阶段 `LEAVE` 语义；`list-user-id` 是仍然 ACTIVE 的 owner，用于后置读取当前 roster。

## 关键结果

| 指标 | 结果 |
| --- | --- |
| request_count | `1` |
| success_count | `1` |
| error_count | `0` |
| success_rate | `1.0` |
| p95 / p99 | `14.832ms / 14.832ms` |
| operator_user_id | `roster-user-leave` |
| list_user_id | `owner-1` |
| change_type | `LEAVE` |
| target_user_id | `roster-user-leave` |
| saga_count | `1` |
| saga_done_count | `0` |
| timeline_count | `1` |
| outbox_total_count | `1` |
| outbox_pending_count | `1` |
| conversation_seq_current | `1` |
| sample_get_status | `MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED` |
| member_list_count | `1` |
| member_list_sample_users | `owner-1` |
| member_list_target_present | `false` |
| member_list_target_absent_verified | `true` |

数据库复核：

```text
owner-1,ACTIVE,,5,7
roster-user-leave,LEFT,1,6,8
```

目标成员本人 LEAVE 后被写成 `LEFT`，`leave_seq=1`，并且不再出现在普通 ACTIVE roster。

## 结论

- `CreateMemberChange(LEAVE)` 的当前成员状态更新和普通 roster 过滤已通过真实进程 smoke。
- `ListConversationMembers` 保持低耦合边界：只暴露当前 ACTIVE roster；离开成员历史和审计视图后续单独设计。
- runner 的 `--list-user-id` 允许把“执行成员变更的人”和“后置查询 roster 的人”分开，避免自退用户已经不是 ACTIVE 成员后无法验证列表。

## 限制

- 本轮只验证 `LEAVE -> LEFT -> roster excludes target`。
- 未覆盖 `ROLE_CHANGED` 后的 roster role 更新。
- 未启动 outbox relay / member-change-worker，所以 summary 里 `outbox_pending_count=1`、`saga_done_count=0` 是预期现象。
- 不验证 admin-only 成员历史查询、owner transfer、BANNED / ban 流程或 DLQ repair。
