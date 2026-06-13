# api-gateway

## 当前状态

- 已有 `GatewayService` facade、gateway token 验证、verified metadata 注入和下游代理。
- 已有 health / ready / metrics、correlation propagation、基础 rate limiter。
- 后端启用 verified-metadata auth 时必须配合 loopback / 内网隔离或 mTLS peer allowlist。

## 后续

- 统一 OpenTelemetry、配额治理、逐步收敛 legacy descriptors。
