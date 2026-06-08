# <service-name> SDD v1.0

状态：Draft / Frozen

本文是单个服务的软件设计文档。SDD 必须能直接指导代码、migration、契约、测试和 Runbook。

## 1. 服务定位

说明服务的业务定位、事实源归属、职责和非职责。

职责：

- ...

不负责：

- ...

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 |  |  |
| 同步依赖 |  |  |
| 异步下游 |  |  |
| 事实源 |  |  |

## 3. 六层 DDD 包结构

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

| 层 | 本服务内容 |
| --- | --- |
| `api` |  |
| `app` |  |
| `domain` |  |
| `infrastructure` |  |
| `types` |  |
| `trigger` |  |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
|  |  |  |

## 5. 同步 API 契约

说明 gRPC / HTTP API。

```text
rpc Example(ExampleRequest) returns (ExampleResponse)
```

请求字段：

```text
...
```

响应字段：

```text
...
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
|  |  |  |

## 6. 异步事件契约

| 事件 | Topic | 分区键 | 下游 |
| --- | --- | --- | --- |
|  |  |  |  |

事件必须说明：

- event_id；
- event_type；
- event_version；
- partition_key；
- trace_id；
- payload；
- 幂等键；
- retry / DLQ 策略。

## 7. 数据库设计

列出本服务拥有的表。禁止设计会修改其他服务事实源的表。

```sql
CREATE TABLE example (
    id TEXT PRIMARY KEY
);
```

## 8. 核心流程

用文本或 Mermaid 描述主流程。

```text
request
-> app use case
-> domain rule
-> db transaction
-> outbox/event
-> response
```

## 9. 一致性和事务

强一致边界：

```text
...
```

最终一致边界：

```text
...
```

## 10. 幂等、重试和补偿

| 场景 | 幂等键 | 重试策略 | 补偿 |
| --- | --- | --- | --- |
|  |  |  |  |

## 11. 权限和安全

说明：

- auth context；
- tenant/user/device 派生；
- 权限检查；
- fail closed；
- 审计字段。

## 12. SLO 和指标

| 指标 | 目标 |
| --- | --- |
| p95 |  |
| error rate |  |

必须打点：

```text
...
```

## 13. 测试方案

| 测试 | 目标 |
| --- | --- |
| unit |  |
| integration |  |
| contract |  |
| loadtest |  |

## 14. Runbook

包括：

- 启动方式；
- 健康检查；
- 常见故障；
- DLQ/replay；
- migration；
- rollback；
- 压测命令。

## 15. 验收标准

服务进入编码或发布前必须满足：

- SDD Frozen；
- API 契约存在；
- Kafka schema 存在；
- migration 存在；
- 本地集成测试可运行；
- 关键指标可观测；
- Runbook 可执行。
