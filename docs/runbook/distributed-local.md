# NexusIM Local Distributed Runbook

本文说明当前本地可运行的 NexusIM 分布式拓扑。它不是生产部署手册，而是面向开发、演示和面试复盘的最小分布式 smoke 入口。

## 1. 当前分布式边界

当前已落地的真实微服务 / 网关：

| 组件 | 当前职责 | 本地进程模式 |
| --- | --- | --- |
| `conversation-service` | 会话、成员、成员变更 saga、`GetSendContext` | `grpc` |
| `message-service` | `SendMessage` 本地事务、message outbox relay | `grpc`、`outbox-relay` |
| `delivery-service` | timeline projection、`PullInbox`、`AckDelivery`、delivery outbox relay | `grpc`、`timeline-consumer`、`outbox-relay` |
| `push-gateway` | WebSocket 在线连接、`im.delivery.events` 消费、Redis route 在线路由 | `ws`、`delivery-consumer` |

基础设施：

```text
PostgreSQL -> 交易事实源 / inbox / outbox / cursor
Kafka      -> conversation.timeline.events / im.delivery.events
Redis      -> push-gateway online route
```

## 2. 最小分布式 smoke 链路

```text
client WebSocket
-> push-gateway-ws

SendMessage
-> message-service grpc
-> PostgreSQL local transaction
-> message_outbox
-> message-service outbox-relay
-> Kafka conversation.timeline.events
-> delivery-service timeline-consumer
-> user_inbox + delivery_outbox
-> delivery-service outbox-relay
-> Kafka im.delivery.events
-> push-gateway delivery-consumer
-> Redis route / PubSub
-> push-gateway-ws local fanout
-> WebSocket delivery.notify
-> delivery-service PullInbox
-> delivery.ack frame
-> delivery-service AckDelivery
-> delivery.ack.ok
```

关键点：

- `push-gateway-ws` 和 `push-gateway-consumer` 是两个不同进程。
- WebSocket 连接只在 `push-gateway-ws`。
- Kafka `im.delivery.events` 只由 `push-gateway-consumer` 消费。
- 如果 Redis route / PubSub 不工作，客户端收不到 `delivery.notify`。
- 可靠投递不依赖 Redis：客户端最终以 `PullInbox` 和 `AckDelivery` 为准。

## 3. 运行命令

先启动本地基础设施：

```powershell
.\tools\local-up.ps1
```

如需本地 PostgreSQL failover 拓扑，改用：

```powershell
.\tools\local-up-postgres-ha.ps1
```

运行分布式 smoke：

```powershell
. .\tools\go-env.ps1
.\tools\local-distributed-smoke.ps1
```

常用参数：

```powershell
.\tools\local-distributed-smoke.ps1 `
  -RunName nexusim-distributed-smoke-YYYYMMDD-HHMMSS `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RedisAddr 127.0.0.1:6379
```

如果二进制已构建：

```powershell
.\tools\local-distributed-smoke.ps1 -SkipBuild
```

本地 PostgreSQL failover smoke：

```powershell
.\tools\local-postgres-failover-smoke.ps1 -SkipBuild
```

该脚本会：

```text
1. 启动三节点 `postgresql-repmgr` + `pgpool` 本地写入口；
2. 通过 pgpool `15432` 端口应用 message / conversation / delivery core migrations；
3. 用同一个 pgpool DSN 跑一遍 full distributed smoke；
4. 停止当前 primary 容器，等待 pgpool 指向新 primary 且连续写探针成功；
5. 继续用同一个 pgpool DSN 再跑一遍 full distributed smoke；
6. 输出 before/after summary 和 primary 切换结果。
```

如需释放 PG HA 资源：

```powershell
.\tools\local-down-postgres-ha.ps1
```

同步 Mac 专用 smoke checkout：

```powershell
.\tools\sync-mac-distributed-smoke.ps1
```

该脚本会：

```text
1. Windows 本地用当前 HEAD 生成 Git bundle；
2. 通过 172.31.50.2 有线 SSH/scp 传到 Mac；
3. 重建 /Users/qsyy0921/Desktop/IM/_local/distributed-smoke；
4. 从 Windows 交叉编译 darwin/arm64 push-gateway 和 pushgateway-smoke；
5. 通过有线 scp 把二进制放到 Mac checkout 的 bin/darwin-arm64；
6. 在 Mac 上跑一次 NEXUSIM_PUSH_GATEWAY_MODE=noop 验证二进制能启动。
```

脚本默认拒绝操作非 `/Users/qsyy0921/Desktop/IM/_local/distributed-smoke` 的 Mac 路径，避免覆盖 Mac 上已有的 `/Users/qsyy0921/Desktop/IM` 工作区。

## 4. 结果位置

大结果文件保存到机械盘：

```text
H:\NexusIM\loadtest-results\<run-name>\pushgateway-summary.json
```

报告归档在仓库：

```text
docs/runbook/loadtest/push-gateway/
```

当前系统级分布式 smoke 原始结果：

```text
H:\NexusIM\loadtest-results\nexusim-distributed-smoke-redis-cleanup-20260609-193331\pushgateway-summary.json
```

该 run 使用 clean commit `29b8cc6`，`git_dirty=false`，WebSocket gateway 与 delivery consumer gateway 分离，通过 Redis route 收到 `delivery.notify`，随后 `PullInbox` 返回 1 条 inbox，`delivery.ack.ok` 推进 cursor 到 seq `2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。

当前 Redis route 故障 smoke 原始结果：

```text
H:\NexusIM\loadtest-results\push-gateway-redis-fault-smoke-20260609-195200\pushgateway-summary.json
```

该 run 使用 clean commit `074902b`，`git_dirty=false`。runner 在 WebSocket session 已注册后停止 `nexusim-redis`，随后发送一条消息；在线 `delivery.notify` 不作为成功条件，客户端通过 `PullInbox` 拉到 seq `2`，恢复 Redis 后重连并通过 `delivery.ack.ok` 推进 cursor 到 seq `2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。

当前 Win-Mac Docker route smoke 原始结果：

```text
H:\NexusIM\loadtest-results\push-gateway-win-mac-redis-smoke-20260609-210034\pushgateway-summary.json
```

该 run 使用 clean commit `8c322fc`，`git_dirty=false`。拓扑为：Windows 运行 PostgreSQL / Kafka / Redis / 核心业务进程 / `push-gateway delivery-consumer`，Windows Docker 运行 `nexusim/delivery-service:local` gRPC，Mac Docker 运行 `nexusim/push-gateway:local` WebSocket gateway。Windows runner 通过 `ws://172.31.50.2:11598` 连接 Mac；Mac gateway 通过有线 `172.31.50.1:6379` 使用 Redis route，并通过 `172.31.50.1:11597` 回调 Windows Docker delivery-service ACK。结果：收到 seq `2` 的 `delivery.notify`，`PullInbox item_count=1/max_seq=2`，`delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。

当前 Win-Mac Docker cross-instance resume smoke 原始结果：

```text
H:\NexusIM\loadtest-results\push-gateway-win-mac-cross-instance-resume-20260609\pushgateway-summary.json
```

该 run 使用 clean commit `b8d8f92`，`git_dirty=false`。拓扑为：Windows 运行 PostgreSQL / Kafka / Redis / 核心业务进程 / `push-gateway delivery-consumer` / `push-gateway ws-reconnect`，Windows Docker 运行 `nexusim/delivery-service:local` gRPC，Mac Docker 运行 `nexusim/push-gateway:local` 首连 WebSocket gateway。客户端第一次连接 `ws://172.31.50.2:11598`，收到 seq `2` 的 `delivery.notify` 后在 ACK 前断开；随后携带同一 `resume_token` 和 `last_received=1` 重连到 Windows `ws://127.0.0.1:11599`，命中 Redis-backed resume buffer，replay 同一条 `event_id/message_id/conversation_seq` 的 notify。结果：consumer gateway `redis_resume_append_count=1`，重连 gateway `redis_resume_replay_count=1 / redis_resume_miss_count=0`，随后 `PullInbox item_count=1/max_seq=2`，`delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。

当前 Redis Sentinel discovery route / resume smoke 原始结果：

```text
H:\NexusIM\loadtest-results\push-gateway-redis-sentinel-route-resume-final-20260609\pushgateway-summary.json
```

该 run 使用 clean commit `7bc35a5`，`git_dirty=false`。拓扑为本地三 Redis / 三 Sentinel，push-gateway 使用 `NEXUSIM_PUSH_REDIS_MODE=sentinel` 和 `mymaster` 发现 master；Sentinel 返回 master `172.31.50.1:6380`，启动脚本已验证该地址可从宿主机 TCP 连接、可从 Sentinel 容器内 `PING`。结果：consumer gateway `redis_resume_append_count=1`、`redis_route_remote_publish_call_count=1`、`redis_route_remote_publish_error_count=0`，重连 gateway `redis_resume_replay_count=1 / redis_resume_miss_count=0`，随后 `PullInbox item_count=1/max_seq=2`，`delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。这证明 Sentinel discovery 正常路径可用，不证明 master failover / Redis HA。

当前 PostgreSQL failover smoke 原始结果：

```text
H:\NexusIM\loadtest-results\postgres-failover-smoke-20260614f\postgres-failover-summary.json
```

该 run 使用本地工作树上的 PostgreSQL HA 脚本切片，写入口固定为 `postgres://nexusim:nexusim@127.0.0.1:15432/nexusim?sslmode=disable`，执行前后两次 distributed smoke：

```text
before primary = postgres-ha-0
stop current primary container = nexusim-postgres-ha-0
after primary  = postgres-ha-1
before: delivery.notify seq=2, PullInbox item_count=1/max_seq=2, delivery.ack.ok last_received_seq=2
after:  delivery.notify seq=2, PullInbox item_count=1/max_seq=2, delivery.ack.ok last_received_seq=2
```

这证明本地 `repmgr + pgpool` 稳定写入口切主后，`CreateMemberChange -> SendMessage -> delivery.notify -> PullInbox -> delivery.ack.ok` 最小链路仍可跑通。它不代表生产级 PostgreSQL HA，不覆盖 split-brain、quorum、防抖、自动回切、in-flight transaction continuity 或跨机存储故障。

前一轮脚本验证结果：

```text
H:\NexusIM\loadtest-results\push-gateway-win-mac-redis-smoke-20260609-205219\pushgateway-summary.json
```

该 run 使用工作区 dirty commit `3c07305`，用于验证脚本和双机 Docker 路径，不作为 clean 性能基线。拓扑为：Windows 运行 PostgreSQL / Kafka / Redis / 核心业务进程 / `push-gateway delivery-consumer`，Mac Docker 运行 `nexusim/push-gateway:local` WebSocket gateway。Windows runner 通过 `ws://172.31.50.2:11598` 连接 Mac，Mac gateway 通过 `172.31.50.1:6379` 使用 Redis route，通过 `172.31.50.1:11597` 回调 Windows delivery-service ACK。结果：收到 seq `2` 的 `delivery.notify`，`PullInbox item_count=1/max_seq=2`，`delivery.ack.ok last_received_seq=2`，`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。

## 5. Win/Mac 双机分布式模拟计划

当前直连网络规划：

```text
Windows wired: 172.31.50.1/24
Mac wired:     172.31.50.2/24
Wi-Fi:         两端继续用于上网和下载依赖
Proxy:         两端各自对外代理端口均为本机 127.0.0.1:7890
```

后续双机 smoke / 小压测优先使用 `172.31.50.*`，避免走随身 Wi-Fi 的 `192.168.0.*` 管理网段。
只要 Win-Mac 之间能通过网线直连，服务间地址、SSH、文件传输和 smoke callback 都优先使用 `172.31.50.*`；Wi-Fi 只负责访问互联网和下载依赖。
GitHub / Docker / Go module 等必须访问外网的下载才使用各自机器的本机 `127.0.0.1:7890` 代理：Windows 的 `127.0.0.1:7890` 只给 Windows 本机外网访问使用，Mac 的 `127.0.0.1:7890` 只给 Mac 本机外网访问使用。它们不是 Win-Mac 服务间代理，也不能给另一台机器当外网代理。只要数据可以在 Windows 和 Mac 之间直接传输，就必须走有线 `172.31.50.*`，不要绕 GitHub / 云盘 / 外网代理，避免消耗流量。

用户当前希望用两台机器模拟多节点，而不是继续做重型单机硬件矩阵。建议资源切分：

| 机器 | 模拟节点 | 建议资源上限 | 初始用途 |
| --- | --- | --- | --- |
| Windows | `win-node-a` | 4 CPU / 8GB | PostgreSQL、Kafka、Redis、message/conversation 核心写链路 |
| Windows | `win-node-b` | 4 CPU / 8GB | delivery-service、push-gateway consumer、局部压测器 |
| Mac | `mac-node-a` | 4 CPU / 4GB | push-gateway WebSocket 实例、Redis route 跨机验证 |
| Mac | `mac-node-b` | 4 CPU / 4GB | 轻量 load generator / 第二 gateway 实例 |

第一阶段不需要真的把每个服务都拆进独立容器。更稳的顺序是：

1. 先在 Mac 配好 Docker / Go / repo 路径，确认 `docker version`、`go test ./...` 或必要 smoke runner 可用。
2. 在 Windows 保留 PostgreSQL / Kafka / Redis，Mac 运行一个或两个 `push-gateway` 实例，使用 `172.31.50.1` 访问 Windows 基础设施。
3. 跑跨机器 route smoke：Windows consumer gateway -> Redis route -> Mac WebSocket gateway，验证 `delivery.notify -> PullInbox -> ACK`。
4. 再考虑用 Docker Compose profiles / resource limits 把服务按 `win-node-a/win-node-b/mac-node-a/mac-node-b` 分组。

当前 Mac 准备状态：

```text
172.31.50.2:22 reachable
192.168.0.182:22 reachable
Windows -> Mac SSH key auth: OK
Docker CLI: OK
Docker Desktop: 29.5.3
Docker resource pool observed through Docker info: 8 CPU / about 8GB memory
Mac repo path: /Users/qsyy0921/Desktop/IM exists
```

Windows 当前公钥已加入 Mac 的 `/Users/qsyy0921/.ssh/authorized_keys`。Windows 侧可用：

```powershell
Get-Content $env:USERPROFILE\.ssh\id_ed25519.pub
```

Mac 侧 Docker CLI 已加入用户级 PATH，可通过 SSH 验证：

```bash
docker version
docker info
```

Windows 侧可用脚本复查 Mac Docker Desktop 配置：

```powershell
.\tools\check-mac-docker-desktop.ps1
```

当前验证结果：

```text
docker_cli=Docker version 29.5.3
docker_context=desktop-linux
cpus=8
memory_mib=8192
swap_mib=1024
proxy_http=http://127.0.0.1:7890
proxy_https=http://127.0.0.1:7890
proxy_exclude includes 172.16.0.0/12
mac_docker_desktop_config=OK
```

后续如果要在 Mac 上模拟两个节点，不再改 Docker Desktop 全局资源池；直接对容器设置资源上限：

```text
mac-node-a: --cpus 4 --memory 4g
mac-node-b: --cpus 4 --memory 4g
```

注意：Mac 的 `/Users/qsyy0921/Desktop/IM` 当前不是由 Windows 侧管理的干净工作区，已有本地 ahead / untracked 文件。后续同步代码时不能强行 reset；优先选择：

```text
1. Windows 本地生成 Git bundle；
2. 通过 `scp` / SSH 走 `172.31.50.2` 有线传到 Mac；
3. 在 Mac 上从 bundle clone / fetch，避免 Mac 直接访问 GitHub；
4. 若 Mac `Desktop/IM` 不能 fast-forward，则使用 `/Users/qsyy0921/Desktop/IM/_local/distributed-smoke` 作为干净 smoke checkout。
```

当前已采用第 4 种方式，`/Users/qsyy0921/Desktop/IM/_local/distributed-smoke` 是可重建的专用 smoke checkout；Mac 桌面根目录不再散放 NexusIM bundle 或 smoke 目录，bundle 归档到 `/Users/qsyy0921/Desktop/IM/_local/artifacts/bundles`。

四个业务服务镜像已采用本地构建方式准备，避免外网拉取业务镜像：

```powershell
.\tools\sync-mac-service-docker-images.ps1
```

该脚本在 Windows 构建 `linux/amd64` 镜像，在 Mac 构建 `linux/arm64` 镜像，镜像名为：

```text
nexusim/conversation-service:local
nexusim/message-service:local
nexusim/delivery-service:local
nexusim/push-gateway:local
```

业务镜像基于 `scratch` 和交叉编译后的静态 Go 二进制，不需要从 Docker Hub 拉基础层；Windows -> Mac 的二进制传输走有线 `172.31.50.2`。

压测原始结果继续放机械盘：

```text
H:\NexusIM\loadtest-results
```

仓库内只保存 Markdown 报告、脚本和小体积 summary 链接。

## 6. 面试讲法

可以说：

```text
我把 IM 系统拆成 message / conversation / delivery / push-gateway 四个真实进程边界。
消息写入只提交 PostgreSQL 本地事务和 outbox，不直接发 Kafka。
delivery-service 从 timeline 事件构建 user_inbox，再写 delivery_outbox。
push-gateway 只消费 delivery 事件做在线唤醒，WebSocket 连接和 Kafka consumer 可以落在不同 gateway 实例，通过 Redis route 找到目标连接。
可靠恢复由 PullInbox 和 AckDelivery 保证，Redis/WebSocket 只负责在线提示。
```

不要说：

```text
已经完成生产级多机部署和完整容量验证。
```

更准确的限制：

```text
当前是本地多进程分布式 smoke。它证明服务边界、outbox、Kafka、durable inbox、Redis route、WebSocket notify 和 ACK 能串起来。
生产级还需要真实鉴权、Kubernetes 部署、Redis HA / Sentinel / Cluster、跨实例慢连接组合验证、正式 metrics、容量和故障演练。
```

## 7. 已知缺口

- Redis route 已做一次真实 stop/start fault smoke，证明 online notify 可丢但 `PullInbox + AckDelivery` 可恢复；push-gateway 已在三 Redis / 三 Sentinel 拓扑上跑通 Sentinel discovery、手动 failover 和 master-stop recovery smoke，但尚未跑 quorum 异常 / 网络分区，因此仍不是 Redis HA、Redis Cluster 或生产级高可用结论。
- PostgreSQL 当前已补本地 `repmgr + pgpool` failover smoke，证明同一个 pgpool DSN 在 primary 切换前后仍可跑通最小分布式链路；但这仍不是 Patroni / etcd / 云托管 PostgreSQL 的生产级 HA 验收，也不覆盖 split-brain、quorum、防抖、自动回切或 in-flight transaction continuity。
- Redis route 已有 TTL 续期和后台 stale route cleanup；异常进程退出后 session route 仍依赖 TTL 过期，user route set 中的 stale 成员由 lookup / cleanup loop 移除。
- `push-gateway` Redis-backed cross-instance resume buffer 已有本机跨进程 smoke 和 Win-Mac Docker smoke；跨实例 replay miss、Redis error 或 token mismatch 时仍必须 fallback `PullInbox`。
- `push-gateway` `/debug/metrics` 仍是本地 smoke 调试端点，不是正式 Prometheus 指标。
- 真实生产部署还未接入 Kubernetes / service discovery / mTLS / OTel。
- Mac Docker CLI / SSH 已可用；双机 Docker Compose profile 尚未完成配置和验证。Mac `Desktop/IM` 有本地变更，后续跨机 smoke 前需选择 fast-forward 更新或新建干净 smoke checkout。
