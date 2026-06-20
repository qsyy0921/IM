# presence-service

状态：product-active。SDD v0.1 和 stage-switch review 已通过，第一实现切片
已同步 service-registry、proto、migration、六层 skeleton、`grpc` runtime、
Docker / Prometheus / Grafana，并通过最小 gRPC smoke。

定位：在线状态服务，负责用户在线、设备在线、输入中、最后在线时间和 presence
订阅 / 广播。

边界：

- push-gateway session registry 只服务在线路由，不是完整 presence 事实源。
- presence 状态是热状态 / 近实时投影，不作为消息投递或权限事实源。
- durable inbox 和 ACK 仍属于 delivery-service。
- Redis 可作为热状态候选，但状态过期、去抖和隐私策略必须在服务层定义。

Stage-switch 记录：`docs/runbook/stage-switch/presence-service.md`。

第一实现切片已落：

- 具体边界见 `docs/sdd/presence-service.md`。
- `UpdatePresence` / `GetPresence` / `UpdateTyping`。
- 先用显式 heartbeat / client update 证明 gRPC + PostgreSQL + visibility 边界；
  push-gateway session event consumer、SubscribePresence、stale scanner 和 outbox relay
  后续再补。
- 支持 privacy / contacts policy 过滤。
- smoke 证据：
  `docs/runbook/loadtest/presence-service/loadtest-report-20260620-presence-grpc-smoke.md`。
