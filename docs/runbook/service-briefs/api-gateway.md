# api-gateway

## 当前状态

- 已有 `GatewayService` facade、gateway token 验证、verified metadata 注入和下游代理。
- 默认只注册 `nexusim.gateway.v1.GatewayService` facade；legacy contacts / conversation / message / delivery / receipt descriptors 需要显式 `NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=true` 才会注册。
- 已有 health / ready / metrics、correlation propagation、rate limiter、RetryInfo、W3C `traceparent` 输入桥接和默认关闭的 first-stage OpenTelemetry gRPC server span。
- rate limiter 已支持 token scope、tenant scope、静态 tenant plan override 和 first-stage tenant plan 文件热更新；tenant scope 使用已验证 gateway token 的 `tenant_id` 做 per-method quota key。
- `/debug/metrics` 已暴露低敏 gRPC、auth JWK、rate-limit key scope / tenant plan count / tenant plan reload count / reload error count / identity error count、runtime config 和 OTel trace config 快照，其中包含 legacy descriptor 是否仍注册。
- 当 `NEXUSIM_API_GATEWAY_AUTH_MODE=mock` 时，gRPC 监听地址仅允许 loopback / RFC1918 私网；公网监听地址会在启动前直接失败，避免把本地 smoke 身份模式暴露到公网。
- 当 `NEXUSIM_API_GATEWAY_AUTH_MODE=hmac|jwt` 且 gRPC 监听地址是公网地址时，若未启用入口 gRPC TLS，进程也会在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口。
- 后端启用 verified-metadata auth 时，api-gateway 启动阶段会拒绝“公网地址 + 无 mTLS client cert”的危险组合；当前已覆盖 conversation / message / delivery / receipt / contacts / identity 下游，私网 / loopback 仍可保留第一阶段 trusted metadata 直连。

## 后续

- OTel collector / alerting / 跨服务 rollout、legacy descriptor opt-in 使用面的后续迁移审计、配置中心 / DB-backed quota hardening。
