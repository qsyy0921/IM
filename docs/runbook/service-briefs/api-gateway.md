# api-gateway

## 当前状态

- 已有 `GatewayService` facade、gateway token 验证、verified metadata 注入和下游代理。
- 已有 health / ready / metrics、correlation propagation、基础 rate limiter。
- 后端启用 verified-metadata auth 时，api-gateway 启动阶段会拒绝“公网地址 + 无 mTLS client cert”的危险组合；私网 / loopback 仍可保留第一阶段 trusted metadata 直连。

## 后续

- 统一 OpenTelemetry、配额治理、逐步收敛 legacy descriptors。
