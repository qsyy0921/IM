# contacts-service unblock smoke

日期：2026-06-11

结果文件：

```text
H:\NexusIM\loadtest-results\contacts-unblock-smoke-clean-20260611-165139\contacts-summary.json
```

代码基线：

```text
commit=c799d09
git_dirty=false
```

## 目标

验证 `contacts-service` 的解除拉黑最小真实链路：

```text
SendContactRequest
-> RespondContactRequest(ACCEPT)
-> BlockContact
-> UnblockContact
-> contacts_outbox
-> contacts-service outbox-relay
-> Kafka im.contact.events
-> ListContacts / GetContactState
```

这不是容量压测，只证明 owner 视角 `BLOCKED -> ACTIVE` 状态恢复、outbox 和 Kafka 闭环成立。

## 方法

1. 使用本机 Docker `nexusim-postgres` 和 `nexusim-kafka`。
2. 创建独立 Kafka topic：

```text
im.contact.events.contacts-unblock-smoke.20260611-165139
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
BlockContact(sender -> receiver)
UnblockContact(sender -> receiver)
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
commit=c799d09
git_dirty=false
block_status=CONTACT_EDGE_STATUS_BLOCKED
unblock_status=CONTACT_EDGE_STATUS_ACTIVE
sender_state=CONTACT_EDGE_STATUS_ACTIVE
receiver_state=CONTACT_EDGE_STATUS_ACTIVE
sender_list_count=1
receiver_list_count=1
contacts_outbox total=4 pending=0 published=4 dlq=0
```

Kafka 读回事件：

| event_type | aggregate_version | status |
| --- | ---: | --- |
| `contact.request.created.v1` | 1 | `PENDING` |
| `contact.request.accepted.v1` | 2 | `ACCEPTED` |
| `contact.edge.blocked.v1` | 3 | `BLOCKED` |
| `contact.edge.unblocked.v1` | 4 | `ACTIVE` |

## 结论

`UnblockContact` 最小真实闭环通过。

已证明：

- `UnblockContact` 只允许当前 owner 视角从 `BLOCKED` 恢复到 `ACTIVE`。
- 解除拉黑不会写 `conversation_members`，不会自动创建会话，也不会直接修改 message-service 发送权限。
- `contact.edge.unblocked.v1` 通过 contacts outbox relay 发布到 Kafka，并保持同一 canonical user pair partition 上的版本单调递增。
- 解除后 `ListContacts` 双方可见，`GetContactState` 双方均为 ACTIVE。

## 后续

后续如果要把 BLOCKED 影响发送权限，应由 policy-service 或 conversation-service 的权限投影消费 contacts 事件，而不是让 message-service 同步依赖 contacts-service。
