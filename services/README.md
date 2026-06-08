# 服务目录

`services/` 存放 NexusIM 的服务实现。当前只落第一阶段 `message-service`，其他服务在 SDD 和契约冻结前不创建实现目录。

## 六层 DDD 结构

每个服务统一使用：

```text
services/<service-name>/
  cmd/
  internal/
    api/
    app/
    domain/
    infrastructure/
    types/
    trigger/
```

| 层 | 职责 |
| --- | --- |
| `api` | gRPC / HTTP handler 和协议转换 |
| `app` | UseCase、事务编排、port 调用 |
| `domain` | 领域模型、状态流转、不变量 |
| `infrastructure` | PostgreSQL、Kafka、Redis、外部 RPC client 等实现 |
| `types` | 命令、DTO、枚举、错误码、跨层轻量类型 |
| `trigger` | Outbox Relay、Kafka consumer、定时任务、补偿任务 |

## 当前服务

| 服务 | 状态 | 说明 |
| --- | --- | --- |
| `message-service` | 第一阶段实现中 | 只实现普通会话 `SendMessage` 主写链路；热点 sequencer、delivery、push、RAG、Agent 暂不实现。 |

## 约束

- 服务之间不能 import 对方的 `internal`。
- 跨服务只通过 Protobuf、Kafka schema 或明确 port interface 协作。
- `domain` 不依赖 SQL、Kafka、Redis、OpenSearch、Milvus、Temporal 或 Kratos SDK。
- 业务事务不能直接 publish Kafka，只能写 outbox，由 `trigger/outbox` 发布。
