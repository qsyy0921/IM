# contacts-service decline smoke

日期：2026-06-10

结果文件：

```text
H:\NexusIM\loadtest-results\contacts-decline-smoke-20260610-155311\contacts-summary.json
```

代码基线：

```text
commit=3a392df
commit_full=3a392dfeb5e947a8f04dbe795804d1856c2c48cf
git_dirty=false
```

## 目标

验证联系人申请被拒绝时的最小真实链路：

```text
SendContactRequest
-> RespondContactRequest(DECLINE)
-> contacts_outbox
-> contacts-service outbox-relay
-> Kafka im.contact.events
-> ListContacts / GetContactState
```

这不是容量压测，只验证拒绝路径不会错误创建联系人边，并且拒绝事件能通过 outbox 发布。

## 方法

1. 使用本机已有 `nexusim-postgres` 和 `nexusim-kafka` 容器，不拉取新镜像。
2. 创建本次独立 Kafka topic：

```text
im.contact.events.contacts-decline-smoke.20260610-155311
```

3. 启动两个本地进程：

```text
contacts-service grpc
contacts-service outbox-relay
```

4. runner 调用 gRPC：

```text
SendContactRequest(sender -> receiver)
RespondContactRequest(receiver DECLINE)
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
tenant_id=tenant-contacts-decline-smoke-20260610-155311
request_id=contact_req_60f70962e2bb8169f8ef09cfff66b771
send_status=CONTACT_REQUEST_STATUS_PENDING
respond_status=CONTACT_REQUEST_STATUS_DECLINED
sender_list_count=0
receiver_list_count=0
sender_state_error=NotFound
receiver_state_error=NotFound
contacts_outbox total=2 pending=0 published=2 dlq=0
```

Kafka 读回事件：

| event_type | aggregate_version | status | partition_key |
| --- | ---: | --- | --- |
| `contact.request.created.v1` | 1 | `PENDING` | `tenant-contacts-decline-smoke-20260610-155311:contacts-receiver:contacts-sender` |
| `contact.request.declined.v1` | 2 | `DECLINED` | `tenant-contacts-decline-smoke-20260610-155311:contacts-receiver:contacts-sender` |

本次延迟仅作为 smoke 参考，不做容量结论：

| step | latency_ms |
| --- | ---: |
| SendContactRequest | 50.807 |
| RespondContactRequest | 8.158 |
| ListContacts(sender) | 1.661 |
| ListContacts(receiver) | 1.145 |
| GetContactState(sender) | 1.591 |
| GetContactState(receiver) | 1.274 |

## 结论

`contacts-service` DECLINE 路径最小闭环通过。

已证明：

- 拒绝好友申请不会创建 `contact_edges`。
- 双方 `ListContacts` 结果为空。
- 双向 `GetContactState` 返回 NotFound。
- contacts outbox relay 能发布 `contact.request.created.v1` 和 `contact.request.declined.v1`，并把 outbox 标为 PUBLISHED。
- 两条事件使用同一个 canonical user pair partition key，顺序为 created -> declined。

边界仍保持：

- 不写 `conversation_members`。
- 不自动创建 direct conversation。
- 不让 message / delivery / push 同步依赖 contacts-service。

## 后续

短期可继续：

- 设计好友删除 / 拉黑 / 备注名。
- 如果要接受好友后创建单聊，先补 saga / app port 设计，避免跨服务内部表写入。
