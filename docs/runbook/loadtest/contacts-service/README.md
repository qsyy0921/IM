# contacts-service Loadtest Reports

本目录保存 `contacts-service` 的小规模验证报告和阶段总入口。

## 当前阶段结论

`contacts-service` 已完成联系人 / 好友关系第一条真实小闭环：

```text
SendContactRequest
-> contact_requests(PENDING)
-> contacts_outbox(contact.request.created.v1)
-> RespondContactRequest(ACCEPT)
-> contact_edges 双向 ACTIVE
-> contacts_outbox(contact.request.accepted.v1)
-> contacts-service outbox-relay
-> Kafka im.contact.events
-> ListContacts / GetContactState
```

本阶段重点不是容量，而是证明联系人关系作为独立事实源成立，并且保持低耦合：

- 不写 `conversation_members`。
- 不自动创建会话。
- 不让 `message-service` 同步依赖 `contacts-service`。
- Kafka 事件仍通过 outbox relay 发布，业务事务不直接 publish Kafka。

## 报告列表

| 报告 | 内容 |
| --- | --- |
| `loadtest-report-20260610-contacts-smoke.md` | `SendContactRequest -> RespondContactRequest(ACCEPT) -> contacts_outbox -> im.contact.events -> ListContacts / GetContactState` 真实进程 smoke |

## 面试可讲重点

- `contacts-service` 是第三层 IM 产品能力，专门管理好友申请和联系人关系。
- 好友关系和会话成员关系解耦：接受好友不会自动把用户写入某个会话，也不会直接创建 direct conversation。
- 好友申请和接受在 PostgreSQL 本地事务内写事实表与 outbox，Kafka 发布由 relay 异步完成，保持 at-least-once。
- 所有联系人事件按 canonical user pair 做 partition key，保证同一对用户的 created / accepted / declined 顺序不会被打乱。
- `ListContacts` / `GetContactState` 从 contacts-service 自己的 read model 读取，不跨服务读其它内部表。
- 当前 smoke 已验证双向 ACTIVE edge、outbox 清空、Kafka 读回 `contact.request.created.v1` 和 `contact.request.accepted.v1`。
- 后续如果要“接受好友后自动创建单聊”，应通过显式 saga / app port 编排，而不是在 contacts-service 事务里写 conversation-service 表。
