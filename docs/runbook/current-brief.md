# NexusIM Current Brief

本文件是每轮低 token 入口，只回答“现在处于什么阶段、下一步去哪里看”。不要在这里维护长历史或完整待办。

## 按需读取

- 具体执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 服务细节：先读 `docs/runbook/service-briefs/README.md`，再读对应服务 brief。
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`docs/runbook/archive/` 或 `docs/runbook/history/`。

## 当前阶段

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端。

已进入真实链路的 9 个后端服务：

```text
api-gateway
identity-service
message-service
conversation-service
delivery-service
push-gateway
receipt-service
contacts-service
policy-service
```

当前重点不是新增服务，而是先收干净这 9 个服务。

```text
安全边界
-> trusted metadata / TLS
-> 观测和故障 smoke
-> repair / DLQ / audit
-> 逐服务 P2 hardening
-> 容量和复杂度治理
```

`search-service`、RAG、summary、agent 和客户端属于后续阶段。

## 文档职责

- 具体执行目标：`docs/runbook/current-goal.md`
- 当前进度总览：`docs/runbook/development-progress.md`
- 当前未完成工作：`docs/runbook/remaining-goals.md`
- 单服务状态：`docs/runbook/service-briefs/<service>.md`
- 开发过程讲述线：`docs/runbook/development-process.md`
- 面试讲述文档：`docs/interview/project-progress.md`

## 硬约束

- 项目统一命名为 NexusIM，不再引入旧项目名。
- 不回滚用户已有修改。
- 不为了“了解项目”全文读取长历史文档。
- 压测原始数据放 `H:\NexusIM\loadtest-results`，E 盘仓库只放报告和文档。
- 新发现的待完成工作写入 `docs/runbook/remaining-goals.md`。
- 每个有意义切片结束后运行 `.\tools\check-local.ps1`，并按风险追加服务级测试、集成测试或 smoke。
- 安全启动门禁索引见 `docs/runbook/security-gate-catalog.json`，分布式 smoke 证据索引见 `docs/runbook/distributed-smoke-evidence.json`，短容量基线证据索引见 `docs/runbook/capacity-baseline-evidence.json`。
