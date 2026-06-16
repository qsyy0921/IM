# api-gateway Loadtest Reports

本目录保存 api-gateway 入口能力的小规模验证报告。api-gateway 只做 user-facing 入口鉴权、身份覆盖、trusted metadata 注入、限流和低敏观测，不拥有业务事实。

## 报告列表

| 报告 | 说明 |
| --- | --- |
| `loadtest-report-20260614-api-gateway-correlation-smoke.md` | contacts facade smoke 复用链路，验证 api-gateway 生成 / 透传最终 `trace_id` 和 `request_id` 到下游 metadata、response header 和低敏 access log |
| `loadtest-report-20260614-api-gateway-downstream-correlation-smoke.md` | contacts facade smoke 复用链路，验证 contacts-service 下游 gRPC access log 已读取并输出 api-gateway 注入的 `trace_id` / `request_id` |
| `loadtest-report-20260614-api-gateway-message-correlation-smoke.md` | secure E2E facade smoke 复用链路，验证 message-service `SendMessage` gRPC access log 已读取并输出 api-gateway 注入的 `trace_id` / `request_id` |
| `loadtest-report-20260614-api-gateway-delivery-receipt-correlation-smoke.md` | secure E2E facade smoke 复用链路，验证 delivery-service `PullInbox` 与 receipt-service `ListConversations / MarkRead` gRPC access log 已读取并输出 api-gateway 注入的 `trace_id` / `request_id` |
| `loadtest-report-20260614-api-gateway-conversation-correlation-smoke.md` | secure E2E facade smoke 复用链路，验证 conversation-service `CreateMemberChange` gRPC access log 已读取并输出 api-gateway 注入的 `trace_id` / `request_id` |
| `loadtest-report-20260614-message-conversation-correlation-smoke.md` | secure E2E facade smoke 复用链路，验证 message-service 会把 api-gateway 注入的 `trace_id` / `request_id` 继续透传给 conversation-service `GetSendContext` |

## Legacy Descriptor Observation

`tools/record-api-gateway-legacy-observation.ps1` 可用于记录 legacy descriptor quiet-window gate 证据：

```powershell
.\tools\record-api-gateway-legacy-observation.ps1 `
  -MetricsUrl http://127.0.0.1:11904/debug/metrics `
  -RunName api-gateway-legacy-observation-<date> `
  -RequiredQuietDuration 7d `
  -MaxSnapshotAge 30m
```

输出位于 `H:\NexusIM\loadtest-results\<run>`，包含 raw metrics snapshot、gate 输出、summary JSON 和 markdown report。gate 失败也会落盘，并返回非零码。该证据只代表一次 live/offline snapshot，不代表所有环境的生产迁移完成。

删除 legacy descriptor 前，如果已经按天或按固定间隔保存了多次 observation，可用窗口 gate 汇总这些 summary：

```powershell
.\tools\check-api-gateway-legacy-observation-window.ps1 `
  -SummaryRoot H:\NexusIM\loadtest-results `
  -RequiredWindow 7d `
  -MaxObservationGap 24h `
  -MinObservations 7 `
  -OutputPath H:\NexusIM\loadtest-results\api-gateway-legacy-window-summary.json
```

窗口 gate 会检查 observation 数量、持续时间、最大采样间隔、所有单次 gate 是否通过、是否仍注册 legacy descriptor、是否仍有 legacy / other exposure，以及窗口内是否有 facade 流量。它仍只是目标环境观察证据，不等于生产 SLO 或永久删除证明。

## 当前边界

- 当前是 first-stage correlation，不是完整 OpenTelemetry trace。
- correlation id 只使用低敏字符串，不包含 token、user body 或业务 payload。
- api-gateway 会优先保留 gateway token / incoming metadata 中已有的 trace/request；缺失时生成 `trace_*` / `request_*`。
- 下游服务仍通过 `x-nexusim-trace-id` 和 `x-nexusim-request-id` metadata 接收；contacts-service、conversation-service、message-service、delivery-service 和 receipt-service gRPC access log 已完成第一批落地，`message-service -> conversation-service` 的 `GetSendContext` 服务间 RPC 也已透传 correlation。后续可在其它服务间 RPC、Kafka envelope 和 OpenTelemetry exporter 中继续收敛。
