# push-gateway

## 当前状态

- 已有 WebSocket notify、`delivery.hide` 在线隐藏提示、ACK 转发、slow session close、resume buffer、Redis route、cross-instance smoke。
- 只做在线唤醒，不拥有 durable inbox。
- 历史缺口通过 delivery-service `PullInbox` 兜底。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`、Prometheus text `/metrics`、本地 alert rules / Grafana dashboard 原型，可观察低敏 session / resume、Redis route / subscriber、consumer worker、auth JWK 和 OTel trace config 聚合；默认 scrape target 为 `host.docker.internal:11913`，这是本地开发 / 面试演示观测，不是生产 SLO。
- 已补 first-stage OpenTelemetry WebSocket connection span，默认关闭；启用后可输出 stdout 或 OTLP gRPC trace，span 只记录低敏连接形态字段，不记录 tenant / user / device / session id。
- 当 `NEXUSIM_PUSH_AUTH_MODE=mock` 时，WebSocket 监听地址仅允许 loopback / RFC1918 私网；公网监听地址会在启动前直接失败，避免把本地 smoke 身份模式暴露到公网。
- 当 `NEXUSIM_PUSH_AUTH_MODE=hmac|jwt` 且 WebSocket 监听地址是公网地址时，若未启用入口 TLS / WSS，进程也会在启动前直接失败，避免把签名 token 暴露到 plaintext 公网入口。
- 转发 `AckDelivery` 到 delivery-service 时，只传播低敏 `trace_id/request_id`；不符合白名单的 correlation metadata 会在 RPC adapter 出口丢弃。
- `delivery-consumer` 和 `identity-consumer` 仅对运行时错误做退避重试；`invalid frame` / `unsupported event` 仍保持 fail-closed，并在 worker 模式通过 `/debug/metrics` 暴露 consumer retry 快照。
- Redis route subscriber 对非取消运行时错误已改为退避重试，并在 `/debug/metrics` 暴露低敏 retry 快照；malformed / incomplete payload 只记聚合计数，不会入队或打死 subscriber。
- Redis route 已区分“publish 报错”和“publish 成功但 0 subscriber”两类远端失败，避免把 stale route 误记为远端已入队。
- Redis route 续约连续失败达到阈值后会主动踢掉本地 session，避免 route TTL 失效后仍长时间假装在线；客户端改走重连 + `PullInbox` fallback。
- Resume buffer 重放已有回归测试固定 all-or-buffer-miss：新连接队列无法容纳全部待重放 notify 时，不做部分 replay，直接提示客户端用本地 cursor + `PullInbox` 校准。
- `loadtest/pushgateway/run-local-smoke.ps1` 的通用 helper 已拆到同目录 `run-local-smoke.helpers.ps1`，脚本复杂度回到预算线内。
- `loadtest/pushgateway` Go runner 已按 config / model / auth / scenario / util 同 package 拆分，避免后续 Redis route、slow-client、resume 和容量 smoke 继续堆进单个 `main.go`。
- `loadtest/pushgateway` summary 已新增 `capacity_summary` 派生字段，统一输出 duration、device/message/notify/ack/pull 计数和每秒速率；这是容量口径基础，不等于已经完成容量压测。
- 本地 smoke 已覆盖 Redis stop/start、Sentinel discovery、手动 failover、master-stop、quorum-loss fallback 和 network-partition fallback；network-partition 场景会断开 Sentinel 当前 master 的 Docker network，并验证 `delivery.notify` 超时后 `PullInbox + AckDelivery` 兜底。Redis Cluster 和生产级 HA 仍未完成。

## 后续

- 跨实例 resume 强化、基于 `capacity_summary` 的容量基线实跑、Redis Cluster / 生产 HA 设计。
