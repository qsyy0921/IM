# Hot Group Loadtest Plan

本目录用于规划热点群聊业务压测。当前已有 `loadtest/hotgroup` runner，可生成
用户模型 dry-run，并在完整本地服务栈可用时执行：

```text
CreateConversation -> batch CreateMemberChange(JOIN)
-> SendMessage
-> delivery membership projection / user_inbox fanout 或 conversation signal
-> 可选 WebSocket conversation subscriber
-> sampled PullInbox / AckDelivery
```

本文继续冻结场景、指标和面试口径，避免只用单接口 QPS 代替真实 IM 业务压测。

SendMessage 发压模型：

```text
--sender-count       生成多少个业务 sender 用户
--send-concurrency  并发 SendMessage worker 数；0 表示使用 sender-count
--message-rate      全局目标投递速率，不等于最终达成 QPS
```

正式压测报告必须同时记录 `message_rate` 和 `achieved_send_rate`。如果
`achieved_send_rate` 明显低于 `message_rate`，先排查 runner 发压能力、gRPC
连接、请求超时和客户端机器资源，不要直接把 target rate 当成服务端容量上限。

## 最新压测记录

| 报告 | 结论 |
| --- | --- |
| `loadtest-report-20260628-hotgroup-relay-bottleneck.md` | 记录 delivery outbox relay 优化前后对比：旧 20 人群在 50/100/150 QPS 卡在 `delivery_outbox` drain；修正为 conversation-sharded relay 后，100 人群 50/100/150 QPS 均能在等待窗口内完成 `user_inbox` 和 `delivery_outbox` drain；200 QPS 暴露下一瓶颈已转移到 delivery timeline projection / `user_inbox` fanout。 |
| 2026-06-30 pre-commit diagnostic | 200 人 / 500 消息 / 16 sender 中等规模诊断曾暴露 `SEQUENCER_BLOCK` 后成员 JOIN 未接 timeline-service 的缺口；修复后 dirty-run 可完成 `BROADCAST_SIGNAL`，`delivery_outbox_pending=0`、Kafka lag=0。正式报告必须用 clean commit 重跑。 |
| `loadtest-report-20260630-hotgroup-clean-redeploy.md` | clean commit `d13bff6c` 重建 / redeploy 后，61 人 / 20 消息、200 人 / 500 消息、500 人 / 1000 消息三档均通过；最大档产生 50000 条 conversation signal，`user_inbox_rows=0`、`delivery_outbox_pending=0`、Kafka lag=0。 |
| `loadtest-report-20260630-hotgroup-message-outbox-relay.md` | message-service outbox relay 已支持 conversation-sharded multi-worker batch publish；1000 人 / 4000 消息 / 800 msg/s 通过，message / delivery outbox 均无积压；随后 clean commit `01b2a70` 完成 READ_FANOUT 6000 人 / 100 subscriber 阶梯复压，最高目标 8000 msg/s、500000 条 conversation signal，outbox / Kafka 均无积压。 |
| `hotgroup-analysis-20260630-readfanout-clean.md` | 由 `tools/analyze-hotgroup-loadtest.ps1` 自动汇总 clean commit `01b2a70` 的 6 档 READ_FANOUT 结果；当前分类为 `online-signal-drain`，证据是 outbox / Kafka 已追平但 500000 条 signal 最慢读完约 176s。 |
| `hotgroup-metrics-window-20260630-readfanout-clean-8000qps.md` | 由 `tools/record-hotgroup-metrics-window.ps1` 采集最高档 Prometheus 时间窗口；核心 4 个 scrape target 全部 up，`SendMessage p99` 约 21ms，`delivery_outbox_pending` 峰值 2258 后归零，push writer / Redis route 指标有数据，slow eviction 为 0。 |
| `hotgroup-analysis-20260701-readfanout-subscriber-step.md` | clean commit `7bff4f3` 的 200 subscriber 阶梯与上一轮 100 subscriber 对比：同为 6000 人 / 5000 消息 / 8000 msg/s，signal 从 500000 增至 1000000，最慢 drain 从 176.554s 增至 349.903s，drain rate 约 2.86k signals/s，继续分类为 `online-signal-drain`。 |
| `hotgroup-metrics-window-20260701-readfanout-200sub.md` | 200 subscriber run 的 Prometheus 低敏窗口：核心 4 个 scrape target 全部 up，`SendMessage p99` 约 21ms，`delivery_outbox_pending` 峰值 2233 后归零，push connected sessions 达到 200，slow eviction 为 0。 |
| `hotgroup-metrics-window-20260701-readfanout-400sub.md` | clean commit `233d695` 的 400 subscriber run 继续通过：6000 人 / 5000 消息 / 8000 msg/s 产生 2000000 条 signal，最慢 drain 704.631s，drain rate 约 2.84k signals/s；Prometheus 窗口内核心 target up、`delivery_outbox_pending` 峰值 2284 后归零、push connected sessions 达到 400、slow eviction 为 0。 |
| `hotgroup-metrics-window-20260701-readfanout-400sub.md` push attribution update | 同一窗口已补 WebSocket writer / Redis route per-event 归因：整窗口 `frame_write_success` 约 200.97 万、`delivery_notify_write_success` 约 200.89 万、`redis subscriber_enqueued` 约 200.89 万，writer / delivery notify / Redis subscriber error 与 eviction 均为 0；下一步瓶颈定位应聚焦写出 / 读取 drain 能力，而不是 Redis 路由失败或 WebSocket 写失败。 |
| `hotgroup-multirunner-analysis-20260701-400sub.md` | clean commit `9e7d4f9` 的 4 runner shard 对照：coordinator 发送 6000 人 / 1000 消息 / 8000 msg/s，4 个 `subscriber-only` shard 共 400 subscriber 读取 400000 条 signal；按首帧到末帧计算总 drain rate 约 2852 signals/s，与单 runner 400 subscriber baseline 约 2840 signals/s 基本一致，说明瓶颈不只是单个 runner JSON decode / accounting。 |
| `hotgroup-push-fanout-optimization-20260701.md` | 第一轮 push-gateway online signal drain 代码级优化记录：memory registry fanout 改为锁内快照、锁外写出，queue full 时精确回锁驱逐仍注册 session；clean commit `4bc4a30` redeploy 后 400 subscriber + 4 shard 复压显示 drain rate 约 2891.8 signals/s，仅比单 runner baseline 约 2839.888 signals/s 高约 1.8%，瓶颈未迁移。 |
| `hotgroup-multirunner-analysis-20260701-pushfanout-400sub.md` | clean commit `4bc4a30` 的 registry fanout 快照优化复压：6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber，message / delivery outbox pending=0，400000 条 signal 全部读完，当前瓶颈仍是 `online-signal-drain`。 |
| `hotgroup-metrics-window-20260701-pushfanout-clean-400sub.md` | registry fanout 快照优化复压的 Prometheus 窗口：核心 target up，`delivery_outbox_pending` 峰值 140 后归零，push connected sessions 达到 400，writer / Redis subscriber error 和 eviction 均为 0。 |
| `hotgroup-multirunner-analysis-20260701-pushpreenc-400sub.md` | clean commit `d8d78fd` 的 WebSocket payload 预编码优化复压：6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber，message / delivery outbox pending=0，400000 条 signal 全部读完，drain rate 约 2863.092 signals/s；相比单 runner baseline 约 2839.888 signals/s 仅约 0.8% 提升，也未超过上一轮 registry lock 优化，当前瓶颈仍是 `online-signal-drain`。 |
| `hotgroup-metrics-window-20260701-pushpreenc-clean-400sub.md` | payload 预编码优化复压的 Prometheus 窗口：核心 target up，`delivery_outbox_pending` 持续为 0，push connected sessions 达到 400，writer / Redis subscriber error 和 eviction 均为 0。 |
| `hotgroup-multirunner-analysis-20260701-writerdur-400sub.md` | clean commit `4f45519` 的 WebSocket writer duration 复压：6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber，message / delivery outbox pending=0，400000 条 signal 全部读完，drain rate 约 2876.698 signals/s，仍未离开旧区间。 |
| `hotgroup-metrics-window-20260701-writerdur-clean-400sub.md` | writer duration 窗口显示 `delivery_notify` write p95 / p99 约 0.345ms / 0.499ms、avg 约 0.125ms、max 约 10.056ms，writer / Redis subscriber error 与 eviction 为 0；下一步应定位 Redis subscriber 本地 fanout / enqueue 调度、writer goroutine 节奏和 runner 读取背压。 |
| `hotgroup-multirunner-analysis-20260701-redisfanout-400sub.md` | clean commit `6099ecd` 的 Redis subscriber fanout duration 复压：6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber，message / delivery outbox pending=0，400000 条 signal 全部读完，drain rate 约 2883.976 signals/s，仍在旧区间。 |
| `hotgroup-metrics-window-20260701-redisfanout-clean-400sub.md` | Redis subscriber fanout duration 窗口显示 WebSocket `delivery_notify` write p95 / p99 约 0.406ms / 0.63ms，但 Redis subscriber conversation signal fanout/enqueue 整窗口 p95 / p99 约 56.14ms / 91.228ms，5m last p95 / p99 约 60.263ms / 92.053ms；下一步应做 fanout worker / shard queue，而不是继续调单次 WebSocket write。 |
| `hotgroup-multirunner-analysis-20260701-writerqueue-400sub.md` | clean commit `fedb5f43` 的 WebSocket writer queue latency / batch drain 复压：6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber，400000 条 signal 全部读完，drain rate 约 2884.066 signals/s，仍未突破旧区间。 |
| `hotgroup-metrics-window-20260701-writerqueue-clean-400sub.md` | writer queue 窗口显示 `delivery_notify` queue p95 / p99 约 4.665ms / 4.942ms，write p95 / p99 约 0.383ms / 0.587ms，但 worker fanout p95 / p99 仍约 57.759ms / 92.241ms；下一步应评估 conversation-local fanout buckets，而不是继续调 writer queue。 |
| `hotgroup-multirunner-analysis-20260701-fanoutbuckets-400sub.md` | clean commit `a15e0ad` 的 conversation-local fanout buckets 复压：6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber，400000 条 signal 全部读完，drain rate 约 2874.378 signals/s，未突破旧区间。 |
| `hotgroup-metrics-window-20260701-fanoutbuckets-400sub.md` | fanout buckets 窗口显示 `delivery_notify` queue p95 / p99 约 4.616ms / 4.931ms，write p95 / p99 约 0.383ms / 0.574ms，Redis subscriber fanout p95 / p99 约 54.133ms / 90.827ms；下一步应评估持久 bucket worker、跨 push 实例分摊订阅或超大房间 pull-first 策略。 |
| `hotgroup-multirunner-analysis-20260701-multiws-400sub.md` | clean commit `4be4b2d` 的 4 个 push-gateway ws 实例拓扑复压：400 subscriber 按 100 / 100 / 100 / 100 分散到 4 个 ws 端口，400000 条 signal 全部读完，drain rate 约 2822.479 signals/s，低于单 ws fanout-buckets baseline 约 2874.378 signals/s；说明简单多开 ws 进程不是当前瓶颈解。 |
| `hotgroup-metrics-window-20260701-multiws-400sub.md` | multi-ws 窗口显示 4 个 push debug target 均 up，`delivery_notify` queue p95 / p99 约 3.703ms / 4.742ms，write p95 / p99 约 0.425ms / 0.769ms，Redis subscriber fanout p95 / p99 约 69.014ms / 93.803ms；writer / Redis error、queue-full 和 slow eviction 均为 0。 |
| `hotgroup-push-fanout-optimization-20260701.md` pull-first sampled signal update | 已实现显式 `NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY` 和 runner `--conversation-signal-sample-every`。默认 `1` 保持全量 signal；大群可显式采样在线唤醒，并用 PullInbox 追平 durable timeline。后续 sample=10 clean Docker 复压和 5000 消息扩大复压均已记录在本目录。 |
| `hotgroup-multirunner-analysis-20260701-sample10-400sub.md` | clean commit `bac71c65` 的 pull-first sampled online signal 复压：delivery-consumer 和 4 个 ws 实例均配置 `NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY=10`；6000 人 / 1000 消息 / 8000 msg/s / 400 subscriber 只发出并读完 40000 条 signal，span 25.243s；SendMessage / PullInbox / ACK 成功，message / delivery outbox pending=0。 |
| `hotgroup-metrics-window-20260701-sample10-400sub.md` | sample=10 窗口显示核心 target up，`delivery_outbox_pending` 峰值 267 后归零，Redis subscriber message/window 约 407、enqueued/window 约 40720，符合 100 sampled seq * 4 ws * 100 subscribers；`delivery_notify` write p95 / p99 低于 1ms，说明减少在线 frame 总量能显著缩短 drain span，但该结果不是生产容量上限。 |
| `hotgroup-multirunner-analysis-20260701-sample10-400sub-5000msg.md` | sample=10 扩大消息数复压：6000 人 / 5000 消息 / 8000 msg/s / 400 subscriber 产生并读完 200000 条 signal，span 138.555s；SendMessage p95 / p99 为 18.103ms / 20.914ms，PullInbox p95 为 23.874ms，message / delivery outbox pending=0。相比 1000 消息 sample run 的 40000 signal / 25.243s，drain rate 从约 1584.587 降至 1443.474 signals/s，瓶颈仍在 online signal drain。 |
| `hotgroup-metrics-window-20260701-sample10-400sub-5000msg.md` | 5000 消息 sample=10 窗口显示核心 target up，`delivery_outbox_pending` 峰值 1763 后归零，push writer / Redis subscriber error、queue-full 和 eviction 均为 0；`delivery_notify` write p95 / p99 约 0.458ms / 0.87ms，queue p95 / p99 约 3.991ms / 4.799ms，而 Redis subscriber conversation fanout p95 / p99 约 54.541ms / 90.908ms，说明放大后仍需优化/重构本地 signal fanout 或进一步减少在线 signal 总量。 |
| `hotgroup-multirunner-analysis-20260701-fanoutpolicy-400sub-5000msg.md` | clean commit `37b575e5` 将 push-gateway signal cadence 从全局 sample knob 收敛为 fanout-mode policy；default=1、READ_FANOUT=10、BROADCAST_SIGNAL=10 复压在 6000 人 / 5000 消息 / 8000 msg/s / 400 subscriber 下读完 200000 条 sampled signal，span 141.504s，message / delivery outbox pending=0。该结果证明 room policy 边界生效，但 READ_FANOUT=10 的性能仍与 global sample=10 同量级。 |
| `hotgroup-metrics-window-20260701-fanoutpolicy-400sub-5000msg.md` | fanout-mode policy 窗口显示核心 target up，`delivery_outbox_pending` 峰值 1721 后归零，writer / Redis subscriber error、queue-full 和 eviction 均为 0；`delivery_notify` write p95 / p99 约 0.465ms / 0.893ms，Redis subscriber conversation fanout window p95 / p99 约 39.944ms / 84.614ms。下一步应做 adaptive cadence 控制面或持久 fanout worker，而不是继续调同一个 sample knob。 |
| `hotgroup-multirunner-analysis-20260701-subscriberpolicy-400sub-5000msg.md` | clean commit `9bdf21c5` 将 push-gateway cadence 从 fanout-mode policy 扩展为 subscriber-aware threshold policy，并以 READ_FANOUT `100:20` 复压同一 6000 人 / 5000 消息 / 8000 msg/s / 400 subscriber 场景。4 个 shard 共读完 100000 条 signal，message / delivery outbox pending=0；但 signal span 289.249s、span rate 约 345.723 signals/s，低于 `37b575e5` baseline 的 1413.391 signals/s。结论：first-stage subscriber threshold 没有突破 drain 瓶颈。 |
| `hotgroup-metrics-window-20260701-subscriberpolicy-400sub-5000msg.md` | subscriber-aware threshold 窗口显示核心 target up，`delivery_outbox_pending` 峰值 1806 后归零，writer / Redis subscriber error、queue-full 和 eviction 均为 0；`delivery_notify` write p95 / p99 约 0.241ms / 0.438ms，Redis subscriber fanout p95 / p99 约 1.977ms / 6.538ms。服务侧未出现 outbox、writer 或 Redis 错误，本轮主要暴露静态阈值 cadence 不足以形成稳定吞吐提升。 |
| `hotgroup-multirunner-analysis-20260701-routecache-400sub-5000msg.md` | clean commit `304383ea` 的 Redis conversation route cache 复压：同一 6000 人 / 5000 消息 / 8000 msg/s / 400 subscriber / READ_FANOUT `100:20` 场景下，4 个 shard 共读完 100000 条 signal，signal span 从 289.249s 降至 146.62s，span rate 从约 345.723 提升到约 682.034 signals/s，约 1.97x；message / delivery outbox pending=0，writer / Redis subscriber error、queue-full 和 eviction 均为 0。该结果说明重复 route lookup 是 subscriber-aware 策略的重要成本，但仍未回到 fanout-mode policy baseline。 |
| `hotgroup-metrics-window-20260701-routecache-400sub-5000msg.md` | route cache 复压窗口显示核心 target up，`delivery_outbox_pending` 峰值 1985 后归零，WebSocket `delivery_notify` write p95 / p99 约 0.241ms / 0.433ms，queue p95 / p99 约 1.928ms / 2.529ms，Redis subscriber fanout p95 / p99 约 1.96ms / 6.25ms。该窗口中 Prometheus 只 scrape 4 个 ws debug target，delivery-consumer 尚未进入 debug scrape，因此 route cache hit / miss 为 0。clean commit `b119716d` 已补并 redeploy delivery-consumer debug endpoint / Prometheus core target；下一轮应复压验证命中率曲线。 |
| `hotgroup-multirunner-analysis-20260701-routecache-metrics-400sub-5000msg.md` | delivery-consumer metrics target 接入后的负例诊断：Docker env 中 READ_FANOUT subscriber policy 为空，导致 5000 条消息走全量 remote publish，`remote_publish_call` 约 20021，100000 条 expected signal 的 span 拉长到 486.339s。该 run 用来证明配置漂移风险，不作为容量结论。 |
| `hotgroup-metrics-window-20260701-routecache-metrics-400sub-5000msg.md` | 同一负例窗口已能看到 delivery-consumer route cache 指标：`conversation_route_cache_hit_window` 约 3352.621、miss 约 1653.5，`remote_publish_call_window` 约 20021.44，说明观测链路打通，也暴露了全量 publish 放大。 |
| `hotgroup-multirunner-analysis-20260701-policydefaults-400sub-5000msg.md` | 固定本地 Docker 默认 policy 后复压：READ_FANOUT / BROADCAST_SIGNAL 默认 sample=10、subscriber policy `100:20`，同一 6000 人 / 5000 消息 / 8000 msg/s / 400 subscriber 场景下，4 个 shard 共读完 100000 条 signal，span 193.559s，span rate 约 516.638 signals/s，message / delivery outbox pending=0，writer / Redis subscriber error、queue-full 和 eviction 均为 0。 |
| `hotgroup-metrics-window-20260701-policydefaults-400sub-5000msg.md` | 修正后窗口显示 delivery-consumer route cache hit / miss 已可见：hit window 约 4414.596、miss 约 730.621，`remote_publish_call_window` last 约 1029.043，`remote_enqueued_sessions_window` last 约 102904.324。该 run 比旧 routecache baseline 146.62s 慢，下一步需同配置重复复验确认波动来源。 |
| `hotgroup-multirunner-analysis-20260701-policydefaults-repeat-400sub-5000msg.md` | 同配置 repeat：clean commit `623c797`，6000 人 / 5000 消息 / 8000 msg/s / 400 subscriber / READ_FANOUT `100:20`，4 个 shard 共读完 100000 条 signal，span 193.012s，span rate 约 518.102 signals/s；与上一轮 policydefaults baseline 193.559s / 516.638 signals/s 的 ratio 为 1.003。结论：corrected policy 曲线稳定。 |
| `hotgroup-metrics-window-20260701-policydefaults-repeat-400sub-5000msg.md` | repeat 窗口显示 delivery-consumer route cache hit / miss 约 4411.541 / 728.917，`remote_publish_call_window` 约 1028.091，`remote_enqueued_sessions_window` 约 102809.147；writer / Redis subscriber error、queue-full 和 eviction 均为 0。下一步不再重复同一静态配置，应设计 dynamic cadence / 更强 pull-first / 持久 fanout worker。 |
| total-subscriber-aware policy code module | push-gateway 已补 total-subscriber-aware pull-first policy：Redis route 会先按全局 conversation route 数计算 effective sample，再把该 decision 传给远端 ws gateway，解决 400 subscriber 被 4 个 gateway 拆成 100 / 100 / 100 / 100 后无法触发更强 cadence 的问题。本地 Docker 默认 READ_FANOUT / BROADCAST_SIGNAL total policy 为 `400:50`。该模块已完成 focused checks，后续复压结果见下一行；容量结论必须以复压报告为准。 |
| `hotgroup-multirunner-analysis-20260701-totalsubpolicy-400sub-5000msg.md` | clean commit `9046dc3` 的 total-subscriber-aware policy 复压：6000 人 / 5000 消息 / 8000 msg/s / 400 subscriber / expected sample=50，4 个 shard 全部完成，signal 数从 baseline 100000 降到 40000，message / delivery outbox pending=0；但 signal span 为 193.02s，和 baseline 193.012s 基本相同。原始 summary 显示 5000 条 SendMessage 实际发送耗时 74.916s，achieved send rate 约 66.741 msg/s，远低于 target 8000 msg/s。结论：在线 frame 减量成立，但 drain 时间没有改善，不能写成吞吐提升，也不能把 target rate 当作真实 QPS。 |
| `hotgroup-metrics-window-20260701-totalsubpolicy-400sub-5000msg.md` | total-subscriber-aware policy 的 Prometheus 窗口：核心 target up，`delivery_outbox_pending` 峰值 2666 后归零，delivery-consumer route cache hit / miss 约 4345 / 723，remote publish call window 约 405，writer / Redis subscriber error、queue-full 和 slow eviction 均为 0；下一步应查 actual SendMessage duration、delivery_outbox signal production cadence、Kafka cadence 和 push event pacing。 |
| `hotgroup-analysis-20260701-stageattr-diagnostic.md` | dirty diagnostic `b9612a9` 的 SendMessage 阶段指标验证：6000 人 / 5000 消息 / 512 concurrency，5000/5000 成功，实际约 2145 msg/s，SendMessage p99 546.306ms，message / delivery outbox pending=0。该 run 因非 Backend Lab 文档脏改标记 dirty，不能作为 clean 容量点；但 Prometheus 窗口确认阶段指标可用，recent p99 主要落在 dependency read / policy check、repository append call 和 PG pool acquire。 |
| `hotgroup-metrics-window-20260701-stageattr-6000x5000-512c.md` | stage attribution diagnostic 的低敏窗口：核心 target up，`send_message_recent p99` 约 247.194ms，`dependency_read_recent p99` 约 210.849ms，`policy_check_recent p99` 约 190.723ms，`repository_append_call_recent p99` 约 120.568ms，`repository_pool_acquire_recent p99` 约 86.471ms，sequencer allocation recent p99 约 31.605ms。下一轮仍需 clean 512 / 768 对照。 |
| `hotgroup-analysis-20260701-procresource-512c.md` | dirty diagnostic `2a06960` 用新的 process/container 资源采样重跑 6000 人 / 5000 消息 / 512 concurrency：5000/5000 成功，实际约 2360.625 msg/s，SendMessage p99 306.885ms，message / delivery outbox pending=0。该 run 因采样工具和文档未提交标记 dirty，不能作为 clean 容量点；但验证了压测相关进程 / Ubuntu 容器资源窗口可用，PostgreSQL、message-service、policy-service、Kafka 等容器已有按进程口径的 CPU / memory 证据。 |
| `hotgroup-metrics-window-20260701-procresource-512c.md` | process-resource diagnostic 的 Prometheus 窗口：核心 target up，`send_message_recent p99` 约 251.436ms，`dependency_read_recent p99` 约 214.337ms，`policy_check_recent p99` 约 194.543ms，`repository_append_call_recent p99` 约 91.9ms，`repository_pool_acquire_recent p99` 约 58.525ms。下一步需在采样工具 clean commit 后复跑 512 / 768。 |
| `hotgroup-analysis-20260701-procresource-clean-512c.md` | clean commit `234e834` 的 process/container 资源口径 512 concurrency 复压：6000 人 / 5000 消息 / READ_FANOUT + SEQUENCER_BLOCK，5000/5000 成功，实际约 2464.17 msg/s，SendMessage p99 277.387ms，message / delivery outbox pending=0，分类仍是 `send-path-latency`。原始 process-resource summary 在 `H:\NexusIM\loadtest-results\hotgroup-procresource-clean-6000x5000-512c-234e8347-20260701-2040\lab-process-resource\lab-process-resource-summary.md`；PostgreSQL 平均约 130% CPU、Kafka 约 42%、message-service gRPC 约 13%、policy-service gRPC 约 9%，说明已进入 Ubuntu 相关容器资源视角，但整机仍未被打满。 |
| `hotgroup-metrics-window-20260701-procresource-clean-512c.md` | clean 512 process-resource 窗口：核心 target up，`send_message_recent p99` 约 251.436ms，`dependency_read_recent p99` 约 214.337ms，`policy_check_recent p99` 约 193.372ms，`repository_append_call_recent p99` 约 55.715ms，`repository_pool_acquire_recent p99` 约 24.246ms。下一步继续 clean 768 对照，观察吞吐提升是否仍伴随 policy/dependency 长尾。 |
| `hotgroup-analysis-20260701-procresource-clean-768c.md` | clean commit `fa8e475` 的 process/container 资源口径 768 concurrency 对照：6000 人 / 5000 消息 / READ_FANOUT + SEQUENCER_BLOCK，5000/5000 成功，实际约 2484.691 msg/s，SendMessage p99 397.149ms，message / delivery outbox pending=0，分类仍是 `send-path-latency`。相比 512 concurrency，吞吐仅从约 2464.17 到 2484.691 msg/s，p99 从 277.387ms 升至 397.149ms，说明继续加客户端并发收益很小且长尾恶化。原始 process-resource summary 在 `H:\NexusIM\loadtest-results\hotgroup-procresource-clean-6000x5000-768c-fa8e475f-20260701-2047\lab-process-resource\lab-process-resource-summary.md`；PostgreSQL 平均约 163% CPU、Kafka 约 39%、policy-service gRPC 约 13%、message-service gRPC 约 6%。 |
| `hotgroup-metrics-window-20260701-procresource-clean-768c.md` | clean 768 process-resource 窗口：核心 target up，`send_message_recent p99` 约 354.557ms，`dependency_read_recent p99` 约 331.762ms，`policy_check_recent p99` 约 285.685ms，`repository_append_call_recent p99` 约 49.473ms，`repository_pool_acquire_recent p99` 约 16.126ms。对比 512，长尾主要继续落在 dependency read / policy check，不是 repository append 或 PG pool acquire。 |

## Conversation signal cadence

push-gateway 的 conversation signal cadence 分四层：

```text
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_<FANOUT_MODE>
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SUBSCRIBER_POLICY_<FANOUT_MODE>
NEXUSIM_PUSH_CONVERSATION_SIGNAL_TOTAL_SUBSCRIBER_POLICY_<FANOUT_MODE>
```

前两层是固定 sample cadence。第三层是 per-gateway subscriber-aware policy，第四层是
whole-conversation total-subscriber-aware policy。后两层格式相同：

```text
min_subscribers:sample_every[,min_subscribers:sample_every...]
```

例子：

```text
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_READ_FANOUT=10
NEXUSIM_PUSH_CONVERSATION_SIGNAL_SUBSCRIBER_POLICY_READ_FANOUT=100:20
NEXUSIM_PUSH_CONVERSATION_SIGNAL_TOTAL_SUBSCRIBER_POLICY_READ_FANOUT=400:50
```

含义是：READ_FANOUT 默认每 10 条 seq 发一次 online signal；如果本机 memory
registry 或某个远端 gateway 对同一 conversation 有至少 100 个 subscriber，则改成每
20 条 seq 发一次；如果整个 conversation 在所有 gateway 上共有至少 400 个
subscriber，则改成每 50 条 seq 发一次。effective sample 只会比 fanout-mode sample
更保守，不会因为 threshold 变得更频繁。

注意：

- subscriber threshold 按本机 / 每个远端 gateway 的 subscriber 数计算，不是全局房间
  subscriber 总数。4 个 ws 各 100 个 subscriber 的 400 subscriber 拓扑，应使用
  `100:20`；如果要按全局 400 subscriber 触发更强 pull-first，使用
  `NEXUSIM_PUSH_CONVERSATION_SIGNAL_TOTAL_SUBSCRIBER_POLICY_READ_FANOUT=400:50`。
- total-subscriber threshold 由 Redis route 基于全局 conversation routes 计算，并把
  effective sample decision 传给各 ws gateway；这样各 gateway 不会只按本机 100
  subscriber 重复做较弱策略。
- 没有配置 subscriber threshold 时，Redis route 会保留旧的采样前置快速路径，不会为了
  sampled-out signal 查询 route。
- sampled-out remote signal 不写 Redis resume；durable 展示仍以 PullInbox / ACK 为准。
- runner 侧必须同步设置 `--conversation-signal-sample-every` 为预期 effective cadence，
  否则 expected signal 数会和服务端策略不一致。

## 目标

热点群聊压测要证明端到端链路，而不是单个服务的孤立吞吐：

```text
CreateConversation / member seed
-> 多 sender SendMessage
-> message_outbox
-> Kafka conversation.timeline.events
-> delivery-service membership projection + user_inbox fanout
-> delivery_outbox / im.delivery.events
-> push-gateway online notify
-> PullInbox
-> AckDelivery
```

## 推荐场景

| 场景 | 目的 |
| --- | --- |
| medium group fanout | 100 成员验证小群 `WRITE_FANOUT`；1,000 成员验证自动 promotion 到 `HYBRID_FANOUT` 和 end-to-end latency。 |
| hot sender burst | 多 sender 高频发送，观察 conversation seq、Kafka lag 和 outbox pending。 |
| online notify storm | 高在线比例 + 慢客户端，验证 push queue、slow eviction 和 PullInbox 兜底。 |
| member churn during send | 发送期间 join / leave / remove / role change，验证历史可见窗口。 |
| delivery outage recovery | 压测中停止 / 恢复 delivery worker，验证 projection 追平和幂等。 |
| push route fault | Redis / push-gateway 局部故障，验证在线通知可丢但 durable inbox 不丢。 |

## 必需指标

runner summary 至少输出：

```text
group_size
online_ratio
sender_count
send_concurrency
message_rate
achieved_send_rate
duration
send_success_count
send_error_count
send_p95_ms
send_p99_ms
timeline_lag_max
delivery_projection_lag_max
inbox_rows_created
inbox_rows_per_message
pull_visible_p95_ms
pull_visible_p99_ms
ack_p95_ms
ack_p99_ms
push_notify_received
push_notify_missed
slow_session_evicted
outbox_pending
dlq_count
postgres_pool_acquire_p95_ms
postgres_lock_wait_count
kafka_lag_max
```

原始大文件继续写入 `H:\NexusIM\loadtest-results`；仓库只保留低敏 summary 和报告。

## 自动分析工具

每轮正式压测后先用离线分析器把多个 `hotgroup-summary.json` 汇总成低敏报告：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\analyze-hotgroup-loadtest.ps1 `
  -RunNamePattern hotgroup-readfanout-6000-*clean-01b2a70e-* `
  -OutputPath docs\runbook\loadtest\hotgroup\hotgroup-analysis-20260630-readfanout-clean.md `
  -RequireCleanCommit
```

分析器只读取 `H:\NexusIM\loadtest-results` 下的原始 summary，不改原始数据。
它会输出 run matrix、clean / dirty 状态、SendMessage / PullInbox 延迟、outbox pending、
conversation signal drain、瓶颈分类和下一步策略。

多 runner 对照需要用专用分析器把一个 coordinator 和多个 subscriber shard 合成同一轮报告：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\analyze-hotgroup-multirunner.ps1 `
  -CoordinatorRunName hotgroup-multirunner-400sub-coordinator-20260701-013557 `
  -ShardRunNamePattern 'hotgroup-multirunner-400sub-shard*-20260701-013557' `
  -BaselineRunName hotgroup-readfanout-6000-8000qps-400sub-233d6956-20260701-004948 `
  -OutputPath docs\runbook\loadtest\hotgroup\hotgroup-multirunner-analysis-20260701-400sub.md
```

该报告会用首帧到末帧的 signal span 计算 drain rate，避免把 subscriber-only 提前等待
coordinator 建群 / 加成员的时间误算为在线信号写出瓶颈。

当前瓶颈分类规则保持保守：

```text
send errors / high p99         -> send path
message_outbox pending / DLQ   -> message outbox relay
delivery_outbox pending / DLQ  -> delivery outbox relay
PullInbox / ACK error or slow  -> receiver read / ack
subscriber incomplete / error  -> push subscribe / read path
writer duration p95 / p99 high -> WebSocket write / flush / network
outbox 追平但 signal drain 长  -> online signal drain
signal 数下降但 span 不降       -> signal-volume-reduced-without-drain-improvement
缺少信号或 lag 字段            -> insufficient observability
```

大群 sampled online signal 场景必须同时配置服务端和 runner：

```text
push-gateway:
  NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY=10
  # 可选：只对指定 fanout mode 覆盖默认采样间隔
  NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_READ_FANOUT=10
  NEXUSIM_PUSH_CONVERSATION_SIGNAL_SAMPLE_EVERY_BROADCAST_SIGNAL=10

loadtest/hotgroup:
  --conversation-signal-sample-every 10
```

默认值 `1` 表示全量 signal，旧压测口径不变。`WRITE_FANOUT` / `HYBRID_FANOUT` /
`READ_FANOUT` / `BROADCAST_SIGNAL` 可以通过 mode-specific env 设置不同在线唤醒
cadence；未设置 mode-specific env 时使用显式 default policy。采样场景的成功条件不再是
`subscriber_count * message_count`，而是
`subscriber_count * expected_signals_per_subscriber`。这不是可靠投递降级：
WebSocket signal 只作为在线唤醒，最终展示和 ACK 仍必须通过 PullInbox 追平最新 seq。

该报告只能作为本地 / 三机压测诊断材料，不能单独替代 Grafana / Prometheus 时间窗口，
也不能写成生产 SLO。

## 多 Runner 读取验证

400 subscriber 阶梯已证明 Redis route / WebSocket writer 没有错误或 eviction，但
`online-signal-drain` 仍稳定在约 2.8k signals/s。随后 4 个 `subscriber-only`
runner shard 的对照 run 也没有把总 drain rate 提升到新的量级，因此当前不能继续把瓶颈
简单归因到单个 runner 的 JSON decode / accounting。

下一步不要继续只增大 `--conversation-subscriber-count`；应进入 push-gateway
conversation signal 写出路径、WebSocket flush cadence、Redis subscriber fanout、
per-connection write scheduling 和有线网络吞吐的代码级定位。
第一轮代码优化已经处理本地 memory registry fanout 的锁持有范围；clean commit
`4bc4a30` 复压后 drain rate 仍在约 2.89k signals/s，说明 registry global mutex
不是主瓶颈。第二轮优化在 registry fanout 时预编码 delivery / conversation notify
JSON，让 WebSocket writer 优先写 cached payload，避免同一条热点 signal 在每个
connection 写出前重复 marshal；clean commit `d8d78fd` 复压后 drain rate 约
2863 signals/s，仍未离开 2.85k-2.89k signals/s 区间。下一步应转向 WebSocket
writer flush cadence、per-connection write scheduling、nhooyr 写入策略、连接侧读取背压和网络吞吐。

为避免继续靠 CPU 空闲或进程列表猜瓶颈，下一轮 push-gateway 镜像必须包含
WebSocket writer duration histogram。`/metrics` 会输出 `frame_write` 和
`delivery_notify` 的 write duration bucket / sum / count / max；
`tools/record-hotgroup-metrics-window.ps1` 会在窗口报告中写出 delivery notify
p95 / p99 / avg / max。若 write duration 长尾高，优先分析 nhooyr `conn.Write`、
flush cadence、per-connection scheduling 和网络；若 write duration 低但 drain 仍慢，
则继续转向 subscriber read loop、runner 端 decode / accounting 或链路吞吐。
clean commit `4f45519` 复压后，`delivery_notify` write p95 / p99 低于 0.5ms，
而整体 drain rate 仍约 2.88k signals/s，因此下一轮不要继续优先调单次
`conn.Write`；应补 Redis subscriber 收到事件后本地 fanout / enqueue duration，
并把 enqueue、writer duration 和 runner drain span 放在同一报告里对比。
clean commit `6099ecd` 复压后，Redis subscriber conversation signal fanout/enqueue
整窗口 p95 / p99 约 `56.14ms / 91.228ms`，而单次 WebSocket write p99 约
`0.63ms`。因此下一轮不要继续优先调单次 `conn.Write`；应设计 push-gateway
conversation fanout worker / shard queue，让 Redis subscriber 快速 handoff，并用
queue depth、worker drain、enqueue latency、backpressure 和 eviction 指标验证。

`loadtest/hotgroup` 支持两个运行模式：

```text
--runner-mode full             # 默认模式：建群、加成员、发消息、等待投影、可选订阅、Pull/Ack 抽样
--runner-mode subscriber-only  # 只按 deterministic 用户模型打开 WebSocket subscriber 并等待 signal
```

多 runner 运行时，先启动多个 `subscriber-only` 进程，使用相同 tenant /
conversation / group / sender / message_count，并通过 shard 参数拆分订阅者：

```powershell
# shard 0/4 示例；其它机器改 --subscriber-shard-index 为 1、2、3
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --runner-mode subscriber-only `
  --run-name hotgroup-readfanout-6000-8000qps-400sub-shard0 `
  --tenant-id tenant-shared `
  --conversation-id conv-shared `
  --group-size 6000 `
  --sender-count 256 `
  --message-count 5000 `
  --conversation-subscriber-count 400 `
  --subscriber-shard-count 4 `
  --subscriber-shard-index 0 `
  --push-url ws://172.31.50.2:10498/ws `
  --require-conversation-notify `
  --wait-timeout 25m
```

所有 shard 都连上后，再启动一个 coordinator，不打开本地 subscriber，只负责建群和发消息：

```powershell
go run .\loadtest\hotgroup `
  --runner-mode full `
  --run-name hotgroup-readfanout-6000-8000qps-coordinator `
  --conversation-target 172.31.50.2:10496 `
  --message-target 172.31.50.2:10495 `
  --delivery-target 172.31.50.2:10497 `
  --pg-dsn "postgres://nexusim:nexusim@172.31.50.2:5432/nexusim?sslmode=disable" `
  --tenant-id tenant-shared `
  --conversation-id conv-shared `
  --group-size 6000 `
  --sender-count 256 `
  --message-rate 8000 `
  --message-count 5000 `
  --conversation-subscriber-count 0 `
  --receiver-sample-count 20 `
  --expect-fanout-mode READ_FANOUT `
  --require-delivery-outbox-drain `
  --cleanup
```

多 runner 的每个 shard 都会输出自己的 `hotgroup-summary.json`，其中
`push.subscriber_total_count` 是总订阅目标，`push.subscriber_count` 是本 shard
实际连接数，`push.subscriber_shard_index/count` 记录分片身份。正式报告需要把所有 shard
的 signal 总数、最慢 `last_signal_after_ms`、error / eviction 与 coordinator 的
SendMessage / PullInbox / outbox 指标一起记录。

每轮正式压测还必须记录至少一个 Prometheus / Grafana 或 debug metrics 时间窗口。
如果使用 Prometheus，可用窗口记录工具从 H 盘原始目录生成低敏报告：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\record-hotgroup-metrics-window.ps1 `
  -ResultDir H:\NexusIM\loadtest-results\hotgroup-readfanout-6000-8000qps-clean-01b2a70e-20260630-2336 `
  -MarkdownPath docs\runbook\loadtest\hotgroup\hotgroup-metrics-window-20260630-readfanout-clean-8000qps.md
```

工具会把原始 Prometheus JSON 写回对应 `H:\NexusIM\loadtest-results\<run>`，
仓库只保存低敏 Markdown 摘要。窗口报告仍不是生产 SLO，只用于解释该轮压测
是否有核心 target、outbox、projection、push writer、Redis route 和 PostgreSQL pool 指标。
`_5m` 指标表示移动五分钟压力窗口，`_window` 指标表示整个捕获窗口内的近似累计值；
后续定位 online signal drain 时必须同时记录 writer success/error、Redis subscriber enqueue/error、
session eviction 和 runner 侧 subscriber 完成时间。

## 可视化要求

热点群聊压测必须配套趋势图。仓库已提供 first-stage Grafana dashboard：

```text
deploy/local/grafana/dashboards/hotgroup-observability.json
```

Grafana 通过 `deploy/local/docker-compose.grafana.yml` 自动加载该 dashboard，标题为
`NexusIM Hot Group Loadtest`。正式压测报告至少要记录：

当前三机本地压测约定端口：

```text
Kafka UI:    http://172.31.50.2:19090
Prometheus:  http://172.31.50.2:19091
Grafana:     http://172.31.50.2:13000
OTel gRPC:   172.31.50.2:14317
OTel HTTP:   http://172.31.50.2:14318
OTel health: http://172.31.50.2:14333
```

`19090` 固定留给 Kafka UI；Prometheus 使用 `19091`，避免压测时把两个入口混淆。

```text
dashboard_name
prometheus_time_range
run_name
commit
group_size
fanout_mode / expected_fanout_mode
SendMessage p95 / p99 趋势
message_outbox / delivery_outbox pending 趋势
delivery projection failure / worker error 趋势
user_inbox / membership projection 增长趋势
PullInbox / AckDelivery gRPC 请求和延迟趋势
push session / slow eviction 趋势
PostgreSQL pool 趋势
```

当前 dashboard 只使用已有 `/metrics` 指标，不假装拥有尚未实现的 Kafka consumer lag
exporter 或 fanout-mode distribution exporter。`fanout_mode / expected_fanout_mode`
在 runner summary 和 PostgreSQL 统计中校验；后续再沉淀成 Prometheus exporter。
缺口仍需后续补：

```text
conversation fanout mode distribution
message timeline topic lag
delivery timeline consumer lag by topic / partition / group
delivery_timeline_items count / insert rate
user_inbox rows per message
PostgreSQL lock / WAL / dead tuple time-series exporter
push signal writer flush / client observed gap 的趋势化 dashboard
```

2026-06-30 后续代码已补齐第一阶段 push signal 观测字段：

```text
push-gateway /metrics:
  nexusim_push_gateway_ws_writer_events_total
  nexusim_push_gateway_ws_writer_last_event_unix_milliseconds

loadtest/hotgroup summary:
  push.subscriber_signals[].signal_count
  push.subscriber_signals[].max_conversation_seq
  push.subscriber_signals[].first_signal_after_ms
  push.subscriber_signals[].last_signal_after_ms
  push.subscriber_signals[].completed
  push.subscriber_signals[].error
```

下一轮三机压测必须使用包含这些字段的最新镜像 / runner；否则无法区分 WebSocket writer
未写出、客户端读取慢、session queue 压力或 runner accounting 问题。

没有 Grafana / Prometheus 趋势图的运行，只能作为功能 smoke、dry-run 或一次性
diagnostics，不能写成热点群聊容量证明。

## 当前 v0.1 runner

Dry-run 只生成用户模型和计划，适合先评审 group/user/device/session 建模：

```powershell
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --dry-run `
  --run-name hotgroup-review-dryrun `
  --group-size 100 `
  --sender-count 5 `
  --message-count 20 `
  --online-ratio 0.2 `
  --slow-client-ratio 0.05
```

输出：

```text
H:\NexusIM\loadtest-results\<run-name>\hotgroup-summary.json
H:\NexusIM\loadtest-results\<run-name>\hotgroup-report.md
H:\NexusIM\loadtest-results\<run-name>\users.jsonl
```

真实执行需要 conversation / message / delivery 主进程、message outbox relay、
delivery timeline consumer、PostgreSQL 和 Kafka 已启动：

```powershell
. .\tools\go-env.ps1
go run .\loadtest\hotgroup `
  --run-name hotgroup-smoke-100 `
  --pg-dsn postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable `
  --conversation-target 127.0.0.1:13096 `
  --message-target 127.0.0.1:13095 `
  --delivery-target 127.0.0.1:13097 `
  --group-size 100 `
  --sender-count 5 `
  --message-rate 10 `
  --duration 60s `
  --receiver-sample-count 10 `
  --expect-fanout-mode WRITE_FANOUT `
  --cleanup
```

业务写入路径必须走公开 gRPC。PostgreSQL 只用于清理测试租户、等待异步投影和读取统计。

conversation-service 会在成员变更事务内按 ACTIVE 成员数做单调 promotion：

```text
<=500      WRITE_FANOUT
501-5000   HYBRID_FANOUT
5001-50000 READ_FANOUT
>50000     BROADCAST_SIGNAL / SEQUENCER_BLOCK active first-stage
```

promotion 只向更高版本推进，不因为成员离开自动降级，避免压测中策略反复震荡。
当前热点 `SEQUENCER_BLOCK` 已不是 contract-only：消息写入和成员边界事件都必须通过
timeline-service `AllocateSeqBlock` 获取 valid lease。未配置 sequencer、lease 无效或
lease 过期时 fail-closed，不能回退到本地 row lock，否则压测结果会把热点路径误测成小群路径。
hotgroup runner 的结果必须记录实际 `fanout_mode`，否则不能解释不同规模下的
写放大和延迟曲线。
如果传入 `--expect-fanout-mode`，runner 会在发送前校验 conversation 当前策略；
不匹配时 fail-closed，避免把小群 `WRITE_FANOUT` 结果误写成中大群 fanout 结论。

如果本轮要把 `delivery_outbox -> im.delivery.events` 也纳入通过条件，必须同时启动
`delivery-service outbox-relay`，并显式打开：

```powershell
  --require-delivery-outbox-drain
```

该开关会等待当前测试 conversation 的 `delivery_outbox PENDING` 行追平到 `0`。没有打开时，
runner 只证明 durable inbox / PullInbox / AckDelivery，不证明在线通知事件已经全部发布。

## 设计边界

- 不把热点群聊压测结果表述为生产 SLO。
- 不用固定字符串 toy endpoint 代替真实 IM 链路。
- 不用当前成员表回查历史可见性；必须经过 conversation / delivery 的成员窗口模型。
- 不让 push-gateway 拥有 durable inbox；在线通知缺口必须通过 PullInbox 恢复。
- 如果引入新中间件，先在架构文档和 middleware catalog 说明瓶颈、替代方案和 owner。

## 面试口径

热点群聊不是单接口 QPS 问题，而是 fanout、写放大、在线通知风暴和补拉追平问题。
NexusIM 的第一阶段策略是：

```text
消息事实只写一次；
delivery 异步 fanout；
push 只做轻量在线唤醒；
可靠恢复靠 durable inbox；
超大群优先考虑 fanout bucket / lazy inbox，再按瓶颈引入中间件。
```
