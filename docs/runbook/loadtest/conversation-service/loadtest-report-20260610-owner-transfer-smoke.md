# conversation-service TransferConversationOwner Smoke

## 目标

验证 owner transfer 不是把两次 `ROLE_CHANGED` 拼在一起，而是一个独立、低耦合的群管理写路径：

```text
seed current OWNER + active ADMIN
-> conversation-service gRPC TransferConversationOwner
-> PostgreSQL local transaction
-> one member_change_saga + one conversation_seq + one timeline event + one outbox event
-> message-service outbox relay publishes conversation.member.owner_transferred.v1
-> conversation-service member-change-worker marks saga DONE
-> ListConversationMembers shows old owner ADMIN and new owner OWNER
```

本轮不是容量压测，只验证单实例、本机真实进程的 owner transfer 最小闭环。

## 环境

| 项 | 值 |
| --- | --- |
| commit | `490db1a` |
| git_dirty | `false` |
| PostgreSQL | `nexusim-postgres` |
| Kafka | `nexusim-kafka` |
| conversation-service gRPC | 本机动态端口，summary 中为 `127.0.0.1:60984` |
| message-service outbox relay | 本机进程 |
| conversation-service member-change-worker | 本机进程 |
| 原始结果 | `H:\NexusIM\loadtest-results\conversation-owner-transfer-smoke-20260610-072627\memberchange-summary.json` |
| Kafka topic | `conversation.timeline.ownertransfer.20260610-072627` |

## 执行方式

使用可复现脚本：

```powershell
.\loadtest\memberchange\run-local-smoke.ps1
```

脚本做了以下事情：

1. 构建 `conversation-service`、`message-service` 和 `memberchange-loadtest`。
2. 应用 message / conversation 相关 PostgreSQL migration，包括 `000004_owner_transfer_contract.sql`。
3. 创建临时 Kafka timeline topic。
4. seed 一个 `GROUP / ACTIVE / LOCAL_ROW_LOCK` 会话：
   - `owner-1`：`ACTIVE OWNER`
   - `owner-transfer-user`：`ACTIVE ADMIN`
   - `member_version=10`，`permission_version=20`
5. 启动三个真实进程：
   - `conversation-service` gRPC
   - `message-service` outbox relay
   - `conversation-service` member-change-worker
6. 调用 `TransferConversationOwner`，并在 summary 中校验 outbox drain、saga DONE 和 roster 角色。

## 关键结果

| 指标 | 结果 |
| --- | --- |
| request_count | `1` |
| success_count | `1` |
| error_count | `0` |
| success_rate | `1.0` |
| p95 / p99 | `65.286ms / 65.286ms` |
| change_type | `OWNER_TRANSFER` |
| previous_owner_user_id | `owner-1` |
| new_owner_user_id | `owner-transfer-user` |
| sample_get_status | `MEMBER_CHANGE_STATUS_DONE` |
| saga_count / saga_done_count | `1 / 1` |
| timeline_count | `1` |
| outbox_total / published / pending / DLQ | `1 / 1 / 0 / 0` |
| conversation_seq_current | `1` |
| member_list_count | `2` |
| active owner count | `1` |
| old owner role/status | `MEMBER_ROLE_ADMIN / MEMBER_STATUS_ACTIVE` |
| new owner role/status | `MEMBER_ROLE_OWNER / MEMBER_STATUS_ACTIVE` |
| new owner member_version / permission_version | `11 / 21` |

## 排查过程

第一次试跑失败，错误是：

```text
unknown method TransferConversationOwner for service nexusim.conversation.v1.ConversationService
```

排查后确认不是业务代码问题，而是脚本固定使用 `127.0.0.1:13496`，该端口上已有旧 conversation-service 进程，旧二进制没有 `TransferConversationOwner` 方法。修复方式是让 `run-local-smoke.ps1` 动态选择空闲端口，并把 runner 指向本轮新启动的 gRPC 进程。

第二次在 clean commit `490db1a` 上复跑，`git_dirty=false`，链路通过。

## 结论

- owner transfer 已完成最小真实进程闭环：一个 RPC、一个本地事务、一个 seq、一个 timeline/outbox 事件、一个 saga DONE。
- roster 读模型可正确反映转移后的当前成员角色：旧 owner 仍 ACTIVE 但降级为 ADMIN，新 owner 成为唯一 ACTIVE OWNER。
- outbox relay 已把 owner transfer 事件发布并标记 `PUBLISHED`，没有遗留 PENDING 或 DLQ。
- 该实现保持低耦合：其它服务不直接读写 `conversation_members`，成员事实仍由 conversation-service 拥有；消息事件仍通过统一 outbox relay 发布 Kafka。

## 限制

- 本轮只验证单次 `OWNER -> ADMIN / ADMIN -> OWNER`。
- 未覆盖 owner transfer 的并发 smoke、非法操作者、目标非 ACTIVE、targeted repair / replay。
- 未做容量压测；这只是功能闭环 smoke。
- 未单独做 Kafka consumer readback；本轮以 message-service outbox relay 成功标记 `PUBLISHED` 作为 Kafka publish 成功证据。
