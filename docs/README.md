# NexusIM 文档索引

本文只做短入口索引，具体设计以各文档正文为准。

| 目录 / 文档 | 说明 |
| --- | --- |
| `architecture/` | 总架构和架构补充文档。入口是 `architecture/target-architecture.md`，细节分卷在 `target-architecture-*.md`。 |
| `sdd/` | 服务级软件设计文档。当前已落地 `message-service`、`conversation-service`、`delivery-service`、`push-gateway`、`identity-service`、`policy-service`、`receipt-service`、`contacts-service` 和第一版 `api-gateway` 的设计入口。 |
| `runbook/` | 本地运行、压测、故障处理和演练说明。压测报告按微服务归档到 `runbook/loadtest/<service>/`。 |
| `interview/project-progress.md` | 面试用项目进度说明：讲述线和后端 / 分布式 / AI 应用后端路线。 |

## 当前阅读路径

Codex 目标框只放根目录 `../prompt.md` 里的短 Prompt。每轮 Codex 工作先读仓库根目录 `prompt.md`，再按它的路由读取必要短文档。

1. `../prompt.md`：Codex 目标 prompt 的唯一维护源。
2. `runbook/current-brief.md`：低 token 当前入口，确认当前阶段和文档路由。
3. `runbook/README.md`：runbook 短路由页。
4. `runbook/development-progress.md`：当前开发进度总览，只写已到哪里。
5. `runbook/remaining-goals.md`：当前还没有完成的工作，只写待办。
6. `runbook/service-briefs/<service>.md`：单服务当前事实。
7. `architecture/target-architecture.md`：总架构短入口，按需跳转到分卷。
8. `sdd/<service>.md`：服务设计，按服务名读取。
9. `runbook/loadtest/<service>/`：smoke / 压测证据。

## 文档职责边界

- `development-progress.md` 只写当前进度和已完成阶段，不维护待办。
- `remaining-goals.md` 只写当前未完成工作，新发现待办追加到这里。
- `service-briefs/<service>.md` 只写单服务当前事实和少量下一步。
- `interview/project-progress.md` 是面试讲述稿，不作为工程任务来源。
- `archive/`、`history/`、`loadtest/` 只存历史证据，不回填到入口文档。

## 写文档规则

- 架构、SDD、Runbook 均使用中文。
- 总架构不要继续堆服务级细节；服务级细节写入 `sdd/`。
- 契约和 schema 必须落到 `api/`、`schemas/`、`migrations/`，不能只写在文档里。
- 文档中提到的路径必须和仓库真实路径一致。
