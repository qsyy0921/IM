# control-plane-service smoke

本目录记录 `control-plane-service` 本地小规模 smoke。原始 summary / log 写入
`H:\NexusIM\loadtest-results`，仓库只保存说明和报告。

## 最小 gRPC smoke

脚本：

```powershell
.\loadtest\controlplane\run-local-smoke.ps1
```

默认行为：

1. 构建 `services/control-plane-service/cmd/control-plane-service` 和 runner。
2. 启动 `NEXUSIM_CONTROL_PLANE_SERVICE_MODE=grpc` 真实进程。
3. runner 通过真实 gRPC 调用：
   `PublishConfigVersion -> replay -> GetConfigSnapshot -> AckAppliedConfigVersion`。
4. runner 查询 PostgreSQL，确认：
   - publish 按 idempotency key 幂等；
   - snapshot 返回相同 version / checksum；
   - ACK 写入 applied state；
   - `control_outbox` 写入 publish / applied 两条低敏事件；
   - outbox payload 不包含 full quota plan、payload_json、secret、token、DSN。

边界：

- 这是 PostgreSQL + gRPC 的最小本地 smoke，不验证 rollback、Kafka relay、
  drift monitor、expiry / cleanup worker 或 api-gateway consumer。
- control-plane 不进入请求热路径；api-gateway 仍负责 quota 执行和本地 snapshot 校验。
