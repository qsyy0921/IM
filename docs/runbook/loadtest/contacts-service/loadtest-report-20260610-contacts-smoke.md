# contacts-service contacts smoke

日期：2026-06-10

结果文件：

```text
H:\NexusIM\loadtest-results\contacts-smoke-20260610-154550\contacts-summary.json
```

代码基线：

```text
commit=584017f
commit_full=584017f24890809ae46df87d31ed6a3cc6d3035e
git_dirty=false
```

## 目标

验证 `contacts-service` 最小真实链路：

```text
SendContactRequest
-> RespondContactRequest(ACCEPT)
-> contacts_outbox
-> contacts-service outbox-relay
-> Kafka im.contact.events
-> ListContacts / GetContactState
```

这不是容量压测，只证明功能闭环和关键边界成立。

## 方法

1. 使用本机已有 `nexusim-postgres` 和 `nexusim-kafka` 容器，不拉取新镜像。
2. 创建本次独立 Kafka topic：

```text
im.contact.events.contacts-smoke.20260610-154550
```

3. 启动两个本地进程：

```text
contacts-service grpc
contacts-service outbox-relay
```

4. runner 调用 gRPC：

```text
SendContactRequest(sender -> receiver)
RespondContactRequest(receiver ACCEPT)
ListContacts(sender)
ListContacts(receiver)
GetContactState(sender -> receiver)
GetContactState(receiver -> sender)
```

5. runner 查询 PostgreSQL outbox 状态，并从 Kafka topic 读回 protobuf `ContactEvent`。

## 结果

核心结果：

```text
success=true
tenant_id=tenant-contacts-smoke-20260610-154550
request_id=contact_req_c44f8f2ea06999b36d76858479636ef7
send_status=CONTACT_REQUEST_STATUS_PENDING
respond_status=CONTACT_REQUEST_STATUS_ACCEPTED
sender_list=[contacts-receiver]
receiver_list=[contacts-sender]
sender_state=CONTACT_EDGE_STATUS_ACTIVE
receiver_state=CONTACT_EDGE_STATUS_ACTIVE
contacts_outbox total=2 pending=0 published=2 dlq=0
```

Kafka 读回事件：

| event_type | aggregate_version | status | partition_key |
| --- | ---: | --- | --- |
| `contact.request.created.v1` | 1 | `PENDING` | `tenant-contacts-smoke-20260610-154550:contacts-receiver:contacts-sender` |
| `contact.request.accepted.v1` | 2 | `ACCEPTED` | `tenant-contacts-smoke-20260610-154550:contacts-receiver:contacts-sender` |

本次延迟仅作为 smoke 参考，不做容量结论：

| step | latency_ms |
| --- | ---: |
| SendContactRequest | 43.908 |
| RespondContactRequest | 8.399 |
| ListContacts(sender) | 3.273 |
| ListContacts(receiver) | 0.858 |
| GetContactState(sender) | 0.503 |
| GetContactState(receiver) | 0.638 |

## 结论

`contacts-service` 第一阶段最小闭环通过。

已证明：

- `SendContactRequest` 会写入 PENDING 申请和 `contact.request.created.v1` outbox。
- `RespondContactRequest(ACCEPT)` 会写入双向 ACTIVE contact edge 和 `contact.request.accepted.v1` outbox。
- contacts outbox relay 能把两条事件发布到 `im.contact.events`，并把 outbox 标为 PUBLISHED。
- Kafka 事件使用同一个 canonical user pair partition key，顺序为 created -> accepted。
- `ListContacts` 和 `GetContactState` 能从 contacts-service 自身事实表读出双向 ACTIVE 关系。

边界仍保持：

- 不写 `conversation_members`。
- 不自动创建 direct conversation。
- 不让 message / delivery / push 同步依赖 contacts-service。

## 后续

短期可继续：

- 补 `DECLINE` 真实进程 smoke。
- 设计好友删除 / 拉黑 / 备注名等后续联系人能力。
- 如果要接受好友后创建单聊，先补 saga / app port 设计，避免跨服务内部表写入。
