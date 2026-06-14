# NexusIM Local Observability

本文件只记录本地开发 / smoke 用的第一阶段观测入口。它不是生产 Prometheus、Tempo、Jaeger、Loki、Alertmanager 或 SIEM 部署手册。

## Debug Metrics / Prometheus Text

各服务的 `/debug/metrics` 仍是本地 JSON 快照入口。api-gateway 额外提供第一阶段 Prometheus text endpoint：

```text
GET http://<NEXUSIM_API_GATEWAY_DEBUG_ADDR>/metrics
```

`/metrics` 复用 `/debug/metrics` 的低敏 snapshot，当前覆盖 gRPC request / error / latency、facade / legacy descriptor / other request exposure、auth JWK、rate-limit、runtime 和 OTel trace config 聚合指标。labels 只允许 method、code、exposure、backend、key_scope、exporter 等低基数字段；不得输出 token、tenant_id、user_id、device_id、session_id、request_id、trace_id、conversation_id、message_id 或 payload。

这个 endpoint 只用于本地 scrape / dashboard 原型，不代表生产 Prometheus、Alertmanager、指标保留策略或 SLO 告警已经完成。

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

## 最小 OTLP Smoke

用 `policy-service` 作为低依赖 gRPC 服务验证 OTLP trace 链路：

```powershell
.\tools\local-otel-policy-smoke.ps1
```

该脚本会：

```text
1. 如 collector 未运行，启动本地 collector；
2. 临时启用 policy-service 的 OTLP gRPC trace exporter；
3. 复用 policy-service 本地 smoke 触发 CheckMessageAction；
4. 从本次运行后的 collector debug logs 中查找 policy-service span；
5. 将 summary 和 collector log tail 写入 H:\NexusIM\loadtest-results\<run>。
```

默认脚本结束时会关闭由它启动的 collector；如需保留：

```powershell
.\tools\local-otel-policy-smoke.ps1 -KeepCollector
```

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
