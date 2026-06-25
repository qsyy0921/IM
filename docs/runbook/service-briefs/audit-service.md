# audit-service

状态：product-active / first-stage append、query、admin-event ingestion、export job
metadata 和 proof 已落。

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
- `admin-consumer`：消费公开 `im.admin.events`，映射为低敏 admin audit record；
  append 成功后才提交 Kafka offset。
- action-executor external audit append operator 可在显式 `-execute` 时调用公开
  `AppendAuditRecord` 追加低敏 action / repair 审计；operator 默认只 preflight，
  不直接写 audit-service 私表。
- 最小 gRPC smoke：`docs/runbook/loadtest/audit-service/`。

后续：更多 Kafka ingestion source、持久 ingestion checkpoint / rewind operator、
export worker / manifest、SIEM forwarding、segment sealing、retention cleanup、
provider-grade audit export。
