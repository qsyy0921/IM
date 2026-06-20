# audit-service

状态：future / SDD v0.1 draft pending。当前不得创建 `services/audit-service`
目录，直到完成 ADR 或 stage switch。

定位：统一审计平台，聚合登录审计、安全审计、管理操作审计、策略决策归档、
Agent 动作审计、审计导出和 hash-chain proof。

边界：

- 各服务本地 audit / outbox 仍是事实产生点；audit-service 负责归档、查询和导出。
- 不替代业务服务的事务内 audit，也不直接修业务事实。
- Agent 写动作必须能关联 proposal、approval、executor result 和 policy decision。
- 对外导出必须低敏脱敏，hash-chain 只证明篡改检测，不保存 secret。

第一切片建议：

- 消费 identity / policy / agent / action audit events。
- 提供 `AppendAuditRecord` / `QueryAuditRecords` 内部 API。
- 增加 hash-chain segment 和 export manifest。
