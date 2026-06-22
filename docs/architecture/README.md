# 架构文档索引

`docs/architecture/target-architecture.md` 现在是总架构短索引，不再承载全部章节正文。详细内容按主题拆到 foundation、timeline、platform、AI 四个分卷；`add.md` 和 `tadd.md` 是早期历史草案，只用于对照，不作为当前开发路线或技术栈事实来源。

## 文档关系

| 文档 | 作用 |
| --- | --- |
| `target-architecture.md` | 总架构短入口。维护阅读顺序、硬边界和分卷路由。 |
| `target-architecture-foundation.md` | 架构定位、技术栈口径、总体拓扑、服务边界、Control Plane。 |
| `target-architecture-timeline.md` | 消息写入、成员边界、Fanout、数据模型、Kafka、Redis 与长连接。 |
| `target-architecture-platform.md` | 权限、搜索、RAG、Agent、多 Region、审计、观测、ADR、演进结论。 |
| `target-architecture-ai.md` | AI / memory / RAG / Agent 的后续目标架构、Python AI Worker 边界、数据模型、检索流程和评测门禁。 |
| `target-architecture-complete.md` | 完整目标架构蓝图：业务平台、数据平台、AI / Agent 平台、中间件平台、客户端、可靠性和演进路线。 |
| `fail-closed-policy.md` | 当前和未来代码的 fail-closed 治理规则：依赖不确定时拒绝推进事实，在线体验退化必须回到事实源恢复，本地测试 adapter 必须显式隔离。 |
| `adr/` | 已接受的关键架构决策记录。 |
| `add.md` | Legacy 业务架构草案。保留早期系统范围、服务边界和阶段路线，用于历史对照；当前路线以 target 系列和 runbook 为准。 |
| `tadd.md` | Legacy 技术架构草案。保留早期技术栈和工程约束讨论；当前中间件、门禁和服务状态以 target 系列、ADR、SDD 和 runbook 为准。 |

## 阅读顺序

1. 先读 `target-architecture.md`，确认总架构入口和阅读路由。
2. 需要目标态边界、拓扑和技术栈时，读 `target-architecture-foundation.md`。
3. 需要消息主链路、Kafka、Redis route、push resume 时，读 `target-architecture-timeline.md`。
4. 需要搜索、RAG、Agent、观测、ADR 和演进路线时，读 `target-architecture-platform.md`。
5. 需要 AI / memory / Agent / Python AI Worker 的详细目标架构时，读 `target-architecture-ai.md`。
6. 需要重新理解整个系统完善后的端到端架构时，读 `target-architecture-complete.md`。
7. 需要判断备用路径、local-test adapter、compat window 或显式恢复是否合理时，读 `fail-closed-policy.md`。
8. 需要早期设计对照或演进背景时，再查 `add.md` / `tadd.md`；不要从这两份文档推导当前开发目标。

## 变更规则

- 不允许在 `add.md` 或 `tadd.md` 中引入与 `target-architecture.md`、ADR、SDD 或 runbook 冲突的新决策。
- 完整扩展后的新服务、中间件、客户端和 AI / Agent 能力，优先对齐
  `target-architecture-complete.md`；短索引只负责路由，不重复维护长内容。
- 当前和未来设计遵守 `fail-closed-policy.md`：不再把隐藏备用路径作为正向业务设计目标。
- 服务级细节优先写入 `docs/sdd/`，不要继续塞回总架构。
- 接口、事件和数据库结构必须落到 `api/`、`schemas/`、`migrations/`，不能只停留在文档描述。
