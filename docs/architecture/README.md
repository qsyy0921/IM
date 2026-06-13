# 架构文档索引

`docs/architecture/target-architecture.md` 现在是总架构短索引，不再承载全部章节正文。详细内容按主题拆到 foundation、timeline、platform 三个分卷；`add.md` 和 `tadd.md` 仍是辅助文档。

## 文档关系

| 文档 | 作用 |
| --- | --- |
| `target-architecture.md` | 总架构短入口。维护阅读顺序、硬边界和分卷路由。 |
| `target-architecture-foundation.md` | 架构定位、技术栈口径、总体拓扑、服务边界、Control Plane。 |
| `target-architecture-timeline.md` | 消息写入、成员边界、Fanout、数据模型、Kafka、Redis 与长连接。 |
| `target-architecture-platform.md` | 权限、搜索、RAG、Agent、多 Region、审计、观测、ADR、演进结论。 |
| `add.md` | 业务架构补充。描述系统范围、参与方、服务边界、核心业务流和阶段路线。 |
| `tadd.md` | 技术架构补充。描述六层 DDD、工程目录、中间件、本地环境、观测、测试和编码门禁。 |

## 阅读顺序

1. 先读 `target-architecture.md`，确认总架构入口和阅读路由。
2. 需要目标态边界、拓扑和技术栈时，读 `target-architecture-foundation.md`。
3. 需要消息主链路、Kafka、Redis route、push resume 时，读 `target-architecture-timeline.md`。
4. 需要搜索、RAG、Agent、观测、ADR 和演进路线时，读 `target-architecture-platform.md`。
5. 需要工程目录、Docker Compose、观测和编码门禁时，再查 `tadd.md`。
6. 需要业务流和服务边界简版时，再查 `add.md`。

## 变更规则

- 不允许在 `add.md` 或 `tadd.md` 中引入与 `target-architecture.md` 冲突的新决策。
- 服务级细节优先写入 `docs/sdd/`，不要继续塞回总架构。
- 接口、事件和数据库结构必须落到 `api/`、`schemas/`、`migrations/`，不能只停留在文档描述。
