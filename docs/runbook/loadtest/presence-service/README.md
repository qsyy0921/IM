# presence-service smoke

本目录记录 `presence-service` 本地小规模 smoke。原始 summary / log 写入
`H:\NexusIM\loadtest-results`，仓库只保存说明和报告。

## 最小 gRPC smoke

脚本：

```powershell
.\loadtest\presence\run-local-smoke.ps1
```

默认行为：

1. 构建 `services/presence-service/cmd/presence-service` 和 runner。
2. 启动 `NEXUSIM_PRESENCE_SERVICE_MODE=grpc` 真实进程。
3. runner 通过真实 gRPC 调用：
   `UpdatePresence -> replay -> GetPresence -> UpdateTyping`。
4. runner 查询 PostgreSQL，确认：
   - presence update 按 idempotency key 幂等；
   - self query 能看到 device state；
   - unauthorized requester 只能看到 `UNKNOWN` 且无设备详情；
   - `INVISIBLE` 对外可见状态为 `OFFLINE`；
   - typing update 写入 `presence.typing.changed.v1`；
   - `presence_outbox` payload 只保存 hash refs / low-sensitive state，不保存
     raw user、device、session、conversation、manual status 或 secret 字段。

边界：

- 这是 PostgreSQL + gRPC 的最小本地 smoke，不验证 push-gateway session event
  consumer、SubscribePresence、stale scanner、outbox relay 或 Redis 热状态。
- presence 不拥有 delivery inbox / ACK，也不决定会话成员或消息权限事实。

## 已归档报告

- `loadtest-report-20260620-presence-grpc-smoke.md`
