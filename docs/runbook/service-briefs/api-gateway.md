# api-gateway

## 当前状态

- 已有 `GatewayService` facade、gateway token 验证、verified metadata 注入和下游代理。
- 默认只注册 `nexusim.gateway.v1.GatewayService` facade；legacy contacts / conversation / message / delivery / receipt descriptors 需要显式 `NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=true` 才会注册，且可用 `NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL` 配置 opt-in 截止时间。
- 已有 health / ready / debug metrics / Prometheus text metrics、correlation propagation、rate limiter、RetryInfo、W3C `traceparent` 输入桥接、legacy/facade traffic audit counters 和默认关闭的 first-stage OpenTelemetry 入口 server span / 下游 gRPC client span。
- rate limiter 已支持 token scope、tenant scope、静态 tenant plan override、first-stage tenant plan 文件热更新、版本化 quota snapshot 文件、HTTP(S) `url` snapshot source、URL bearer token / HTTPS guard、URL source CA / client cert TLS 边界、applied snapshot stale 状态和 tenant plan source fail-closed guard；tenant scope 使用已验证 gateway token 的 `tenant_id` 做 per-method quota key。
- `/debug/metrics` 已暴露低敏 gRPC、facade / legacy descriptor / other request counters、legacy descriptor last-seen、auth JWK、rate-limit key scope / tenant plan count / tenant plan version / tenant plan age / stale / reload count / reload error count / identity error count、runtime config 和 OTel trace config JSON 快照，其中包含 legacy descriptor 是否仍注册及 opt-in 截止时间；`/metrics` 复用同一 snapshot 输出第一阶段 Prometheus text 指标，不输出 tenant / user / token / request id / trace id；本地 Prometheus scrape / alert rules、Grafana dashboard 原型和 trace sampling policy / check 已覆盖 api-gateway gRPC error、legacy descriptor traffic、rate-limit Redis error、tenant quota stale、JWKS refresh failure、OTLP endpoint missing、latency、exposure 和第一阶段采样治理。
- 当 `NEXUSIM_API_GATEWAY_AUTH_MODE=mock` 时，gRPC 监听地址仅允许 loopback / RFC1918 私网；公网监听地址会在启动前直接失败，避免把本地 smoke 身份模式暴露到公网。
- 当 `NEXUSIM_API_GATEWAY_AUTH_MODE=hmac|jwt` 且 gRPC 监听地址是公网地址时，若未启用入口 gRPC TLS，进程也会在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口。
- 后端启用 verified-metadata auth 时，api-gateway 启动阶段会拒绝“公网地址 + 无 mTLS client cert”的危险组合；当前已覆盖 conversation / message / delivery / receipt / contacts / identity 下游，私网 / loopback 仍可保留第一阶段 trusted metadata 直连。
- 已有 `tools/check-api-gateway-legacy-descriptor-migration.ps1`，可基于 `/debug/metrics` 或离线 JSON snapshot 做 legacy descriptor 移除前 gate；默认任何历史 legacy traffic 都失败，也可用 `-RequiredQuietDuration` 要求 `legacy_descriptor_last_seen_unix_ms` 满足静默窗口，并检查 legacy opt-in deadline 是否已过期。
- 已有 `tools/check-api-gateway-quota-snapshot.ps1`，可基于 `/debug/metrics` 或离线 JSON snapshot 做 tenant quota 门禁，检查 rate-limit 是否启用、source、version/checksum、snapshot age/stale 和 reload error。
- `ADR-033` 已固定 tenant quota source 边界：api-gateway 不直接读业务内部表；`url` source 只消费版本化 snapshot，可选 bearer token 且 bearer mode 强制 HTTPS，并支持 URL source 专用 CA / client cert；后续完整配置中心 / DB-backed quota 必须通过控制面 / 配置契约。

## 后续

- 后端服务 server span rollout、采样治理 hardening、legacy descriptor 实际迁移观察 / quiet-window gate 运行和最终删除代码、完整配置中心 quota source hardening。
