# audit-service

状态：product-active / 第一实现切片已落。已同步 service-registry、proto、
migration、`grpc` runtime、Docker、Prometheus、Grafana 和六层目录。

定位：统一审计平台，聚合登录审计、安全审计、管理操作审计、策略决策归档、
Agent 动作审计、审计导出和 hash-chain proof。

边界：

- 各服务本地 audit / outbox 仍是事实产生点；audit-service 负责归档、查询和导出。
- 不替代业务服务的事务内 audit，也不直接修业务事实。
- Agent 写动作必须能关联 proposal、approval、executor result 和 policy decision。
- 对外导出必须低敏脱敏，hash-chain 只证明篡改检测，不保存 secret。

已落地：

- `AppendAuditRecord`：PostgreSQL append，按 source event / idempotency 幂等。
- `QueryAuditRecords`：redacted audit record query。
- `VerifyAuditProof`：hash-chain proof verification。
- `audit_outbox` 第一版低敏 `audit.record.appended.v1` 事件落库。

下一切片建议：

- 具体边界见 `docs/sdd/audit-service.md`。
- stage-switch 记录见 `docs/runbook/stage-switch/audit-service.md`。
- 补最小 gRPC smoke / report。
- Kafka ingestion、export worker、SIEM forwarding、segment sealing 和
  retention cleanup 后置。
