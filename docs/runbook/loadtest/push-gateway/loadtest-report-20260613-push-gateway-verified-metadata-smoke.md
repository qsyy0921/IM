# push-gateway verified metadata auth smoke

日期：2026-06-13

## 结论

通过。

本轮验证 `loadtest/pushgateway` 的 `-VerifiedAuthMetadata` 链路可以在真实本地进程中跑通：

```text
CreateMemberChange(metadata auth)
-> SendMessage(metadata auth)
-> delivery.notify
-> PullInbox(metadata auth)
-> delivery.ack WebSocket frame
-> push-gateway AckDelivery(metadata auth forwarding)
-> delivery.ack.ok
```

这证明 push-gateway 在 ACK 转发时能把 WebSocket auth 派生出的身份写入 delivery-service gRPC metadata，并且 conversation / message / delivery 三个 user-facing RPC 可以在 metadata auth 模式下完成最小 full smoke。

这不是完整 API gateway、真实生产鉴权、证书治理或全服务 mTLS 结论。

## 命令

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -VerifiedAuthMetadata `
  -RunName push-gateway-verified-metadata-smoke-20260613-183530
```

## 原始结果

```text
H:\NexusIM\loadtest-results\push-gateway-verified-metadata-smoke-20260613-183530\pushgateway-summary.json
```

关键元数据：

```text
commit=72d8a1b
git_dirty=false
scenario=full
verified_auth_metadata=true
route_backend=memory
push_auth_mode=mock
```

## 关键证据

```text
server.hello session_id=sess_ea336335dac05e5e26ecd81720135b75
member_join boundary_seq=1 member_version=2 permission_version=2
SendMessage message_id=msg_0958908e-cb25-4716-a6ec-3d3f699fb97a conversation_seq=2
delivery.notify source_event_type=message.persisted.v1 conversation_seq=2
PullInbox item_count=1 max_seq=2
delivery.ack.ok last_received_seq=2
cursor_last_received_seq=2
delivery_outbox total=2 published=2 pending=0 dlq=0
```

`PullInbox` 返回的 durable item 与 WebSocket notify 对齐：

```text
event_type=message.persisted.v1
message_id=msg_0958908e-cb25-4716-a6ec-3d3f699fb97a
conversation_seq=2
sender_id=owner-1
```

## Preflight Isolation

首次尝试时，delivery timeline consumer 在本次业务事件前消费到本地旧集成测试残留的 `message.edited.v1`，因找不到原始 projected message 而 fail-closed。原因是本地 `message_outbox` 中存在旧测试 tenant 的未发布残留，push smoke 脚本先启动 message relay，导致旧行被发布到本次临时 timeline topic。

已在 `loadtest/pushgateway/run-local-smoke.ps1` 增加本地 preflight cleanup：启动 relay 前只清理测试 / smoke tenant 前缀下 `status <> 'PUBLISHED'` 的 `message_outbox` 残留，避免旧测试事件污染当前临时 topic。修复后以 clean commit 重新跑通本报告中的 smoke。

## 边界

- 本轮只验证单实例 `all` mode、单用户、单设备、单条消息。
- push-gateway WebSocket auth 仍使用本地 mock/query identity。
- `-VerifiedAuthMetadata` 验证的是 gateway verified metadata 接口形态，不代表完整 API gateway。
- 本轮不启用 TLS / mTLS，不代表证书签发、轮换、分发或动态服务身份治理。
- 在线通知仍只是唤醒；客户端展示和 ACK 事实仍以 delivery-service `PullInbox / AckDelivery` 为准。
