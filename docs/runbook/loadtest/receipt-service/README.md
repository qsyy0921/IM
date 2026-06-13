# receipt-service Loadtest Reports

本目录保存 `receipt-service` 的小规模验证报告和阶段总入口。

## 当前阶段结论

`receipt-service` 已完成第一条真实小闭环、最小 outbox 发布链路、最小会话列表 / 未读数 read model、未读列表过滤，以及当前用户侧会话归档 / 置顶 / 静音偏好：

```text
im.delivery.events
-> receipt-service delivery-consumer
-> receipt_inbox_projection / message_receipt_states
-> receipt-service GetReceiptState
-> receipt-service MarkRead
-> user_read_cursors / receipt_outbox
-> receipt-service GetReceiptState
-> receipt-service outbox-relay
-> Kafka im.receipt.events
-> user_conversation_summaries / ListConversations
-> ListConversations(unread_only=true)
-> ArchiveConversation / include_archived filtering
-> PinConversation / pinned-first sorting
-> MuteConversation / muted flag
```

本阶段重点不是容量，而是证明送达 / 已读回执不直接读取 `delivery-service` 内部表，而是基于 `im.delivery.events` 重建自己的 read model。

2026-06-13 补充：`-VerifiedAuthMetadata` 真实进程 smoke 已通过，验证 conversation / message / delivery / receipt 四个 user-facing gRPC server 在 metadata auth 模式下完成投递、回执、会话列表、未读、归档、置顶和静音链路。

## 报告列表

| 报告 | 内容 |
| --- | --- |
| `loadtest-report-20260610-receipt-full-smoke.md` | `im.delivery.events -> receipt projection -> MarkRead -> GetReceiptState` 真实进程 smoke |
| `loadtest-report-20260610-receipt-outbox-smoke.md` | `receipt_outbox -> im.receipt.events` 真实进程 smoke，读回 `received/read` 两条 Kafka event |
| `loadtest-report-20260610-conversation-list-smoke.md` | `receipt projection -> user_conversation_summaries -> ListConversations` 真实进程 smoke，验证未读 `1 -> 0` |
| `loadtest-report-20260610-receipt-archive-smoke.md` | `ArchiveConversation` 真实进程 smoke，验证默认列表隐藏、`include_archived` 可见、归档期间新消息不自动取消归档、取消归档后恢复 |
| `loadtest-report-20260610-receipt-pin-smoke.md` | `PinConversation` 真实进程 smoke，验证当前用户置顶 / 取消置顶标志；PostgreSQL integration 覆盖 pinned-first 排序和 cursor |
| `loadtest-report-20260611-receipt-mute-smoke.md` | `MuteConversation` 真实进程 smoke，验证当前用户静音 / 取消静音标志；静音不改变 unread、read cursor、delivery、push 或消息事实 |
| `loadtest-report-20260611-receipt-unread-filter-smoke.md` | `ListConversations(unread_only=true)` 真实进程 smoke，验证投递后未读列表可见、`MarkRead` 后未读列表为空 |
| `loadtest-report-20260613-receipt-verified-metadata-smoke.md` | `-VerifiedAuthMetadata` 真实进程 smoke，验证 metadata auth 下的 delivery / receipt / list / preference 链路 |

## TLS / mTLS smoke 参数

`loadtest/receipt` 和 `loadtest/receipt/run-local-smoke.ps1` 默认仍使用 plaintext gRPC。若本地已通过对应 `NEXUSIM_*_GRPC_TLS_*` 配置开启 conversation / message / delivery / receipt gRPC server TLS，可给 runner 增加对应 client 参数：

```powershell
.\loadtest\receipt\run-local-smoke.ps1 `
  -ConversationTlsCaFile .\certs\ca.pem `
  -ConversationTlsServerName conversation-service.nexusim.local `
  -ConversationTlsClientCertFile .\certs\loadtest-client.crt `
  -ConversationTlsClientKeyFile .\certs\loadtest-client.key `
  -MessageTlsCaFile .\certs\ca.pem `
  -MessageTlsServerName message-service.nexusim.local `
  -MessageTlsClientCertFile .\certs\loadtest-client.crt `
  -MessageTlsClientKeyFile .\certs\loadtest-client.key `
  -DeliveryTlsCaFile .\certs\ca.pem `
  -DeliveryTlsServerName delivery-service.nexusim.local `
  -DeliveryTlsClientCertFile .\certs\loadtest-client.crt `
  -DeliveryTlsClientKeyFile .\certs\loadtest-client.key `
  -ReceiptTlsCaFile .\certs\ca.pem `
  -ReceiptTlsServerName receipt-service.nexusim.local `
  -ReceiptTlsClientCertFile .\certs\loadtest-client.crt `
  -ReceiptTlsClientKeyFile .\certs\loadtest-client.key
```

配置任一 `*-tls-*` 参数后必须提供对应 CA file，client cert/key 必须成对配置。该能力只验证 smoke runner 到 conversation / message / delivery / receipt gRPC server 的静态 TLS / mTLS 连接；证书签发、轮换、分发和全服务 mTLS rollout 仍是后续项。

如果 conversation / message / delivery / receipt 四个 user-facing gRPC server 以 metadata auth 模式启动，`loadtest/receipt` 可用以下开关发送 gateway verified identity metadata：

```text
--verified-auth-metadata
NEXUSIM_RECEIPT_LOADTEST_VERIFIED_AUTH_METADATA=true
```

对应 `run-local-smoke.ps1` 支持 `-VerifiedAuthMetadata`，会把本地 conversation / message / delivery / receipt gRPC 进程切到 metadata auth，并让 runner 发送 metadata。默认仍是 body auth 兼容历史 smoke；这不是完整 API gateway。

## 面试可讲重点

- `receipt-service` 是第三层 IM 产品能力，不是消息事实源；它只消费 `im.delivery.events`，重建送达 / 已读回执 read model。
- `delivery.ack.recorded.v1` 只代表 receiver 设备已收到，不能直接等同已读。
- `MarkRead` 是显式读操作，会受可见最大 seq 和已送达最大 seq 双重约束，不能把未投递消息标已读。
- `GetReceiptState` 支持按 `conversation_seq` 或 `message_id` 查询，当前 smoke 已覆盖两种入口。
- `receipt_outbox` 已通过 relay 发布 `receipt.message.received.v1` / `receipt.message.read.v1` 到 `im.receipt.events`；当前还没有下游真实消费者。
- receipt outbox 的 `aggregate_version` 是 cursor seq，不是 conversation 全局顺序轴，所以 relay 不用低版本 PENDING/DLQ 阻塞同会话更高版本回执事件，避免某个用户回执阻塞其它用户。
- 会话列表 / 未读数放在 `receipt-service` 内扩展，不新增 `conversation-list-service`，降低服务间耦合和部署复杂度。
- `ListConversations` 的 unread 由 `receipt_inbox_projection` 中 `source_event_type=message.persisted.v1` 的可见消息行数减去 read cursor 得出，不把 conversation seq 差值当成未读数，也不把 edit/revoke/delete tombstone 当新未读消息。
- `ListConversations(unread_only=true)` 是过滤条件，不是新的排序模式；它在 SQL 层使用 `unread_count > 0`，cursor 绑定 `unread_only`，避免普通列表 cursor 和未读列表 cursor 混用造成漏页。
- `ListConversations.last_source_event_type` 会返回最后一次可见变化的事件类型，客户端可据此刷新会话列表 UI；消息正文和 tombstone 详情仍以 `PullInbox` 为准。
- `ArchiveConversation` 是当前用户的列表过滤偏好；它不删除消息、不清未读、不停止 delivery/push/receipt projection。新消息不会自动取消归档，客户端需要通过 `include_archived=true` 或用户手动取消归档查看。
- `PinConversation` 也是当前用户的列表偏好；默认列表按 pinned-first 再按 `updated_at desc` 排序，显式 `UPDATED_AT_DESC` 仍可获得纯更新时间排序。pin 不进入 Kafka、不影响 unread 或通知。
- `MuteConversation` 是当前用户的列表 / 通知策略偏好字段；当前阶段只在 `ListConversations` 返回 `muted`，不发布 Kafka、不修改 unread、read cursor、delivery、push 或消息事实。后续如果做真正推送静音，应在 push policy / consumer 侧读取偏好或投影，不应改写 durable delivery 事实。
- 当前 gRPC 访问控制仍使用本地 `StaticAllowAccess`，真实权限应后续接入 policy / AuthContext。
