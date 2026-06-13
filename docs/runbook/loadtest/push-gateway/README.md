# push-gateway Loadtest / Smoke Index

本文是 `push-gateway` 验证报告入口。当前已完成六层骨架、WebSocket frame codec、in-memory session registry、delivery event consumer、identity revoke consumer、`server.pong`、`delivery.notify`、`delivery.ack.ok`、queue-full broad `server.resume_hint` active close、单实例 in-memory resume buffer TTL、Redis route 最小 adapter、Redis-backed cross-instance resume buffer 第一版、HMAC signed gateway token 第一版、标准三段 JWT HS256 gateway token 兼容、RS256 JWKS URL refresh/cache + debug stats、identity-service Login / RegisterUser 签发 JWT gateway token smoke、Redis deny-list revoke projection 和 device / session revoke active close；真实进程 full smoke、HMAC/JWT auth smoke、identity revoke deny-list / active-close smoke、同 user 多 device notify smoke、slow-client 负向 smoke、单实例 resume replay smoke、跨进程 Redis route smoke、cross-instance resume smoke、Win-Mac 双机 cross-instance resume smoke，以及 `edit / revoke / delete` 三类 message-change notify smoke 均已通过。

## 当前验证目标

第一阶段只验证在线通知链路，不做 WebSocket 容量极限：

```text
delivery_outbox
-> delivery-service outbox-relay
-> Kafka im.delivery.events
-> push-gateway delivery event consumer
-> online WebSocket client receives delivery.notify
-> client PullInbox reads durable user_inbox
-> client sends delivery.ack frame
-> push-gateway calls delivery-service AckDelivery
-> client receives delivery.ack.ok
```

必须证明：

- push-gateway 消费的是 `im.delivery.events`，不是 `conversation.timeline.events`。
- `delivery.notify` 是轻量唤醒信号，不是 message 事实源。
- `delivery.notify.source_event_type` 只用于区分新增 / 编辑 / 撤回 / 删除唤醒，客户端展示事实仍以 `PullInbox` 为准。
- 客户端展示和本地持久化以 `PullInbox` 返回为准。
- ACK 仍由 `delivery-service AckDelivery` 推进 cursor。
- `delivery.ack` 成功必须有 `delivery.ack.ok`，失败必须返回稳定 error frame。
- push-gateway 不直接读写 `message_log`、`conversation_members`、`user_inbox`、`device_delivery_cursors`。

当前最小运行模式：

```text
NEXUSIM_PUSH_GATEWAY_MODE=all
NEXUSIM_PUSH_WS_ADDR=0.0.0.0:10496
NEXUSIM_DELIVERY_GRPC_ADDR=127.0.0.1:10497
NEXUSIM_KAFKA_BROKERS=localhost:9092
NEXUSIM_DELIVERY_EVENTS_TOPIC=im.delivery.events
NEXUSIM_PUSH_CONSUMER_GROUP=nexusim-push-gateway-smoke
```

默认情况下，push-gateway 的 `AckDelivery` RPC client 使用 plaintext gRPC。若 delivery-service gRPC server 在本地或双机 smoke 中开启 TLS / mTLS，WebSocket gateway 进程需要配置对应出站 TLS：

```text
NEXUSIM_DELIVERY_SERVICE_TLS_CA_FILE=...
NEXUSIM_DELIVERY_SERVICE_TLS_SERVER_NAME=delivery-service.nexusim.local
NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_CERT_FILE=...
NEXUSIM_DELIVERY_SERVICE_TLS_CLIENT_KEY_FILE=...
```

配置任一 TLS env 后必须提供 CA file，client cert/key 必须成对配置。现有 smoke 默认不启用这些参数；该配置只验证静态证书下的 RPC 加密 / mTLS 连接，不代表证书签发、轮换、分发或全服务 mTLS rollout。

`loadtest/pushgateway` 自身调用 conversation / message / delivery / identity gRPC 时也默认 plaintext。若这些服务端在外部或双机 smoke 中启用 TLS / mTLS，可给 smoke runner 传入对应 client 参数：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -ConversationTlsCaFile .\certs\ca.pem `
  -ConversationTlsServerName conversation-service.nexusim.local `
  -ConversationTlsClientCertFile .\certs\loadtest-client.crt `
  -ConversationTlsClientKeyFile .\certs\loadtest-client.key `
  -MessageTlsCaFile .\certs\ca.pem `
  -MessageTlsServerName message-service.nexusim.local `
  -MessageTlsClientCertFile .\certs\loadtest-client.crt `
  -MessageTlsClientKeyFile .\certs\loadtest-client.key `
  -DeliveryTlsCaFile .\certs\ca.pem `
  -DeliveryTlsServerName delivery-service.nexusim.local `
  -DeliveryTlsClientCertFile .\certs\loadtest-client.crt `
  -DeliveryTlsClientKeyFile .\certs\loadtest-client.key `
  -IdentityTlsCaFile .\certs\ca.pem `
  -IdentityTlsServerName identity-service.nexusim.local `
  -IdentityTlsClientCertFile .\certs\loadtest-client.crt `
  -IdentityTlsClientKeyFile .\certs\loadtest-client.key
```

这些参数只覆盖 smoke runner 到四个 gRPC server 的静态 TLS / mTLS 连接，不覆盖 WebSocket TLS，也不改变 push-gateway 进程自身的 `AckDelivery` 出站 TLS env。

`all` 模式只用于第一阶段本地 smoke：WebSocket handler 和 `im.delivery.events` consumer 共享同一个进程内 session registry。默认 route backend 仍是 memory；跨实例在线路由需要启用 Redis route。

本地分布式模拟使用两个独立 `push-gateway` 进程：

```text
push-gateway-ws       -> 只负责 WebSocket 连接和本机 session registry
push-gateway-consumer -> 只消费 Kafka im.delivery.events
Redis route / PubSub  -> 把 consumer 进程收到的 delivery.notify 转发到 ws 进程
```

这能在一台机器上证明 push-gateway 已经从单进程 `all` 模式推进到最小分布式在线路由模型。它仍不是生产多实例结论：Redis Pub/Sub 是 best-effort online wakeup，可靠恢复仍依赖 `delivery-service PullInbox`。

Redis route 最小参数：

```text
NEXUSIM_PUSH_ROUTE_BACKEND=redis
NEXUSIM_PUSH_GATEWAY_ID=push-gateway-a
NEXUSIM_PUSH_REDIS_MODE=single
NEXUSIM_PUSH_REDIS_ADDR=127.0.0.1:6379
NEXUSIM_PUSH_REDIS_USERNAME=
NEXUSIM_PUSH_REDIS_PASSWORD=
NEXUSIM_PUSH_REDIS_DB=0
NEXUSIM_PUSH_REDIS_KEY_PREFIX=nexusim:push
NEXUSIM_PUSH_RESUME_BUFFER_TTL=10m
NEXUSIM_PUSH_ROUTE_TTL=90s
NEXUSIM_PUSH_ROUTE_CLEANUP_INTERVAL=30s
```

Redis Sentinel client 参数：

```text
NEXUSIM_PUSH_REDIS_MODE=sentinel
NEXUSIM_PUSH_REDIS_SENTINEL_MASTER_NAME=mymaster
NEXUSIM_PUSH_REDIS_SENTINEL_ADDRS=127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381
NEXUSIM_PUSH_REDIS_USERNAME=
NEXUSIM_PUSH_REDIS_PASSWORD=
NEXUSIM_PUSH_REDIS_SENTINEL_USERNAME=
NEXUSIM_PUSH_REDIS_SENTINEL_PASSWORD=
NEXUSIM_PUSH_REDIS_DB=0
```

Sentinel 模式当前已证明三件事：客户端 master discovery 正常路径可用；本地三 Redis + 三 Sentinel Docker 拓扑下，手动 `SENTINEL failover mymaster` 后 route / resume / `PullInbox + AckDelivery` recovery smoke 通过；停止 Sentinel 当前 master 容器后，Sentinel 自主选主，route / resume / `PullInbox + AckDelivery` recovery smoke 也通过。它仍不等于完整 Redis HA 验收；quorum 异常、网络分区、Redis Cluster、切主窗口内零丢失和容量结论仍未覆盖。

## 报告位置

当前报告：

| 报告 | 说明 |
| --- | --- |
| `loadtest-report-20260609-push-gateway-full-smoke.md` | `delivery_outbox -> im.delivery.events -> push-gateway -> WebSocket notify -> PullInbox -> AckDelivery` 真实进程 smoke |
| `loadtest-report-20260610-push-gateway-hmac-auth-smoke.md` | `NEXUSIM_PUSH_AUTH_MODE=hmac` 下使用 `Authorization: Bearer` signed gateway token 完成 WebSocket notify / PullInbox / AckDelivery |
| `loadtest-report-20260609-push-gateway-multidevice-smoke.md` | 同一 user 两个在线 device 均收到同一条 `delivery.notify`，并分别 ACK 到各自 cursor |
| `loadtest-report-20260609-push-gateway-slow-client-smoke.md` | 慢客户端触发 queue full / active close 后，通过 durable `PullInbox` 补拉并 ACK |
| `loadtest-report-20260609-push-gateway-resume-replay-smoke.md` | 单实例 in-memory resume buffer 命中后，重连客户端收到同一条 `delivery.notify` replay |
| `loadtest-report-20260609-push-gateway-redis-route-smoke.md` | WebSocket gateway 与 delivery consumer gateway 分离后，通过 Redis route / PubSub 完成跨进程在线通知 |
| `loadtest-report-20260609-push-gateway-redis-route-ttl-smoke.md` | Redis route 增加 TTL 续期后，clean commit 上再次验证跨进程在线通知 |
| `loadtest-report-20260609-push-gateway-redis-fault-smoke.md` | Redis route 中断时，在线 notify 可丢，但客户端可通过 durable `PullInbox` 恢复并 ACK |
| `loadtest-report-20260609-push-gateway-cross-instance-resume-smoke.md` | 客户端从 WebSocket gateway A 断开后重连到 gateway B，命中 Redis-backed resume buffer 并 replay 同一条 `delivery.notify` |
| `loadtest-report-20260609-push-gateway-win-mac-cross-instance-resume-smoke.md` | 首连 WebSocket gateway 在 Mac Docker，重连 gateway 在 Windows，命中 Redis-backed resume buffer 并 replay 同一条 `delivery.notify` |
| `loadtest-report-20260609-push-gateway-redis-sentinel-route-resume-smoke.md` | Redis Sentinel discovery 正常路径下，跨实例 route / resume smoke 通过；不代表 failover / HA 验收 |
| `loadtest-report-20260609-push-gateway-redis-sentinel-failover-smoke.md` | 本地三 Redis / 三 Sentinel 拓扑下触发 `SENTINEL failover mymaster`，切主后 route / resume / PullInbox / AckDelivery 恢复通过；不代表完整 Redis HA |
| `loadtest-report-20260609-push-gateway-redis-sentinel-master-stop-smoke.md` | 停止 Sentinel 当前 master 容器，等待 Sentinel 自主选主后继续 route / resume / PullInbox / AckDelivery；不代表 quorum / 网络分区 / Redis Cluster 验收 |
| `loadtest-report-20260612-win-mac-arm64-distributed-smoke.md` | Mac Docker arm64 WebSocket gateway + Windows core services / Kafka / Redis 的双机 full route 和 cross-instance resume smoke |
| `loadtest-report-20260612-push-gateway-identity-token-smoke.md` | identity-service 签发短期 gateway token；legacy 自定义 HMAC token、`IssueGatewayToken(jwt)`、`Login(jwt)` 和 `RegisterUser -> Login(jwt)` 均已通过 push-gateway 本地验签并完成 `delivery.notify -> PullInbox -> AckDelivery` |
| `loadtest-report-20260612-push-gateway-identity-revoke-smoke.md` | `RevokeDevice/RevokeSession -> identity_outbox -> im.identity.events -> push-gateway identity-consumer -> Redis deny-list / Redis route eviction` 后，旧在线连接收到 `server.resume_hint(reason=identity_revoked)` 并被主动关闭，旧 gateway token 重连返回 `PERMISSION_DENIED`；session revoke smoke 额外验证 same-device survivor session 仍可 `server.pong` |
| `loadtest-report-20260610-push-gateway-message-change-notify-smoke.md` | `edit / revoke / delete` 三类消息变更均能触发带正确 `source_event_type` 的 `delivery.notify`，且与 `PullInbox` durable item 一致 |

报告 Markdown 保存在仓库内：

```text
E:\development\IM\docs\runbook\loadtest\push-gateway\
```

推荐命名：

```text
loadtest-report-YYYYMMDD-push-gateway-smoke.md
```

中大型原始数据、长日志和趋势图保存到机械盘：

```text
H:\NexusIM\loadtest-results
```

小规模 smoke 的轻量 summary 可以临时放在：

```text
E:\development\IM\loadtest\results
```

## Message Change Notify Smoke

`loadtest/pushgateway/run-local-smoke.ps1` 已支持 `message-change-notify` 场景，用同一条在线 WebSocket 链路验证消息变更通知：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 -Scenario message-change-notify -MessageChangeAction edit
.\loadtest\pushgateway\run-local-smoke.ps1 -Scenario message-change-notify -MessageChangeAction revoke
.\loadtest\pushgateway\run-local-smoke.ps1 -Scenario message-change-notify -MessageChangeAction delete
```

该 runner 会验证 `delivery.notify.source_event_type` 分别为 `message.edited.v1` / `message.revoked.v1` / `message.deleted.v1`，并继续用 `PullInbox` 精确校验 durable inbox 中的 `event_type + message_id + conversation_seq`。三类真实进程 smoke 已在 clean commit `81fe92c` 归档到 `loadtest-report-20260610-push-gateway-message-change-notify-smoke.md`。

## HMAC / JWT Auth / Revoke Smoke

`loadtest/pushgateway/run-local-smoke.ps1` 支持在同一条 full smoke 链路中启用 HMAC signed gateway token：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -PushAuthMode hmac `
  -PushAuthHmacSecret local-push-smoke-secret
```

密钥轮换窗口可以用 current + previous secrets 做最小 smoke：服务端用 current secret 配置新签发密钥，同时把旧密钥放入 previous secrets；runner 用旧密钥签 token，验证旧 token 在 TTL 窗口内仍可建连。

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -PushAuthMode hmac `
  -PushAuthHmacSecret new-local-push-secret `
  -PushAuthHmacPreviousSecrets old-local-push-secret `
  -PushAuthTokenSigningSecret old-local-push-secret
```

HMAC 模式下 runner 用 `Authorization: Bearer` 传 token，summary 会记录：

```text
push_auth_mode=hmac
push_auth_token_transport=authorization_header
push_auth_hmac_previous_secrets_configured=true|false
push_auth_token_signing_secret_explicit=true|false
push_auth_token_signed_with_non_current_secret=true|false
push_auth_query_identity_sent=false
```

这证明 smoke 没有依赖 WebSocket query 中的裸 `tenant_id/user_id`。报告见 `loadtest-report-20260610-push-gateway-hmac-auth-smoke.md`。

identity-service 参与签发 token 时，runner 使用 `-UseIdentityServiceToken` 调用 `IssueGatewayToken`，push-gateway 本地验签，不在 WebSocket 热路径同步查询 identity-service：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -UseIdentityServiceToken `
  -PushAuthMode hmac `
  -PushAuthHmacSecret local-push-smoke-secret
```

identity-service 也可以在相同链路中签发标准三段 JWT HS256 gateway token：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -UseIdentityServiceToken `
  -PushAuthMode hmac `
  -IdentityGatewayTokenFormat jwt `
  -PushAuthHmacSecret local-push-smoke-secret
```

该模式下 summary 会记录 `identity_gateway_token_format=jwt`。HS256 仍是本地共享密钥兼容模式：push-gateway 通过 `NEXUSIM_PUSH_AUTH_HMAC_SECRET` 本地验签，identity-service 不再通过 JWKS 暴露对称密钥。identity-service debug server 的 `/.well-known/jwks.json` / `/jwks.json` 只用于 RS256 公钥发现和旧公钥 overlap；生产级自动 key rotation、KMS/HSM 和多 issuer 治理仍是后续切片。

如果要验证真实 Login 凭据入口，而不是直接调用内部签发 RPC，可以使用 `-IdentityTokenMethod login`。runner 只 seed 已有用户的密码哈希；device、session、refresh token 和 gateway token 都由 identity-service `Login` 写入 / 签发：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -UseIdentityServiceToken `
  -PushAuthMode hmac `
  -IdentityGatewayTokenFormat jwt `
  -IdentityTokenMethod login `
  -PushAuthHmacSecret local-push-smoke-secret
```

该模式下 summary 会记录 `push_auth_token_source=identity_service_login` 和 `identity_token_method=login`。这证明 Login 最小凭据链路可以接入 push-gateway，但不代表注册、密码重置、MFA、OIDC 或登录风控已经完成。

如果要验证注册入口也在真实链路中，可以使用 `-IdentityTokenMethod register_login`。runner 不再直接 seed `identity_users.password_hash`，而是先调用 `RegisterUser`，再调用 `Login`：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario full `
  -UseIdentityServiceToken `
  -PushAuthMode hmac `
  -IdentityGatewayTokenFormat jwt `
  -IdentityTokenMethod register_login `
  -PushAuthHmacSecret local-push-smoke-secret
```

该模式下 summary 会记录 `push_auth_token_source=identity_service_register_login` 和 `identity_token_method=register_login`。它证明最小注册凭据链路可以接入 push-gateway，但仍不代表邮箱 / 手机验证、密码重置、MFA、OIDC 或登录风控已经完成。

device / session revoke projection smoke 使用 `identity-revoke` 场景：

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario identity-revoke `
  -UseIdentityServiceToken `
  -PushAuthMode hmac `
  -RouteBackend redis `
  -PushAuthHmacSecret local-push-smoke-secret
```

该场景默认验证 device revoke：旧 token 先能建连，随后 `RevokeDevice -> identity_outbox -> im.identity.events -> push-gateway identity-consumer -> Redis deny-list` 生效；在 Redis route 分进程模式下，还会保持旧 WebSocket 在线并等待 `server.resume_hint(reason=identity_revoked)` + `StatusPolicyViolation` active close，最后验证同一个旧 token 重连返回 `PERMISSION_DENIED`。session revoke 可以加 `-IdentityRevokeScope session`，runner 会创建同一 device 的两条 session，吊销其中一条并验证目标 session 被主动关闭、survivor session 仍可 `client.ping -> server.pong`。报告见 `loadtest-report-20260612-push-gateway-identity-revoke-smoke.md`。

## 第一阶段不做

- 不做十万级 WebSocket 长连接压测。
- 不打满 Win-Mac 2.5Gbps 链路。
- 不重新做 message-service 硬件矩阵。
- 不把短时 resume buffer 当作 durable inbox。
- 不把 Redis-backed resume buffer 表述为可靠投递或生产级跨实例恢复。当前第一版只缓存轻量 `delivery.notify`，未知、过期、身份不匹配、Redis miss/error 或 buffer gap 都必须回退 `server.resume_hint + PullInbox`；未知客户端 `resume_token` 必须返回 `buffer_miss` 并由服务端签发新 token。
- 不把第一版 HMAC/JWT gateway token 表述为完整 identity-service。`NEXUSIM_PUSH_AUTH_MODE=hmac` 当前同时兼容 legacy 自定义 HMAC token 和标准三段 JWT HS256 token；`NEXUSIM_PUSH_AUTH_MODE=jwt` 当前只接受 RS256 公钥 JWKS 验签和可选 issuer allowlist，JWKS 可来自静态 JSON/file 或 URL 启动拉取 + 定期 refresh/cache。URL 模式会在 `/debug/metrics.auth_jwks` 暴露缓存 key 数、最近 refresh 成功 / 失败时间和失败计数；identity-service 可用额外 JWKS JSON/file 暴露旧公钥用于手动轮换 overlap。两种模式都只校验短期 signed token 的签名、`aud=push-gateway`、过期时间、device 绑定和本地/Redis deny-list。当前支持 HMAC current + previous secrets 的最小密钥轮换，并已补 device/session revoke event consumer，device revoke 和 session revoke 的 Redis deny-list / active close smoke 均已通过。`RegisterUser`、`Login`、RS256 JWKS 和 refresh token rotation 已有最小实现与测试 / smoke 证据，但自动轮换、KMS/HSM、多 issuer 治理、邮箱 / 手机验证、密码重置、MFA、OIDC federation 和登录风控仍是后续真实鉴权切片。`mock` auth 只用于本地 smoke。
- 不把 push smoke 表述为生产容量结论。
- 不把 queue-full active close 表述为完整慢连接治理；当前 `server.resume_hint` 只是 broad pull fallback，客户端必须用本地 durable cursor 决定 `PullInbox` 起点。已完成单实例 slow-client 真实进程负向 smoke，它验证的是 durable `PullInbox` fallback；已另外完成单实例 resume replay smoke 和 cross-instance resume smoke，分别验证短时 in-memory buffer 命中路径和 Redis-backed 跨 gateway replay 路径；后续还没有多实例慢连接验证。
- `/debug/metrics` 目前暴露单实例 in-memory registry、Redis route、Redis resume 和 auth JWKS 调试指标，用于 smoke 排障；其中 resume token count / expired token count 只说明本进程短时 buffer 状态，`redis_resume_*` 只说明 Redis-backed replay / miss / append 过程；`redis_registry_metrics` 和 `redis_subscriber_metrics` 只说明 online route / PubSub / local fanout 过程，不代表 durable delivery 成功率；`auth_jwks` 只说明本进程 RS256 key cache / refresh 状态，不代表完整 issuer federation 或 KMS 状态；该端点不是生产级 Prometheus 指标。WebSocket gateway 可通过 `NEXUSIM_PUSH_WS_ADDR` 暴露该端点，consumer-only gateway 可通过 `NEXUSIM_PUSH_DEBUG_ADDR` 单独暴露只读 debug 端点。
- `NEXUSIM_PUSH_TEST_WRITE_DELAY` 只允许本地 smoke 使用，生产环境必须 unset 或保持 `0`。
- Redis route 当前对在线通知采用 fail-open：lookup / publish 错误不会阻塞 delivery consumer 提交当前 Kafka event；该次在线唤醒可以丢，客户端靠 durable `PullInbox` 恢复。connect 写 route 失败仍 fail-closed，避免把无法跨实例路由的 session 注册成在线。后台 cleanup loop 已能清理 missing / malformed / mismatched stale route；clean commit `074902b` 已完成一次真实 Redis stop/start fault smoke，证明 Redis route 中断时 `PullInbox + AckDelivery` 仍可恢复；clean commit `7bc35a5` 已完成 Redis Sentinel discovery 正常路径下的 route / resume smoke；clean commit `819c14a` 已完成手动 Sentinel master failover 后的 route / resume recovery smoke；clean commit `8ddc2fb` 已完成停止 Sentinel 当前 master 容器后的自动切主 recovery smoke。它们仍不是完整 Redis HA / quorum / 网络分区 / Cluster 结论。

## 面试可讲点

`push-gateway` 的价值不是“直接把消息正文推给客户端”，而是把在线通道放在 durable delivery 之后：

```text
message-service 写消息事实
-> delivery-service 写 user_inbox / delivery_outbox
-> push-gateway 只做在线唤醒
-> 客户端 PullInbox + AckDelivery 完成可靠投递
```

这样断线、重连、成员边界、ACK 丢失和服务重启都可以由 `delivery-service` 的 durable inbox / cursor 兜底。

分布式可讲点：

```text
同一个用户的 WebSocket 连接可能落在 gateway A
Kafka delivery event 可能被 gateway B 消费
gateway B 通过 Redis route 找到 gateway A
gateway A 只做在线 notify
客户端最终仍通过 PullInbox / AckDelivery 完成可靠投递
```

这体现了在线唤醒层和可靠投递层解耦：Redis route 可以丢，WebSocket 可以断，但 message fact、user_inbox 和 ACK cursor 不丢。

双机可讲点：

```text
Windows 运行 PostgreSQL / Kafka / Redis / 核心业务服务 / push delivery-consumer
Mac Docker 运行 push-gateway WebSocket gateway
客户端首连 Mac gateway，断开后重连 Windows gateway
Redis-backed resume buffer replay 同一条 delivery.notify
最终仍用 PullInbox / AckDelivery 验证可靠投递
```

这证明当前系统已经不是单进程 WebSocket demo，而是能把在线连接、Kafka consumer、Redis route、Redis resume 和 durable delivery read model 拆到不同进程 / 不同机器上协作。

排查跨实例在线路由时，优先看 `/debug/metrics` 中的几类计数：

- consumer gateway：`redis_route_remote_matched_sessions`、`redis_route_remote_publish_call_count`、`redis_route_remote_publish_error_count`。
- WebSocket gateway：`redis_route_subscriber_message_count`、`redis_route_subscriber_enqueued_count`、`redis_route_subscriber_evicted_count`。
- route 健康：`redis_route_lookup_error_count`、`redis_route_stale_removed_count`、`redis_route_cleanup_error_count`。
- cross-instance resume：`redis_resume_append_count`、`redis_resume_replay_count`、`redis_resume_miss_count`、`redis_resume_permission_denied_count`。

如果这些指标显示 online route 失败，但 `PullInbox` 能拉到消息并 ACK 成功，说明在线唤醒退化但可靠投递未丢。
