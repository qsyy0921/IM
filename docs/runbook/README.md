# NexusIM Runbook Index

短路由页。不要把历史结论、压测详情或长设计写到这里。

## 当前主线

- 当前 active slice 见 `current-goal.md`；本轮默认只读必要文档。
- 长期完整架构见 `../architecture/target-architecture-complete.md`，不写死服务、
  中间件或部署形态。
- AI / Agent 后续主线包括 group memory、EvidencePack、RAG、summary、Agent、MCP/tool、approval/audit 和 ai-eval。
- 9 个既有 IM 服务作为基础；默认只处理阻塞当前主线的 P0/P1 或用户点名事项。
- 生产级压测、长故障演练和完整生产就绪测试后置。

## 默认入口

- Codex 文档路由入口：`../../prompt.md`
- Agent 进度管理规则：`../../agent.md`
- 具体执行目标：`current-goal.md`
- 每轮短入口：`current-brief.md`
- 剩余工作：`remaining-goals.md`
- 服务短状态索引：`service-briefs/README.md`

## 按需读取

- 客户端平台：`client-platform.md`、`../sdd/client-platform.md`
- 开发进度总览：`development-progress.md`
- 完整目标架构：`../architecture/target-architecture-complete.md`
- 中间件目录：`../platform/middleware-catalog.md`
- 服务设计：`../sdd/<service>.md`
- smoke / 压测证据：`loadtest/<service>/`
- 本地分布式 / Docker / 观测 / 压测：`distributed-local.md`、`mac-arm64-docker-images.md`、`observability-local.md`、`local-loadtest.md`
- AI eval / evidence manifests：`ai-eval/README.md`、`*-evidence.json`
- 复杂度治理：`file-size-hotspots.md`、`file-size-hotspot-baseline.json`
- repair / DLQ operator：`repair-operators.md`、`repair-operators.catalog.json`
- 历史长文档：`archive/`、`history/`

## 维护规则

- 入口和索引保持短；每轮只读取本轮必要文档。
- 历史事实写入 archive / history / loadtest 报告，不回填到入口文档。
