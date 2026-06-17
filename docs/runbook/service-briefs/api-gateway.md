# api-gateway

## 当前状态

- 已有 `GatewayService` facade、gateway token 验证、verified metadata 注入和下游代理，覆盖 delivery `HideInboxItem` 等当前公开主链路 RPC。
- 默认只注册 `nexusim.gateway.v1.GatewayService` facade；legacy contacts / conversation / message / delivery / receipt descriptors 需要显式 `NEXUSIM_API_GATEWAY_REGISTER_LEGACY_DESCRIPTORS=true` 才会注册，且可用 `NEXUSIM_API_GATEWAY_LEGACY_DESCRIPTORS_ALLOWED_UNTIL` 配置 opt-in 截止时间。
- 已有 health / ready / debug metrics / Prometheus text metrics、correlation propagation、rate limiter、RetryInfo、W3C `traceparent` 输入桥接、legacy/facade traffic audit counters 和默认关闭的 first-stage OpenTelemetry 入口 server span / 下游 gRPC client span。
- gRPC 结构化访问日志已限制 `trace_id` / `request_id` 为低敏安全字符集，外部 metadata / body / token 中夹带的邮箱、token 或任意文本不会进入日志 correlation 字段。
- rate limiter 已支持 token scope、tenant scope、静态 tenant plan override、first-stage tenant plan 文件热更新、版本化 quota snapshot 文件、file / URL snapshot source 大小上限、HTTP(S) `url` snapshot source、URL bearer token / HTTPS guard、URL source CA / client cert TLS 边界、拒绝 URL userinfo / redirect、可选 checksum-required gate、future `generated_at_unix_ms` 拒绝、applied snapshot stale 状态和 tenant plan source fail-closed guard；URL source 外部 fetch 错误返回稳定低敏文案，不记录 URL query / bearer token；tenant scope 使用已验证 gateway token 的 `tenant_id` 做 per-method quota key，内部身份预检不把 gateway token 或 trace metadata 写入 request URL。
- rate limiter Redis backend 已支持 single / Sentinel / Cluster 第一阶段客户端配置；Cluster 依赖 `NEXUSIM_API_GATEWAY_RATE_LIMIT_REDIS_CLUSTER_ADDRS`，用于本地 / 分布式模拟中的 quota Redis 拓扑接入，不代表生产级 Redis HA sizing 已完成。
- `/debug/metrics` 已暴露低敏 gRPC、facade / legacy descriptor / other request counters、legacy descriptor last-seen、auth JWK、rate-limit key scope / Redis mode / tenant plan count / tenant plan version / tenant plan age / stale / checksum-required / URL guard / reload count / reload error count / identity error count、runtime config 和 OTel trace config JSON 快照，其中包含 legacy descriptor 是否仍注册及 opt-in 截止时间；`/metrics` 复用同一 snapshot 输出第一阶段 Prometheus text 指标，并以低基数 `redis_mode` label 标明 single / Sentinel / Cluster 配置，不输出 tenant / user / token / request id / trace id；debug HTTP 监听默认只允许 loopback / RFC1918 私网，公网或 unspecified 地址必须显式 `NEXUSIM_API_GATEWAY_DEBUG_ALLOW_PUBLIC=true`；本地 Prometheus scrape / alert rules、Grafana dashboard 原型和 trace sampling policy / check 已覆盖 api-gateway gRPC error、legacy descriptor traffic / registered / opt-in deadline、rate-limit Redis mode / Redis / identity / reload error、tenant quota stale、JWKS refresh failure、OTLP endpoint missing、latency、exposure 和第一阶段采样治理。
- 当 `NEXUSIM_API_GATEWAY_AUTH_MODE=mock` 时，gRPC 监听地址仅允许 loopback / RFC1918 私网；公网监听地址会在启动前直接失败，避免把本地 smoke 身份模式暴露到公网。
- 当 `NEXUSIM_API_GATEWAY_AUTH_MODE=hmac|jwt` 且 gRPC 监听地址是公网地址时，若未启用入口 gRPC TLS，进程也会在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口。
- gRPC TLS / mTLS 配置已有 TLS 1.2 minimum、client certificate identity allowlist positive path 和 unlisted identity rejection 单测；`check-local` 通过 `tools/check-grpc-tls-config-guards.ps1` 强制保留这些本地门禁。
- 后端启用 verified-metadata auth 时，api-gateway 启动阶段会拒绝“公网地址 + 无 mTLS client cert”的危险组合；当前已覆盖 conversation / message / delivery / receipt / contacts / identity 下游，私网 / loopback 仍可保留第一阶段 trusted metadata 直连。
- 已有 `tools/check-api-gateway-legacy-descriptor-migration.ps1`、`record-api-gateway-legacy-observation.ps1`、`check-api-gateway-legacy-observation-window.ps1`、`write-api-gateway-legacy-removal-plan.ps1` 和 `validate-api-gateway-legacy-removal-plan.ps1`，可把 legacy quiet-window 证据汇总并验证为低敏、不可执行、需审批的 descriptor removal plan；默认任何历史 legacy traffic 都会让 gate / plan 阻塞。
- 已有 `tools/check-api-gateway-quota-snapshot.ps1`，可基于 `/debug/metrics` 或离线 JSON snapshot 做 tenant quota 门禁，检查 rate-limit 是否启用、source、Redis mode、version/checksum、checksum-required policy、URL HTTPS / bearer / TLS / client-cert guard、snapshot age / future timestamp / stale、reload error、Redis / identity 错误、tenant plan 数、tracked key 数和最近 reload 成功时间；`check-local` 已包含 legacy / quota gate 正反样本自检。
- `ADR-033` 已固定 tenant quota source 边界：api-gateway 不直接读业务内部表；`url` source 只消费版本化 snapshot，可选 bearer token 且 bearer mode 强制 HTTPS，并支持 URL source 专用 CA / client cert 和强制 checksum；后续完整配置中心 / DB-backed quota 必须通过控制面 / 配置契约。
- tenant quota source / snapshot / reload helper 已从 `cmd/api-gateway/main.go` 拆到同 package 文件，composition root 继续负责 wiring，避免继续堆大文件。
- cmd 层 rate-limit / tenant-plan 配置测试已从 `main_test.go` 拆到同 package `rate_limit_config_test.go`，保留原启动配置覆盖，同时降低单个测试文件复杂度。
- `loadtest/demo --gateway-facade` summary 已输出 `capacity_summary`，包含 GatewayService facade 标记、gateway auth mode、端到端用户侧操作数、WebSocket frame 数、PullInbox item 数、最大 conversation seq、read 前后 unread、PG 聚合和 policy audit Kafka readback 数；本地 api-gateway stack 短基线已跑通 secure mTLS + HMAC facade 链路，clean summary 记录 `git_dirty=false`、7 个用户侧操作、3 个 WebSocket frame、PullInbox 1 条、unread 1->0、policy audit Kafka readback 1 条。
- `loadtest/demo` runner 已按 config / model / auth / summary helper 同 package 拆分，避免 api-gateway facade 演示和容量验证继续堆进单个 `main.go`。
- `loadtest/demo/run-local-secure-demo.ps1` 的 gRPC / debug 端口已参数化，默认端口不变；本地被 Docker / WSL port proxy 占用时可传入备用端口，避免 secure demo 被固定端口阻塞。

## 后续

- 采样治理 hardening、统一 OTel collector / alerting / dashboard、在目标环境持续运行 legacy quiet-window observation window gate、归档 removal plan 并最终删除 legacy descriptor 代码、完整配置中心 quota source hardening、长时间容量曲线和生产 sizing。
