# contacts-service CancelContactRequest Smoke

日期：2026-06-11

原始结果目录：

```text
H:\NexusIM\loadtest-results\contacts-cancel-smoke-clean-20260611-181310
```

代码版本：

```text
commit=f291aa5
commit_full=f291aa5758b2403e141e8457810c013db6b3e091
git_dirty=false
```

## 目标

验证 sender 可以取消自己发出的 pending 好友申请，并且取消不会创建联系人边：

```text
SendContactRequest
-> ListContactRequests(INCOMING, PENDING)
-> CancelContactRequest
-> ListContactRequests(INCOMING, PENDING)
-> ListContactRequests(OUTGOING, CANCELED)
-> contacts_outbox
-> im.contact.events
```

## 结果

本轮 smoke 通过，关键数据如下：

| 指标 | 结果 |
| --- | --- |
| scenario | `cancel` |
| request_id | `contact_req_37a9c4d8ab84a5a8696c317b2140c8ae` |
| send status | `CONTACT_REQUEST_STATUS_PENDING` |
| cancel status | `CONTACT_REQUEST_STATUS_CANCELED` |
| receiver pending before cancel | `1` |
| receiver pending after cancel | `0` |
| sender outgoing canceled after cancel | `1` |
| sender contacts | `0` |
| receiver contacts | `0` |
| sender state | `NotFound` |
| receiver state | `NotFound` |
| contacts_outbox | `total=2 / published=2 / pending=0 / dlq=0` |

Kafka 读回事件：

| aggregate_version | event_type | status |
| --- | --- | --- |
| `1` | `contact.request.created.v1` | `PENDING` |
| `2` | `contact.request.canceled.v1` | `CANCELED` |

## 结论

`CancelContactRequest` 的最小闭环成立：

- 只有 sender 视角的 pending 申请被取消。
- receiver 的 incoming pending 列表不再出现该申请。
- sender 可以在 outgoing canceled 列表中查到该申请。
- 不写 `contact_edges`，双方 `ListContacts` 仍为空。
- outbox relay 发布 `contact.request.created.v1` 和 `contact.request.canceled.v1`，没有 pending / DLQ 积压。

这证明 contacts-service 继续保持独立事实源：取消好友申请不写 `conversation_members`，不自动创建或删除会话，也不让 message-service 同步依赖 contacts-service。

## 限制

- 本轮是小规模真实进程 smoke，不是容量压测。
- 本轮只覆盖正常 sender cancel 路径；receiver cancel 权限拒绝、已 accepted/declined 后 cancel conflict、并发 cancel replay 由 PostgreSQL 集成测试覆盖。
- `contact.request.canceled.v1` 当前只作为联系人事实事件；是否通知用户、是否影响发消息权限由后续 notification / policy projection 决定。
