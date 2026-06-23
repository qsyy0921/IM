# NexusIM 目标态技术架构索引

本文只保留短索引和阅读路由，不再承载全部目标态细节。

目标态原则：

- 核心不变量、服务边界、演进原则保留，但不把服务数量、中间件和部署形态写死。
- 服务级设计继续进入 `docs/sdd/`；接口、事件和 migration 继续落到 `api/`、`schemas/`、`migrations/`。
- 短期阶段不以生产级 HA、全量压测、混沌和跨 Region 验证作为继续推进的阻塞；
  当前 9 个 IM 后端服务已作为可运行底座，AI 大模型应用底座已形成 first-stage
  闭环，当前工程切片按 `current-goal.md` 推进客户端平台 MVP foundation，短线优先
  Web / Windows PC 的好友私聊、群聊和消息 first path。
- 新服务或替换中间件必须说明兼容、迁移、回滚和验证证据，并通过 ADR。

## 阅读路由

1. 先读 [foundation](./target-architecture-foundation.md)：架构定位、技术栈口径、总体拓扑、服务边界、控制面。
2. 再读 [timeline](./target-architecture-timeline.md)：消息写入、成员边界、Fanout、数据模型、Kafka、Redis 与长连接。
3. 需要平台和长期演进时读 [platform](./target-architecture-platform.md)：权限、搜索、RAG、Agent、多 Region、审计、观测、ADR、阶段结论。
4. 需要 AI / memory / Agent / Python AI Worker 的后续目标架构时读 [AI](./target-architecture-ai.md)：结构化记忆、画像、EvidencePack、检索流程、Python worker 边界和 AI eval。
5. 需要完整扩展后的业务中台 / 数据中台 / AI 平台 / 中间件平台总览时读 [complete](./target-architecture-complete.md)。

## 文档范围

| 文档 | 范围 |
| --- | --- |
| `target-architecture.md` | 短入口、阅读顺序、硬边界 |
| `target-architecture-foundation.md` | 第 1-4 章和 `Control Plane` |
| `target-architecture-timeline.md` | 第 5-9 章 |
| `target-architecture-platform.md` | 第 10-16 章 |
| `target-architecture-ai.md` | AI / memory / RAG / Agent / Python worker 后续目标架构 |
| `target-architecture-complete.md` | 完善后完整目标架构：业务平台、数据平台、AI / Agent 平台和中间件平台 |

## 当前硬边界

- 总架构只写跨服务不变量、边界和演进准则。
- 服务级实现细节不重新堆回总架构。
- Codex 每轮执行目标不写在架构文档里，统一读取 `../runbook/current-goal.md`。
- 当前 9 个已实现服务作为 IM / AI / 客户端平台可运行基础；生产级测试和 HA
  证据作为后续加固项，不阻塞当前短线切片。
- AI 底座演进顺序为 `search-service -> memory-service -> retrieval-gateway -> rag-service / summary-service -> agent-service -> skill-registry / mcp-gateway -> action-executor -> ai-eval-service`；当前第一组 foundation-active 已形成 EvidencePack、proposal / approval / audit、Python Worker 候选边界，并完成 2026-06-24 live service-stack gate（47/51 passed、0 failed、4 retrieval negative / miss cases skipped），后续默认推进 collaborative-memory 算法/eval 和 retrieval negative adapter。
- 后续服务和中间件都不是写死终局；新增必须符合独立数据模型、独立伸缩、独立故障、独立安全边界之一，或显著降低复杂度，并通过 ADR 和证据演进。
- 中间件作为平台能力管理，登记和引入规则见 `../platform/middleware-catalog.md`；运行编排放 `deploy/`，服务代码只放 adapter。
- 可以使用 multi sub-agent 推进互不重叠的服务、文档和验证任务；主 agent 负责统一方案、合并结果、最终检查和关闭 stale sub-agent。

## 快速定位

- 消息主链路：`target-architecture-timeline.md`
- push / Redis route / resume：`target-architecture-timeline.md`
- 搜索 / RAG / Agent：`target-architecture-platform.md`、`target-architecture-ai.md`
- ADR、阶段路线和拆分准则：`target-architecture-platform.md`
- 技术栈口径和服务边界：`target-architecture-foundation.md`
