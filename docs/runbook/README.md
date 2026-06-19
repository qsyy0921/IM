# NexusIM Runbook Index

这是 runbook 的短路由页。不要把历史结论、压测详情或长设计内容写到这里。

## 当前主线

- 当前主线是 AI 大模型应用底座：group memory、EvidencePack、RAG、summary、Agent、skill registry、MCP/tool gateway、action-executor、approval/audit 和 ai-eval。
- 9 个既有 IM 服务作为基础；默认只处理阻塞 AI 主线的 P0/P1 或用户点名事项。
- sub-agent 只做互不重叠范围，主 agent 合并、检查并关闭。
- 生产级压测、长故障演练和完整生产就绪测试后置。

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
- AI eval harness / 低敏证据 manifest：`ai-eval/README.md`、`distributed-smoke-evidence.json`、`observability-evidence.json`、`capacity-baseline-evidence.json`、`resource-snapshot-evidence.json`
- 复杂度治理：`file-size-hotspots.md`、`file-size-hotspot-baseline.json`
- repair / DLQ operator：`repair-operators.md`、`repair-operators.catalog.json`
- 研究论文分类：`../research/paper-organization.md`
- 历史长文档：`archive/`、`history/`

## 维护规则

- 当前入口和索引必须保持短；每轮只读取本轮必要文档，不默认全文扫长历史。
- 历史事实写入 archive / history / loadtest 报告，不回填到入口文档。
- 每轮结束按本轮风险运行必要检查；生产级测试不是短期默认收口条件。
