# contacts-service ListContactRequests Smoke

日期：2026-06-11

## 目标

验证好友申请列表读路径已经接入真实 contacts-service 进程：

```text
SendContactRequest
-> ListContactRequests(INCOMING, PENDING)
-> RespondContactRequest(ACCEPT)
-> ListContactRequests(INCOMING, ACCEPTED)
-> ListContacts / GetContactState
-> contacts_outbox
-> im.contact.events
```

本轮不是容量压测，只验证产品读路径、分页契约和 outbox/Kafka 小闭环。

## 环境

- commit：`eecdccb`
- `git_dirty=false`
- 原始结果目录：`H:\NexusIM\loadtest-results\contacts-request-list-smoke-clean-20260611-175513`
- scenario：`accept`
- Kafka topic：`im.contact.events.contacts-accept-smoke.20260611-175513`

## 关键结果

```text
success=true
send_contact_request.status=CONTACT_REQUEST_STATUS_PENDING
receiver_incoming_pending_before_respond.request_count=1
receiver_incoming_pending_after_respond.request_count=0
receiver_incoming_terminal_after_respond.status=CONTACT_REQUEST_STATUS_ACCEPTED
receiver_incoming_terminal_after_respond.request_count=1
sender_list.contact_count=1
receiver_list.contact_count=1
contacts_outbox.total=2
contacts_outbox.pending=0
contacts_outbox.published=2
contacts_outbox.dlq=0
```

Kafka 读回：

```text
contact.request.created.v1
contact.request.accepted.v1
```

## 结论

`ListContactRequests` 已证明可支持客户端的“好友申请收件箱 / 已处理申请”最小视图。实现保持在 contacts-service 自己的事实源内：只读 `contact_requests`，不读 conversation/message/delivery 内部表，不自动创建会话，也不让其它微服务同步依赖 contacts-service。

分页规则固定为 `created_at DESC, request_id ASC`，page token 绑定 `tenant_id / user_id / direction / status / page_size`，避免跨用户、跨方向、跨状态或跨 page size 复用造成串页。

## 非目标

- 不验证大规模联系人容量。
- 不实现管理员全站查询。
- 不实现申请取消 / 过期 worker。
- 不把好友申请结果联动创建 direct conversation。
