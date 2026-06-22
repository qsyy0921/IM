# push-gateway

## 当前状态

- 已有 WebSocket notify、`delivery.hide` 在线隐藏提示、ACK 转发、slow session close、resume buffer、Redis route、cross-instance smoke；只做在线唤醒，不拥有 durable inbox，历史缺口通过 delivery-service `PullInbox` 补拉。
- 已补 `/healthz`、`/readyz`、`/debug/metrics`、Prometheus text `/metrics`、本地 alert rules / Grafana dashboard 原型，可观察低敏 session / resume、Redis route / subscriber、consumer worker、auth JWK 和 OTel trace config 聚合；默认 scrape target 为 `host.docker.internal:11913`，这是本地开发 / 面试演示观测，不是生产 SLO。first-stage OpenTelemetry WebSocket connection span 默认关闭，启用后只记录低敏连接形态字段。
- 当 `NEXUSIM_PUSH_AUTH_MODE=mock` 时，WebSocket 监听地址仅允许 loopback / RFC1918 私网；公网监听地址会在启动前直接失败，避免把本地 smoke 身份模式暴露到公网。
- 当 `NEXUSIM_PUSH_AUTH_MODE=hmac|jwt` 且 WebSocket 监听地址是公网地址时，若未启用入口 TLS / WSS，进程也会在启动前直接失败；WebSocket TLS / mTLS 配置已有 TLS 1.2 minimum、client certificate identity allowlist positive path 和 unlisted identity rejection 单测，`check-local` 通过 `tools/check-grpc-tls-config-guards.ps1` 强制保留这些本地门禁。
- 转发 `AckDelivery` 到 delivery-service 时，只传播低敏 `trace_id/request_id`；不符合白名单的 correlation metadata 会在 RPC adapter 出口丢弃。
- `delivery-consumer` 和 `identity-consumer` 仅对运行时错误做退避重试；`invalid frame` / `unsupported event` 仍保持 fail-closed，并在 worker 模式通过 `/debug/metrics` 暴露 consumer retry 快照。
- Redis route subscriber 对非取消运行时错误已改为退避重试，并在 `/debug/metrics` 暴露低敏 retry 快照；malformed / incomplete payload 只记聚合计数，不会入队或打死 subscriber。
- Redis route 已区分“publish 报错”和“publish 成功但 0 subscriber”两类远端失败，避免把 stale route 误记为远端已入队。
- Redis route 续约连续失败达到阈值后会主动踢掉本地 session，避免 route TTL 失效后仍长时间假装在线；客户端改走重连 + `PullInbox` 补拉。
- Redis route 已支持 `NEXUSIM_PUSH_REDIS_MODE=cluster` 和 `NEXUSIM_PUSH_REDIS_CLUSTER_ADDRS` 第一版配置；route / resume 相关 multi-key pipeline 使用 Redis Cluster hash tag，identity revoke 查询避免跨 slot multi-key `EXISTS`，实现文件已按 core / route / resume 拆分。本地三节点 Redis Cluster topology smoke 已跑通 `cross-instance-resume`，证明 cluster client / key schema / route / resume 最小链路可用；本地 Redis Cluster node-stop recovery smoke 已停止 route key slot owner node，并验证 `delivery.notify` 超时后仍可通过 `PullInbox + AckDelivery` 恢复；本地六节点 Redis Cluster 自动 failover smoke 已停止 route key slot owner master，并验证 replica 提升后在线 `delivery.notify`、`PullInbox` 和 `AckDelivery` 仍可用；本地六节点 Redis Cluster 短容量基线已跑通 16 条消息、2 个 device、32 个 notify 和 2 个 ACK，但不代表生产级 Redis HA、长时间容量曲线或生产 sizing。
- 本地 Kafka consumer group rebalance smoke 已覆盖两个 `delivery-consumer` 进同一 group 后停止一个进程，Kafka 将 `im.delivery.events` 的 3 个 partition 稳定重新分配到剩余 consumer；这是本地 rebalance 观察，不代表持续 rebalance storm 或生产 SLO。
- 本地 Kafka consumer churn smoke 已覆盖 2 轮 delivery-consumer leave / rejoin、8 个 transition，Kafka group 每次都回到 `Stable` 且 3 个 partition 均已分配；这是本地 churn 观察，不代表高频 rebalance storm、消息处理连续性或生产 SLO。
- 本地 Kafka consumer churn probe smoke 已覆盖 8 个 transition 后写入 24 条合法 `delivery.inbox_item.created.v1` probe，producer ack 24，consumer group 每次 post-probe lag 回到 0；这是本地消息连续性观察，不代表高频 rebalance storm、在线 WebSocket 推送连续性或生产 SLO。
- 本地 Kafka KRaft repeated ISR flapping smoke 已覆盖 2 轮 broker stop/start：降级阶段 replicated probe topic 稳定到 `ISR=2` 且 `acks=all` probe 可写，恢复阶段回到 `ISR=3` 且 probe 继续可写；这是本地 flapping 观察，不代表生产 Kafka HA、长时间容量曲线或 exactly-once producer 语义。
- 本地 `kafka-go` producer in-flight broker-fault observation 已覆盖 broker stop/restore 窗口内 120 条 records：producer ack 120，consumer unique 120，missing ack 0，observed duplicate 0；这是本地观察，不代表 exactly-once producer 语义。
- Resume buffer 重放已有回归测试固定 all-or-buffer-miss：新连接队列无法容纳全部待重放 notify 时，不做部分 replay，直接提示客户端用本地 cursor + `PullInbox` 校准。
- `loadtest/pushgateway/run-local-smoke.ps1` 通用 helper、Redis fault helper 和 Go runner config / model / auth / scenario / util 已按同目录 / 同 package 拆分，避免后续 Redis route、slow-client、resume 和容量 smoke 继续堆进单个大文件。
- `loadtest/pushgateway` summary 已新增 `capacity_summary` 派生字段，统一输出 duration、device/message/notify/ack/pull 计数和每秒速率；runner 已支持 `--duration` / `--vus`，用于 full 场景长跑时循环发送消息并把 VU 映射为在线接收 device；本地 push-gateway stack 短基线已跑通 `full` 场景，clean summary 记录 `git_dirty=false`、1 个 device、1 条 message、1 个 notify、1 个 ACK、PullInbox 1 条、delivery_outbox published 2 条。尚未跑 30m push-gateway 长跑切片。
- `loadtest/pushgateway` 已新增并跑通 `redis-resume-negative` 真实进程 smoke，用于验证未知 resume token 被服务端替换并返回 `buffer_miss`、跨 device resume token 返回非重试 `PERMISSION_DENIED`、Redis resume buffer gap 返回 `buffer_miss` 后通过 `PullInbox + AckDelivery` 补拉；报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260616-push-gateway-redis-resume-negative-smoke.md`。
- 本地 smoke 已覆盖 Redis stop/start、Sentinel discovery、手动 failover、master-stop、quorum-loss recovery、network-partition recovery、三节点 Redis Cluster topology、Redis Cluster node-stop recovery 和六节点 Redis Cluster 自动 failover；network-partition 场景会断开 Sentinel 当前 master 的 Docker network，Cluster node-stop 场景会停止 route key slot owner node，Cluster failover 场景会停止 route key slot owner master 并等待 replica 提升。Redis Sentinel / Cluster smoke summary 已有离线 validator。生产级 Redis HA / 容量仍未完成。

## 后续

- 生产级 Redis HA 设计；长时间容量曲线和生产 sizing。
