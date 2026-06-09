# conversation-service ListConversationMembers Smoke

## 目标

验证 `conversation-service` 新增的当前成员列表读接口不是只停留在单元测试里，而是能通过真实 gRPC 进程读取 `conversation_members`：

```text
loadtest/memberchange
-> conversation-service gRPC CreateMemberChange
-> PostgreSQL conversation_members
-> conversation-service gRPC ListConversationMembers
-> memberchange-summary.json
```

本轮不是容量压测，也不验证完整群管理。它只证明第一版 roster API 能列出当前 `ACTIVE` 成员，并保持“其它服务不跨表读取 `conversation_members`”的低耦合边界。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `99aacc6` |
| git_dirty | `false` |
| PostgreSQL | `nexusim-postgres` |
| conversation-service | 本机进程，`127.0.0.1:13496` |
| 原始结果 | `H:\NexusIM\loadtest-results\conversation-members-smoke-20260610-clean\memberchange-summary.json` |

## 执行方式

先 seed 一个 active conversation 和 owner：

```text
tenant_id = tenant-roster-smoke
conversation_id = conv-roster-smoke
owner = owner-1
```

然后运行：

```powershell
bin\memberchange-loadtest.exe `
  --target 127.0.0.1:13496 `
  --vus 1 `
  --duration 1s `
  --request-count 3 `
  --request-timeout 2s `
  --tenant-id tenant-roster-smoke `
  --conversation-id conv-roster-smoke `
  --operator-user-id owner-1 `
  --target-prefix roster-user `
  --idempotency-prefix roster-idem `
  --expected-member-version 0 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir H:\NexusIM\loadtest-results\conversation-members-smoke-20260610-clean
```

`loadtest/memberchange` 在 3 条 `CreateMemberChange(JOIN)` 后调用一次 `ListConversationMembers`，并把成员列表采样写入 summary。

## 关键结果

| 指标 | 结果 |
| --- | --- |
| request_count | `3` |
| success_count | `3` |
| error_count | `0` |
| success_rate | `1.0` |
| p95 / p99 | `52.204ms / 52.204ms` |
| saga_count | `3` |
| conversation_seq_current | `3` |
| member_list_count | `4` |
| member_list_error | 空 |
| member_list_sample_users | `owner-1`, `roster-user-1`, `roster-user-2`, `roster-user-3` |

`member_list_count=4` 符合预期：seed 的 owner 加上 3 个新 JOIN 的 active member。

## 结论

- `ListConversationMembers` 真实 gRPC 入口可用。
- 当前接口只返回 `ACTIVE` 成员，适合作为第三层群管理的最小 roster read path。
- 该能力没有新增服务、没有让其它服务直接读 `conversation_members`，也没有把成员历史 / 审计列表塞进普通成员列表。

## 限制

- 本轮只验证 `JOIN -> ACTIVE roster`。
- 本报告只覆盖 `JOIN -> ACTIVE roster`；`LEAVE -> roster excludes target` 已由 `loadtest-report-20260610-list-conversation-members-leave-smoke.md` 覆盖，`REMOVE -> roster excludes target` 已由 `loadtest-report-20260610-list-conversation-members-remove-smoke.md` 覆盖。
- 未覆盖 `ROLE_CHANGED` 后的 roster role 更新。
- 未验证大群分页容量；当前只证明 keyset 分页契约和真实进程可用。
- 未启动 outbox relay / member-change-worker，所以 summary 里 `outbox_pending_count=3`、`saga_done_count=0` 是预期现象。
