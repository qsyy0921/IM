# conversation-service ListConversationMembers REMOVE Smoke

## 目标

验证 `ListConversationMembers` 不只是能列出 `JOIN` 后的 ACTIVE 成员，也能在成员被 `REMOVE` 后按当前 roster 语义过滤掉目标成员：

```text
seed owner + active member
-> conversation-service gRPC CreateMemberChange(REMOVE)
-> PostgreSQL conversation_members.status = LEFT
-> conversation-service gRPC ListConversationMembers
-> target user is absent from ACTIVE roster
```

本轮不是容量压测，也不验证完整群管理。它只证明第一版普通成员列表是“当前 ACTIVE roster”，不承担成员历史、审计列表或 admin-only 历史查询。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `be2e039` |
| git_dirty | `false` |
| PostgreSQL | `nexusim-postgres` |
| conversation-service | 本机进程，`127.0.0.1:13496` |
| 原始结果 | `H:\NexusIM\loadtest-results\conversation-members-remove-smoke-20260610-045357\memberchange-summary.json` |

## 执行方式

先 seed 一个 active group conversation：

```text
tenant_id = tenant-roster-remove-smoke
conversation_id = conv-roster-remove-smoke
owner = owner-1 ACTIVE OWNER
target = roster-user-remove ACTIVE MEMBER
```

然后运行：

```powershell
bin\memberchange-loadtest.exe `
  --target 127.0.0.1:13496 `
  --vus 1 `
  --duration 3s `
  --request-count 1 `
  --request-timeout 3s `
  --tenant-id tenant-roster-remove-smoke `
  --conversation-id conv-roster-remove-smoke `
  --operator-user-id owner-1 `
  --target-user-id roster-user-remove `
  --change-type remove `
  --idempotency-prefix roster-remove-20260610-045357 `
  --expected-member-version 0 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir H:\NexusIM\loadtest-results\conversation-members-remove-smoke-20260610-045357
```

`loadtest/memberchange` 在 `CreateMemberChange(REMOVE)` 后调用一次 `ListConversationMembers`，并分页扫描 ACTIVE roster，判断 `target_user_id` 是否仍然存在。

## 关键结果

| 指标 | 结果 |
| --- | --- |
| request_count | `1` |
| success_count | `1` |
| error_count | `0` |
| success_rate | `1.0` |
| p95 / p99 | `52.274ms / 52.274ms` |
| change_type | `REMOVE` |
| target_user_id | `roster-user-remove` |
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
roster-user-remove,LEFT,1,6,8
```

目标成员被写成 `LEFT`，`leave_seq=1`，并且不再出现在普通 ACTIVE roster。

## 结论

- `CreateMemberChange(REMOVE)` 后，`ListConversationMembers` 会过滤掉目标成员。
- 普通 roster API 的边界保持清晰：只返回当前 ACTIVE 成员，不返回历史成员、被移除成员或审计信息。
- 该验证继续遵守低耦合原则：其它服务不跨表读取 `conversation_members`，需要成员列表时走 conversation-service API。

## 限制

- 本轮只验证 `REMOVE -> LEFT -> roster excludes target`。
- 未覆盖 `LEAVE / ROLE_CHANGED` 后的 roster 变化。
- 未启动 outbox relay / member-change-worker，所以 summary 里 `outbox_pending_count=1`、`saga_done_count=0` 是预期现象。
- 不验证 admin-only 成员历史查询、owner transfer、BANNED / ban 流程或 DLQ repair。
