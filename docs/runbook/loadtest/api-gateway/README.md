# api-gateway Loadtest Reports

本目录保存 api-gateway 入口能力的小规模验证报告。api-gateway 只做 user-facing 入口鉴权、身份覆盖、trusted metadata 注入、限流和低敏观测，不拥有业务事实。

## 报告列表

| 报告 | 说明 |
| --- | --- |
| `loadtest-report-20260614-api-gateway-correlation-smoke.md` | contacts facade smoke 复用链路，验证 api-gateway 生成 / 透传最终 `trace_id` 和 `request_id` 到下游 metadata、response header 和低敏 access log |
| `loadtest-report-20260614-api-gateway-downstream-correlation-smoke.md` | contacts facade smoke 复用链路，验证 contacts-service 下游 gRPC access log 已读取并输出 api-gateway 注入的 `trace_id` / `request_id` |
| `loadtest-report-20260614-api-gateway-message-correlation-smoke.md` | secure E2E facade smoke 复用链路，验证 message-service `SendMessage` gRPC access log 已读取并输出 api-gateway 注入的 `trace_id` / `request_id` |
| `loadtest-report-20260614-api-gateway-delivery-receipt-correlation-smoke.md` | secure E2E facade smoke 复用链路，验证 delivery-service `PullInbox` 与 receipt-service `ListConversations / MarkRead` gRPC access log 已读取并输出 api-gateway 注入的 `trace_id` / `request_id` |

## 当前边界

- 当前是 first-stage correlation，不是完整 OpenTelemetry trace。
- correlation id 只使用低敏字符串，不包含 token、user body 或业务 payload。
- api-gateway 会优先保留 gateway token / incoming metadata 中已有的 trace/request；缺失时生成 `trace_*` / `request_*`。
- 下游服务仍通过 `x-nexusim-trace-id` 和 `x-nexusim-request-id` metadata 接收；contacts-service、message-service、delivery-service 和 receipt-service gRPC access log 已完成第一批落地，后续可在其它服务结构化日志、Kafka envelope 和 OpenTelemetry exporter 中继续收敛。
