# NexusIM Runbook Index

这是 runbook 的短路由页。不要把历史结论、压测详情或长设计内容写到这里。

## 当前主线

- 当前主线是必要收口 + 转向 AI 大模型应用底座；短期优先关闭阻碍 search / memory / retrieval / evaluation 底座的必要缺口。
- 生产级压测、长周期故障演练和完整生产就绪测试后置到明确阶段或用户指定任务。
- 具体长目标不复制到本页；按入口路由读取 owner 文档。

## 默认入口

- Codex 目标 prompt：`../../prompt.md`
- Agent 进度管理规则：`../../agent.md`
- Codex 具体执行目标：`current-goal.md`
- 当前每轮入口：`current-brief.md`
- 服务短状态索引：`service-briefs/README.md`

## 按需读取

- 单服务状态：`service-briefs/<service>.md`
- 开发进度总览：`development-progress.md`
- 剩余目标 / P2 backlog：`remaining-goals.md`
- 开发过程与阶段顺序：`development-process.md`
- 服务设计：`../sdd/<service>.md`
- smoke / 压测证据：`loadtest/<service>/`
- 本地分布式 / Docker / 观测 / 压测：`distributed-local.md`、`mac-arm64-docker-images.md`、`observability-local.md`、`local-loadtest.md`
- 低敏证据 manifest：`distributed-smoke-evidence.json`、`observability-evidence.json`、`capacity-baseline-evidence.json`、`resource-snapshot-evidence.json`
- 复杂度治理：`file-size-hotspots.md`、`file-size-hotspot-baseline.json`
- repair / DLQ operator：`repair-operators.md`；机器可读 catalog：`repair-operators.catalog.json`
- 研究论文分类：`../research/paper-organization.md`
- 历史长文档：`archive/`、`history/`

## 维护规则

- 当前入口和索引必须保持短。
- 每轮只读取本轮必要文档；不要默认全文扫长 SDD、archive、history 或 loadtest 报告。
- 历史事实写入 archive / history / loadtest 报告，不回填到入口文档。
- sub-agent 可并行协助开发或审查，但要限制数量、拆分写入范围，结果合入后立即关闭。
- 每轮结束按本轮风险运行必要检查；生产级测试不是短期默认收口条件。
