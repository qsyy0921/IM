# receipt-service Loadtest Reports

本目录保存 `receipt-service` 的小规模验证报告和阶段总入口。

## 当前阶段结论

`receipt-service` 已完成第一条真实小闭环：

```text
im.delivery.events
-> receipt-service delivery-consumer
-> receipt_inbox_projection / message_receipt_states
-> receipt-service GetReceiptState
-> receipt-service MarkRead
-> user_read_cursors / receipt_outbox
-> receipt-service GetReceiptState
```

本阶段重点不是容量，而是证明送达 / 已读回执不直接读取 `delivery-service` 内部表，而是基于 `im.delivery.events` 重建自己的 read model。

## 报告列表

| 报告 | 内容 |
| --- | --- |
| `loadtest-report-20260610-receipt-full-smoke.md` | `im.delivery.events -> receipt projection -> MarkRead -> GetReceiptState` 真实进程 smoke |

## 面试可讲重点

- `receipt-service` 是第三层 IM 产品能力，不是消息事实源；它只消费 `im.delivery.events`，重建送达 / 已读回执 read model。
- `delivery.ack.recorded.v1` 只代表 receiver 设备已收到，不能直接等同已读。
- `MarkRead` 是显式读操作，会受可见最大 seq 和已送达最大 seq 双重约束，不能把未投递消息标已读。
- `GetReceiptState` 支持按 `conversation_seq` 或 `message_id` 查询，当前 smoke 已覆盖两种入口。
- 当前 `receipt_outbox` 只证明 `receipt.message.received.v1` / `receipt.message.read.v1` 已落库为 `PENDING`；`im.receipt.events` 发布链路尚未实现，不能宣称 receipt event 已发布。
- 当前 gRPC 访问控制仍使用本地 `StaticAllowAccess`，真实权限应后续接入 policy / AuthContext。
