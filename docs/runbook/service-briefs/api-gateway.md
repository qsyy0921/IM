# api-gateway

## 当前状态

- 已有 `GatewayService` facade、gateway token 验证、verified metadata 注入和下游代理。
- 已有 health / ready / metrics、correlation propagation、rate limiter、RetryInfo 和 W3C `traceparent` 输入桥接。
- rate limiter 已支持 token scope、tenant scope 和静态 tenant plan override；tenant scope 使用已验证 gateway token 的 `tenant_id` 做 per-method quota key。
- `/debug/metrics` 已暴露低敏 gRPC、auth JWK、rate-limit key scope / tenant plan count / identity error count 和 runtime config 快照，其中包含 legacy descriptor 是否仍注册。
- 当 `NEXUSIM_API_GATEWAY_AUTH_MODE=mock` 时，gRPC 监听地址仅允许 loopback / RFC1918 私网；公网监听地址会在启动前直接失败，避免把本地 smoke 身份模式暴露到公网。
- 当 `NEXUSIM_API_GATEWAY_AUTH_MODE=hmac|jwt` 且 gRPC 监听地址是公网地址时，若未启用入口 gRPC TLS，进程也会在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口。
- 后端启用 verified-metadata auth 时，api-gateway 启动阶段会拒绝“公网地址 + 无 mTLS client cert”的危险组合；当前已覆盖 conversation / message / delivery / receipt / contacts / identity 下游，私网 / loopback 仍可保留第一阶段 trusted metadata 直连。

## 后续

- 统一 OpenTelemetry、运行时动态 quota 配置、逐步把 legacy descriptors 从兼容默认迁移到显式 opt-in。
