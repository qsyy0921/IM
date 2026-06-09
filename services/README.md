# 服务目录

`services/` 存放 NexusIM 的服务实现。每个微服务独立使用六层 DDD。

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
| `message-service` | 第一阶段主链路已落地 | 普通会话 `SendMessage -> PostgreSQL transaction -> outbox -> Kafka` 已实现；热点 sequencer、RAG、Agent 暂不实现。 |
| `conversation-service` | 最小 read/write path 已落地 | 提供 `GetSendContext`，并已实现成员变更 `CreateMemberChange / GetMemberChange`、成员边界事件和 saga progress worker。 |
| `delivery-service` | 最小投递链路已落地 | 消费 conversation timeline，维护 `user_inbox`、`AckDelivery` cursor、`delivery_outbox` 和 `im.delivery.events`。 |
| `push-gateway` | 最小在线通知 / 分布式 route 已落地 | 消费 `im.delivery.events`，通过 WebSocket 发送轻量 notify，并通过 Redis route / resume 支持跨实例在线唤醒。 |
| `receipt-service` | SDD v0.1 Draft | 下一步第三层产品能力：基于 `im.delivery.events` 建送达 / 已读回执 read model，先冻结 SDD 再落代码。 |

## 约束

- 服务之间不能 import 对方的 `internal`。
- 跨服务只通过 Protobuf、Kafka schema 或明确 port interface 协作。
- `domain` 不依赖 SQL、Kafka、Redis、OpenSearch、Milvus、Temporal 或 Kratos SDK。
- 业务事务不能直接 publish Kafka，只能写 outbox，由 `trigger/outbox` 发布。
