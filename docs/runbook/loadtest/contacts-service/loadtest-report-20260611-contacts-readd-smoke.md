# contacts-service Re-add Smoke

日期：2026-06-11

## 目的

验证联系人被当前 owner 单向删除后，可以通过重新申请 / 接受恢复联系人关系，同时 `contacts_outbox` 对同一 canonical user pair 的 `aggregate_version` 保持单调递增。

本轮不是容量压测，只验证真实进程小闭环。

## 方法

运行脚本：

```powershell
.\loadtest\contacts\run-local-smoke.ps1 -Scenario readd -RunName contacts-readd-smoke-clean-20260611-165922
```

链路：

```text
SendContactRequest
-> RespondContactRequest(ACCEPT)
-> DeleteContact(owner=sender)
-> SendContactRequest(second)
-> RespondContactRequest(ACCEPT second)
-> contacts_outbox
-> contacts-service outbox-relay
-> Kafka im.contact.events
-> ListContacts / GetContactState
```

原始结果：

```text
H:\NexusIM\loadtest-results\contacts-readd-smoke-clean-20260611-165922\contacts-summary.json
```

## 结果

```text
commit=f3ad8b4
git_dirty=false
success=true
scenario=readd
contacts_outbox: total=5, published=5, pending=0, dlq=0
sender_state=ACTIVE, version=3
receiver_state=ACTIVE, version=2
sender_list.contact_count=1
receiver_list.contact_count=1
```

Kafka 读回事件：

| 顺序 | event_type | aggregate_version |
| --- | --- | --- |
| 1 | `contact.request.created.v1` | 1 |
| 2 | `contact.request.accepted.v1` | 2 |
| 3 | `contact.edge.deleted.v1` | 3 |
| 4 | `contact.request.created.v1` | 4 |
| 5 | `contact.request.accepted.v1` | 5 |

## 结论

- `DeleteContact` 仍是 owner 单向 edge 操作，不删除对方视角 edge，不自动改会话或消息事实。
- 重新申请 / 接受可以恢复 sender 视角联系人，最终双方 `GetContactState` 都回到 `ACTIVE`。
- 同一 canonical user pair 的 contacts Kafka 事件版本保持 `1..5` 单调递增，后续消费者可以按 `partition_key + aggregate_version` 处理顺序。
- contacts-service 仍保持独立事实源：不写 `conversation_members`，不创建会话，不让 message-service 同步依赖 contacts-service。

## 限制

- 本轮只覆盖同一 sender 删除后再次申请同一 receiver 的 happy path。
- 不覆盖删除后反向申请、拉黑状态下重新申请、并发 re-add 或下游 push / audit consumer。
- contacts outbox 仍是 at-least-once，消费者必须按 `event_id` 幂等。
