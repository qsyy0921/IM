# NexusIM

NexusIM 是面向企业协同的 IM + 智能协作平台。当前仓库处于第一阶段工程落地：优先实现 `message-service` 的普通会话 `SendMessage` 主写链路，并验证 PostgreSQL 本地事务、message outbox 和 Kafka 事件发布。

## 目录结构

| 目录 | 作用 |
| --- | --- |
| `api/` | 同步接口契约。`api/proto/` 存放 gRPC Protobuf；`api/openapi/` 和 `api/asyncapi/` 后续分别承接外部 HTTP 与异步接口说明。 |
| `schemas/` | 异步事件契约。`schemas/kafka/` 存放 Kafka topic 的 Protobuf schema。 |
| `services/` | 服务实现。每个服务统一使用 `api / app / domain / infrastructure / types / trigger` 六层 DDD 目录。 |
| `migrations/` | 数据库 migration，按数据库和服务归档。 |
| `deploy/` | 本地和部署基础设施。当前 `deploy/local/docker-compose.yml` 用于启动 PostgreSQL、Kafka、Schema Registry、Redis。 |
| `loadtest/` | 压测脚本、压测结果和本地双机压测入口。脚本必须支持参数化目标地址、并发和持续时间。 |
| `docs/` | 中文文档。包含目标态架构、服务级 SDD、Runbook 和文档索引。 |
| `tools/` | 本地辅助脚本，例如 proto 生成和本地依赖启动/关闭。 |

## 关键文档

| 文档 | 作用 |
| --- | --- |
| `docs/architecture/target-architecture.md` | NexusIM 目标态技术架构冻结稿，是总架构的唯一主文档。 |
| `docs/architecture/add.md` | 业务架构补充文档，描述系统范围、服务边界和核心业务流。 |
| `docs/architecture/tadd.md` | 技术架构补充文档，描述技术栈、工程目录、中间件、观测和编码门禁。 |
| `docs/sdd/README.md` | 服务级 SDD 索引和编码门禁。 |
| `docs/sdd/message-service.md` | 第一份服务级 SDD，指导 `message-service` 第一条代码切片。 |
| `docs/runbook/local-loadtest.md` | 本地和双机压测 Runbook。 |

## 当前实现范围

第一条代码切片只实现：

```text
message-service SendMessage
-> PostgreSQL local transaction
-> conversation_seq
-> message_log
-> conversation_timeline_events
-> message_outbox
-> outbox relay
-> Kafka publish path
```

第一阶段暂不实现：

```text
EditMessage / RevokeMessage / DeleteMessage 业务实现
热点 sequencer 生产逻辑
delivery-service
push-gateway
RAG
Agent workflow
```

## 六层 DDD 约束

服务目录统一为：

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

依赖方向：

```text
api -> app
api -> types
trigger -> app
trigger -> types
app -> domain
app -> infrastructure
app -> types
domain -> types
infrastructure -> types
```

禁止方向：

```text
domain -> infrastructure
domain -> api
domain -> trigger
infrastructure -> api
infrastructure -> trigger
types -> app/domain/infrastructure/api/trigger
```

说明：

- `api` 只做 gRPC/HTTP 适配和 request/response 转换；
- `app` 负责编排 use case、事务和 port；
- `domain` 只表达领域规则，不依赖 SQL、Kafka、Redis 或外部 SDK；
- `infrastructure` 实现 PostgreSQL、Kafka、Redis、外部 RPC client 等能力；
- `types` 放稳定类型、命令、枚举、错误码和跨层轻量 DTO；
- `trigger` 放 Outbox Relay、Kafka consumer、定时巡检和补偿任务。

## 常用命令

生成 Protobuf 代码：

```powershell
make proto
```

启动本地依赖：

```powershell
make local-up
```

停止本地依赖：

```powershell
make local-down
```

查看本地依赖日志：

```powershell
make local-logs
```

也可以直接使用 `tools/` 下的 PowerShell 脚本。

## 开发顺序

1. 先落契约：Proto、Kafka schema、PostgreSQL migration。
2. 只实现普通会话 `LOCAL_ROW_LOCK` 下的 `SendMessage`。
3. 外部依赖全部通过 port 隔离：policy、conversation、sequencer、event publisher。
4. 数据库事务只覆盖本地事实源：`conversation_seq + message_log + conversation_timeline_events + message_outbox`。
5. Kafka 事件只能由 outbox relay 发布，业务事务不能直接 publish Kafka。
