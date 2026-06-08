# 架构文档索引

`docs/architecture/target-architecture.md` 是 NexusIM 总架构的唯一主文档。`add.md` 和 `tadd.md` 是辅助文档，只用于把业务架构和技术落地规则拆开阅读，不能覆盖主文档中的冻结决策。

## 文档关系

| 文档 | 作用 |
| --- | --- |
| `target-architecture.md` | 目标态技术架构冻结稿。维护服务边界、核心不变量、技术栈、事件平台、控制面、容灾、安全、RAG/Agent 治理和第一阶段门禁。 |
| `add.md` | 业务架构补充。描述系统范围、参与方、服务边界、核心业务流和阶段路线。 |
| `tadd.md` | 技术架构补充。描述六层 DDD、工程目录、中间件、本地环境、观测、测试和编码门禁。 |

## 阅读顺序

1. 先读 `target-architecture.md`，确认目标态和不可退让项。
2. 再读 `docs/sdd/message-service.md`，确认第一条可编码切片。
3. 需要工程目录、Docker Compose、观测和编码门禁时，再查 `tadd.md`。
4. 需要业务流和服务边界简版时，再查 `add.md`。

## 变更规则

- 不允许在 `add.md` 或 `tadd.md` 中引入与 `target-architecture.md` 冲突的新决策。
- 服务级细节优先写入 `docs/sdd/`，不要继续塞回总架构。
- 接口、事件和数据库结构必须落到 `api/`、`schemas/`、`migrations/`，不能只停留在文档描述。
