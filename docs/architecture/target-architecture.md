# NexusIM 目标态技术架构索引

本文只保留短索引和阅读路由，不再承载全部目标态细节。

目标态原则：

- 核心不变量、服务边界、演进原则保留，但不把服务数量、中间件和部署形态写死。
- 服务级设计继续进入 `docs/sdd/`；接口、事件和 migration 继续落到 `api/`、`schemas/`、`migrations/`。
- 新服务或替换中间件必须说明兼容、迁移、回滚和验证证据，并通过 ADR。

## 阅读路由

1. 先读 [foundation](./target-architecture-foundation.md)：架构定位、技术栈口径、总体拓扑、服务边界、控制面。
2. 再读 [timeline](./target-architecture-timeline.md)：消息写入、成员边界、Fanout、数据模型、Kafka、Redis 与长连接。
3. 需要平台和长期演进时读 [platform](./target-architecture-platform.md)：权限、搜索、RAG、Agent、多 Region、审计、观测、ADR、阶段结论。
4. 需要 AI / memory / Agent 的后续目标架构时读 [AI](./target-architecture-ai.md)：结构化记忆、画像、EvidencePack、检索流程和 AI eval。

## 文档范围

| 文档 | 范围 |
| --- | --- |
| `target-architecture.md` | 短入口、阅读顺序、硬边界 |
| `target-architecture-foundation.md` | 第 1-4 章和 `Control Plane` |
| `target-architecture-timeline.md` | 第 5-9 章 |
| `target-architecture-platform.md` | 第 10-16 章 |
| `target-architecture-ai.md` | AI / memory / RAG / Agent 后续目标架构 |

## 当前硬边界

- 总架构只写跨服务不变量、边界和演进准则。
- 服务级实现细节不重新堆回总架构。
- 当前 9 个已实现服务优先继续清理和 hardening，再进入后续新服务。
- 后续服务和中间件都不是写死终局，只能按 ADR 和证据演进。

## 快速定位

- 消息主链路：`target-architecture-timeline.md`
- push / Redis route / resume：`target-architecture-timeline.md`
- 搜索 / RAG / Agent：`target-architecture-platform.md`、`target-architecture-ai.md`
- ADR、阶段路线和拆分准则：`target-architecture-platform.md`
- 技术栈口径和服务边界：`target-architecture-foundation.md`
