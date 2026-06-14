# push-gateway

## 当前状态

- 已有 WebSocket notify、ACK 转发、slow session close、resume buffer、Redis route、cross-instance smoke。
- 只做在线唤醒，不拥有 durable inbox。
- 历史缺口通过 delivery-service `PullInbox` 兜底。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`，可观察低敏单实例 session / resume 聚合、Redis route / subscriber 聚合和 auth JWK 聚合。
- 当 `NEXUSIM_PUSH_AUTH_MODE=mock` 时，WebSocket 监听地址仅允许 loopback / RFC1918 私网；公网监听地址会在启动前直接失败，避免把本地 smoke 身份模式暴露到公网。
- 当 `NEXUSIM_PUSH_AUTH_MODE=hmac|jwt` 且 WebSocket 监听地址是公网地址时，若未启用入口 TLS / WSS，进程也会在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口。
- `delivery-consumer` 和 `identity-consumer` 仅对运行时错误做退避重试；`invalid frame` / `unsupported event` 仍保持 fail-closed，并在 worker 模式通过 `/debug/metrics` 暴露 consumer retry 快照。
- Redis route subscriber 对非取消运行时错误已改为退避重试，并在 `/debug/metrics` 暴露低敏 retry 快照；malformed payload 仍只记聚合计数，不会把 subscriber 进程打死。
- Redis route 已区分“publish 报错”和“publish 成功但 0 subscriber”两类远端失败，避免把 stale route 误记为远端已入队。
- Redis route 续约连续失败达到阈值后会主动踢掉本地 session，避免 route TTL 失效后仍长时间假装在线；客户端改走重连 + `PullInbox` fallback。

## 后续

- Redis 故障语义、跨实例 resume 强化、容量测试。
