# NexusIM 文档索引

本文只做短入口索引，具体设计以各文档正文为准。

| 目录 / 文档 | 说明 |
| --- | --- |
| `architecture/` | 总架构和架构补充文档。入口是 `architecture/target-architecture.md`，细节分卷在 `target-architecture-*.md`。 |
| `architecture/target-architecture-complete.md` | 完整目标架构蓝图：业务平台、数据平台、AI / Agent 平台、中间件平台、客户端和演进路线。 |
| `architecture/target-architecture-ai.md` | 后续 search / memory / RAG / Agent / Python AI Worker 的目标架构、证据边界、评测门禁和演进顺序。 |
| `platform/` | 平台能力文档。当前包含中间件能力目录、runtime profile 和引入规则。 |
| `sdd/` | 服务级软件设计文档。当前已落地 `message-service`、`conversation-service`、`delivery-service`、`push-gateway`、`identity-service`、`policy-service`、`receipt-service`、`contacts-service` 和第一版 `api-gateway` 的设计入口。 |
| `runbook/` | 本地运行、压测、故障处理和演练说明。压测报告按微服务归档到 `runbook/loadtest/<service>/`。 |
| `interview/project-progress.md` | 面试用项目进度说明：讲述线和后端 / 分布式 / AI 应用后端路线。 |

## 当前阅读路径

Codex 目标框只放根目录 `../prompt.md` 里的短 Prompt，不复制具体长目标。长期完整架构以
`architecture/target-architecture-complete.md` 为准；当前 active slice 以
`runbook/current-goal.md` 为准。短期生产级测试、长周期演练和完整生产就绪验证后置到明确阶段或用户指定任务。

每轮 Codex 工作先读仓库根目录 `prompt.md` 和 `agent.md`，再按任务路由读取必要短文档；不要为了了解全局而全文扫 SDD、archive、history 或 loadtest 长文档。

1. `../prompt.md`：Codex 目标 prompt 的唯一维护源。
2. `../agent.md`：Codex / sub-agent 文档路由、进度维护和并行协作规则。
3. `runbook/current-brief.md`：低 token 当前入口，确认当前阶段和文档路由。
4. `runbook/README.md`：runbook 短路由页。
5. `runbook/development-progress.md`：当前开发进度总览，只写已到哪里。
6. `runbook/remaining-goals.md`：当前还没有完成的工作，只写待办。
7. `runbook/service-briefs/<service>.md`：单服务当前事实。
8. `architecture/target-architecture.md`：总架构短入口，按需跳转到分卷。
9. `architecture/target-architecture-complete.md`：完整目标架构基准，涉及服务拆分、中间件、数据平台、AI / Agent 或客户端长期边界时读取。
10. `platform/middleware-catalog.md`：新增或替换中间件时读取。
11. `sdd/<service>.md`：服务设计，按服务名读取。
12. `runbook/loadtest/<service>/`：smoke / 压测证据，只有任务需要具体证据时读取。

## 文档职责边界

- `development-progress.md` 只写当前进度和已完成阶段，不维护待办。
- `remaining-goals.md` 只写当前未完成工作，新发现待办追加到这里。
- `service-briefs/<service>.md` 只写单服务当前事实和少量下一步。
- `interview/project-progress.md` 是面试讲述稿，不作为工程任务来源。
- `archive/`、`history/`、`loadtest/` 只存历史证据，不回填到入口文档。
- sub-agent 可以并行协助开发或审查，但数量要受控，写入范围要拆开，结果合入后立即关闭。

## 写文档规则

- 架构、SDD、Runbook 均使用中文。
- 总架构不要继续堆服务级细节；服务级细节写入 `sdd/`。
- 契约和 schema 必须落到 `api/`、`schemas/`、`migrations/`，不能只写在文档里。
- 文档中提到的路径必须和仓库真实路径一致。
