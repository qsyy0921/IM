# audit-service

状态：product-active / first-stage append、query、export job metadata 和 proof 已落。

定位：统一审计归档和证明服务，聚合登录、安全、管理操作、策略决策和 Agent
动作审计；对外导出必须低敏脱敏。

边界：
- 各服务本地 audit / outbox 仍是事实产生点；audit-service 负责归档、查询和导出视图。
- 不替代业务服务事务内 audit，不直接修业务事实。
- hash-chain 只证明篡改检测，不保存 secret。

已覆盖：
- `AppendAuditRecord`：PostgreSQL append，按 source event / idempotency 幂等。
- `QueryAuditRecords`：redacted audit record query。
- `CreateAuditExport` / `GetAuditExport`：只创建 / 查询 PENDING job 和低敏
  filter hash / redaction profile / requester refs；不生成 manifest。
- `VerifyAuditProof`：hash-chain proof verification。
- 低敏 `audit.record.appended.v1` outbox。
- 最小 gRPC smoke：`docs/runbook/loadtest/audit-service/`。

后续：Kafka ingestion、export worker / manifest、SIEM forwarding、segment sealing、
retention cleanup、provider-grade audit export。
