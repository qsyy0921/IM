# NexusIM Local Observability

本文件只记录本地开发 / smoke 用的第一阶段观测入口。它不是生产 Prometheus、Tempo、Jaeger、Loki、Alertmanager 或 SIEM 部署手册。

## OpenTelemetry Collector

默认本地基础设施不会启动 collector。需要验证 OTLP trace exporter 时单独启动：

```powershell
.\tools\local-up-otel.ps1
```

停止：

```powershell
.\tools\local-down-otel.ps1
```

本地端点：

```text
OTLP gRPC: 127.0.0.1:4317
OTLP HTTP: http://127.0.0.1:4318
health:    http://127.0.0.1:13133
```

collector 配置位于 `deploy/local/otel-collector.yml`，当前只把 traces / metrics 输出到 collector debug exporter，方便本地看日志确认 span 到达；不做持久化、不做采样治理、不做告警。

## 服务端启用方式

各服务默认关闭 trace。需要验证某个进程时，只打开该服务自己的 env 前缀：

```powershell
$env:NEXUSIM_MESSAGE_OTEL_TRACES_ENABLED = "true"
$env:NEXUSIM_MESSAGE_OTEL_TRACES_EXPORTER = "otlp-grpc"
$env:NEXUSIM_MESSAGE_OTEL_TRACES_OTLP_ENDPOINT = "127.0.0.1:4317"
$env:NEXUSIM_MESSAGE_OTEL_TRACES_OTLP_INSECURE = "true"
```

已支持第一阶段 gRPC server span 的前缀：

```text
NEXUSIM_CONTACTS_OTEL_*
NEXUSIM_IDENTITY_OTEL_*
NEXUSIM_MESSAGE_OTEL_*
NEXUSIM_CONVERSATION_OTEL_*
NEXUSIM_DELIVERY_OTEL_*
NEXUSIM_RECEIPT_OTEL_*
NEXUSIM_POLICY_OTEL_*
```

api-gateway 还支持入口 server span 和下游 gRPC client span：

```text
NEXUSIM_API_GATEWAY_OTEL_*
```

## 边界

- span 只能记录低敏 transport / correlation 字段。
- 不记录 token、tenant/user/device/session id、conversation/message id、payload、rule 参数、SQL 错误或 provider body。
- 本地 collector 只证明 OTLP 链路可接收，不证明生产级观测。
