# loadtest/hotgroup

`loadtest/hotgroup` 是热点群聊端到端业务压测 runner。它不替代
`loadtest/sendmessage` 的单服务基线，而是把 conversation / message / delivery
链路串起来验证群聊 fanout。

当前 v0.1 覆盖：

```text
CreateConversation(GROUP)
-> batch CreateMemberChange(JOIN)
-> SendMessage
-> wait delivery membership projection
-> wait user_inbox fanout or delivery_timeline_items
-> optional WebSocket conversation.subscribe / delivery.notify signal verification
-> sampled PullInbox
-> sampled AckDelivery
-> summary/report/users.jsonl
```

热点群准备状态：

```text
conversation-service fanout policy
-> timeline-service seq block allocator / lease status / gap marker repair operator
-> message-service SEQUENCER_BLOCK local seq block cache
-> delivery-service READ_FANOUT / BROADCAST_SIGNAL projection
-> push-gateway conversation signal notify
```

注意：本 runner 仍只记录压测事实，不负责自动修复 gap 或自动调整群规模策略。
正式三机压测前需要先重建最新 Docker 镜像并重新部署服务容器。

暂不覆盖：

```text
slow client active close
Redis route fault
member churn during send
delivery outage recovery
```

Dry-run：

```powershell
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --dry-run `
  --run-name hotgroup-review-dryrun `
  --group-size 100 `
  --sender-count 5 `
  --message-count 20
```

真实执行前必须启动 conversation-service、message-service、delivery-service、
push-gateway、message outbox relay、delivery timeline consumer、delivery outbox relay、
PostgreSQL 和 Kafka。

热点群 `BROADCAST_SIGNAL` 路径示例：

```powershell
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --run-name hotgroup-broadcast-push-smoke `
  --conversation-target 172.31.50.2:10496 `
  --message-target 172.31.50.2:10495 `
  --delivery-target 172.31.50.2:10497 `
  --push-url ws://172.31.50.2:10498/ws `
  --pg-dsn "postgres://nexusim:nexusim@172.31.50.2:5432/nexusim?sslmode=disable" `
  --group-size 61 `
  --sender-count 4 `
  --message-count 20 `
  --expect-fanout-mode BROADCAST_SIGNAL `
  --conversation-subscriber-count 3 `
  --require-conversation-notify `
  --require-delivery-outbox-drain
```

结果默认写入：

```text
H:\NexusIM\loadtest-results\<run-name>\
```

正式三机压测还要同步采集 Windows / Ubuntu / Mac 的 CPU 和内存曲线。原始
CSV / SVG 仍放在 H 盘结果目录，不写入仓库：

```powershell
.\tools\record-lab-resource-window.ps1 `
  -OutputDir H:\NexusIM\loadtest-results\<run-name>\lab-resource `
  -DurationSeconds 180 `
  -IntervalSeconds 1 `
  -UbuntuHost qsyy0921@172.31.50.2 `
  -IncludeMac `
  -MacHost qsyy0921@172.31.50.3
```

如果任一机器采样全失败，脚本默认失败，避免生成缺机器的误导性曲线。

容量判断优先使用压测相关进程 / 容器资源窗口，而不是整机汇总。采样必须在
压测开始前启动，压测结束后等待 30 秒再写 stop file，让 outbox / projection /
consumer 尾部工作也进入统计：

```powershell
$runName = "<run-name>"
$resourceDir = "H:\NexusIM\loadtest-results\$runName\lab-process-resource"
$stopFile = Join-Path $resourceDir "STOP"
New-Item -ItemType Directory -Force -Path $resourceDir | Out-Null
Remove-Item -LiteralPath $stopFile -Force -ErrorAction SilentlyContinue

$sampler = Start-Process powershell -PassThru -WindowStyle Hidden `
  -WorkingDirectory (Get-Location) `
  -ArgumentList @(
    "-NoProfile", "-ExecutionPolicy", "Bypass",
    "-File", ".\tools\record-lab-process-resource-window.ps1",
    "-OutputDir", $resourceDir,
    "-StopFile", $stopFile,
    "-IntervalSeconds", "1",
    "-UbuntuHost", "qsyy0921@172.31.50.2",
    "-IncludeMac",
    "-MacHost", "qsyy0921@172.31.50.3"
  )

# run hotgroup loadtest here

Start-Sleep -Seconds 30
New-Item -ItemType File -Force -Path $stopFile | Out-Null
Wait-Process -Id $sampler.Id
```

后续正式后端压测必须把 Ubuntu 服务端资源利用率作为有效性门槛之一：
若 Ubuntu 整机 CPU 长时间接近空闲，不能直接把低 achieved rate 写成服务端容量上限；
需要继续提高实际发压、拆分 runner、上调安全的 runtime profile，或定位 RPC / DB /
连接池等待，直到确认瓶颈已迁移到 Ubuntu 上的具体服务、PostgreSQL、Kafka、
Redis、网络或磁盘资源。报告必须同时给出三机资源曲线和 Prometheus / PostgreSQL
证据，避免只凭整机 CPU 判断。

三台机器资源充足时，正式压测默认采用分布式压力模型：Windows、Ubuntu、Mac
都应参与发压、subscriber-only 读取 shard、或资源采样 / 观测中的至少一种。除非
单机 runner 已证明能把 Ubuntu 服务端 CPU / IO / 网络打到瓶颈区，否则不要只用
Windows 单机 runner 下结论。send-only 阶段优先拆分多个 runner 共同调用
`SendMessage`；online signal 阶段优先把 subscriber shard 分散到三台机器，避免
客户端单机 JSON decode、WebSocket read 或 accounting 限制掩盖服务端能力。

发送压力由 `--message-rate` 控制全局目标速率，由 `--send-concurrency` 控制
并发 `SendMessage` worker 数。`--send-concurrency=0` 表示使用 `--sender-count`。
报告中的 `achieved_send_rate` 才是本轮真实发压速率；不要把 target
`--message-rate` 直接当成服务端 QPS。

Prepared-group send-only A/B:

```powershell
go run .\loadtest\hotgroup `
  --skip-setup `
  --tenant-id <existing-tenant> `
  --conversation-id <existing-conversation> `
  --group-size <same-group-size> `
  --sender-count <same-sender-count> `
  --message-count 5000 `
  --send-concurrency 256
```

`--skip-setup` reuses the deterministic hotgroup users and existing
conversation membership. It records the current `user_inbox` /
`delivery_timeline_items` baseline and waits for `baseline + this-run messages`,
so it can isolate per-message SendMessage / message timeline export behavior
without adding setup-time member boundary events.
