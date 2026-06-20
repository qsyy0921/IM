# audit-service smoke

本目录记录 `audit-service` 本地小规模 smoke。原始 summary / log 写入
`H:\NexusIM\loadtest-results`，仓库只保存说明和报告。

## 最小 gRPC smoke

脚本：

```powershell
.\loadtest\audit\run-local-smoke.ps1
```

默认行为：

1. 构建 `services/audit-service/cmd/audit-service` 和 `loadtest/audit`。
2. 启动 `NEXUSIM_AUDIT_SERVICE_MODE=grpc` 真实进程。
3. runner 通过真实 gRPC 调用：
   `AppendAuditRecord -> replay -> QueryAuditRecords -> VerifyAuditProof`。
4. runner 查询 PostgreSQL，确认：
   - append 按 source event / idempotency key 幂等；
   - query 返回两条低敏 audit record；
   - second record 的 proof 指向 first record hash；
   - `audit_outbox` 写入两条 `audit.record.appended.v1`；
   - outbox payload 不包含 raw prompt、EvidencePack、message body、password、
     provider body、TOTP / recovery code 或本轮 session marker。

边界：

- 这是 PostgreSQL + gRPC 的最小本地 smoke，不验证 Kafka ingestion、export worker、
  SIEM forwarding、segment sealing 或 retention cleanup。
- hash-chain proof 只证明 audit-service 已接收记录后的篡改检测，不证明上游业务事实完整或真实。
- raw summary / logs 写入 `H:\NexusIM\loadtest-results`。
