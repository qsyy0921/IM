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

## 当前服务清单

active 服务清单以 `docs/runbook/service-registry.json` 为准，不能手写漂移。

每个服务的当前状态看 `docs/runbook/service-briefs/<service>.md`。

`future` / `product-active` / `foundation-active` 等 stage 也以 registry 和 service
brief 为准；不要在本文件复制服务名列表。

## 约束

- 服务之间不能 import 对方的 `internal`。
- 跨服务只通过 Protobuf、Kafka schema 或明确 port interface 协作。
- 优先降低耦合、控制代码复杂度；禁止为了模拟分布式而直接读取其它服务内部表或引入不必要的跨服务同步调用。
- `domain` 不依赖 SQL、Kafka、Redis、OpenSearch、Milvus、Temporal 或 Kratos SDK。
- 业务事务不能直接 publish Kafka，只能写 outbox，由 `trigger/outbox` 发布。
