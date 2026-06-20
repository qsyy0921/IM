# audit-service

状态：future / SDD v0.1 draft / stage-switch 已通过。当前不得单独创建
`services/audit-service` 目录；下一轮实现切片必须与 service-registry、proto、
migration、runtime 和 observability 同步切换。

定位：统一审计平台，聚合登录审计、安全审计、管理操作审计、策略决策归档、
Agent 动作审计、审计导出和 hash-chain proof。

边界：

- 各服务本地 audit / outbox 仍是事实产生点；audit-service 负责归档、查询和导出。
- 不替代业务服务的事务内 audit，也不直接修业务事实。
- Agent 写动作必须能关联 proposal、approval、executor result 和 policy decision。
- 对外导出必须低敏脱敏，hash-chain 只证明篡改检测，不保存 secret。

下一切片建议：

- 具体边界见 `docs/sdd/audit-service.md`。
- stage-switch 记录见 `docs/runbook/stage-switch/audit-service.md`。
- 先实现 `AppendAuditRecord` / `QueryAuditRecords` / `VerifyAuditProof`。
- 第一版只做 PostgreSQL append、redacted query 和 hash proof；Kafka ingestion、
  export worker、SIEM forwarding 和 retention cleanup 后置。
