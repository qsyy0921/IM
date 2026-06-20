# workflow-service

状态：future / SDD v0.1 draft pending。当前不得创建 `services/workflow-service`
目录，直到完成 ADR 或 stage switch。

定位：长事务和审批工作流服务，负责 Agent approval wait、repair approval、
retention、外部系统调用补偿和人工审批状态。

边界：

- 不替代 agent-service proposal、action-executor 执行或 audit-service 归档。
- 业务服务热路径不得同步等待长工作流。
- 高风险动作必须保留 proposal / approval / executor / audit 链。
- Temporal 等工作流引擎只是候选，中间件不写死。

第一切片建议：

- 把现有 operator approval manifest 抽成 workflow request / decision model。
- 先支持 action approval 和 repair approval 两类低敏 workflow。
