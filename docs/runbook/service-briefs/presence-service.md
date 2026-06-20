# presence-service

状态：future / SDD v0.1 draft pending。当前不得创建 `services/presence-service`
目录，直到完成 ADR 或 stage switch。

定位：在线状态服务，负责用户在线、设备在线、输入中、最后在线时间和 presence
订阅 / 广播。

边界：

- push-gateway session registry 只服务在线路由，不是完整 presence 事实源。
- presence 状态是热状态 / 近实时投影，不作为消息投递或权限事实源。
- durable inbox 和 ACK 仍属于 delivery-service。
- Redis 可作为热状态候选，但状态过期、去抖和隐私策略必须在服务层定义。

第一切片建议：

- `UpdatePresence` / `SubscribePresence` / `GetPresence`。
- 先消费 push-gateway session events 或显式 heartbeat。
- 支持 privacy / contacts policy 过滤。
