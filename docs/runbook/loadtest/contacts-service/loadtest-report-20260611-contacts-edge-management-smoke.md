# contacts-service edge management smoke

日期：2026-06-11

结果文件：

```text
H:\NexusIM\loadtest-results\contacts-delete-smoke-clean-20260611-163517\contacts-summary.json
H:\NexusIM\loadtest-results\contacts-block-smoke-clean-20260611-163554\contacts-summary.json
H:\NexusIM\loadtest-results\contacts-remark-smoke-clean-20260611-163634\contacts-summary.json
```

代码基线：

```text
commit=c1893c1
git_dirty=false
```

## 目标

验证 `contacts-service` v0.2 三条联系人边管理链路：

```text
SendContactRequest
-> RespondContactRequest(ACCEPT)
-> DeleteContact / BlockContact / UpdateContactRemark
-> contacts_outbox
-> contacts-service outbox-relay
-> Kafka im.contact.events
-> ListContacts / GetContactState
```

这不是容量压测，只证明本地真实进程、PostgreSQL 事务、outbox relay 和 Kafka 事件闭环成立。

## 方法

1. 使用本机 Docker `nexusim-postgres` 和 `nexusim-kafka`。
2. 每个 scenario 创建独立 tenant 和 Kafka topic。
3. 启动 `contacts-service grpc` 与 `contacts-service outbox-relay` 两个本地进程。
4. runner 先建立 ACCEPT 后的双向 ACTIVE 好友关系，再分别执行 `delete`、`block`、`remark`。
5. runner 验证 `ListContacts`、`GetContactState`、`contacts_outbox` 和 Kafka 读回事件。

## 结果

| scenario | event_type | owner final status | sender list count | receiver list count | outbox published | pending | dlq |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| `delete` | `contact.edge.deleted.v1` | `CONTACT_EDGE_STATUS_DELETED` | 0 | 1 | 3 | 0 | 0 |
| `block` | `contact.edge.blocked.v1` | `CONTACT_EDGE_STATUS_BLOCKED` | 0 | 1 | 3 | 0 | 0 |
| `remark` | `contact.edge.remark_updated.v1` | `CONTACT_EDGE_STATUS_ACTIVE` | 1 | 1 | 3 | 0 | 0 |

Kafka 读回顺序均为：

```text
contact.request.created.v1
contact.request.accepted.v1
contact.edge.<deleted|blocked|remark_updated>.v1
```

三条链路的 `aggregate_version` 均保持 `1 -> 2 -> 3`，验证了同一 canonical user pair partition 上的事件版本单调递增。

## 修复点

本轮 smoke 前修复了两个评审发现的问题：

- `SendContactRequest` / `RespondContactRequest` 不再固定 outbox `aggregate_version=1/2`，统一通过当前 partition 的最大版本递增，避免删除后重新添加好友时版本倒退。
- `UpdateContactRemark` 的幂等 replay 保存并返回原命令结果快照，避免后续 remark 更新后 replay 旧 key 返回当前 edge。

## 结论

`contacts-service` v0.2 最小真实闭环通过。

已证明：

- `DeleteContact` 只删除当前 owner 视角的联系人 edge，不影响对方 edge。
- `BlockContact` 只把当前 owner 视角 edge 标为 BLOCKED，并从当前 owner 的联系人列表隐藏。
- `UpdateContactRemark` 更新当前 owner 视角备注，不影响对方 edge。
- 三类 edge 事件均通过 contacts outbox relay 发布到 Kafka，并把 outbox 标为 PUBLISHED。

边界仍保持：

- 不写 `conversation_members`。
- 不自动创建或删除会话。
- 不让 `message-service` 同步依赖 `contacts-service`。

## 后续

后续可继续：

- 补好友 re-add / unblock 的 SDD 和最小链路。
- 如果要“接受好友后自动创建单聊”，先补显式 saga / app port 设计。
- 生产化前补 contacts 事件 consumer、审计视图和权限接入。
