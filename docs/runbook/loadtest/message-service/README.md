# Message Service 压测报告索引

本目录保存 `message-service` 第一阶段所有压测报告。优先阅读总报告：

```text
loadtest-report-20260609-message-service-consolidated.md
```

## 报告结构

| 类型 | 文件 |
| --- | --- |
| 总报告 | `loadtest-report-20260609-message-service-consolidated.md` |
| 第一轮真实链路基线 | `loadtest-report-20260609.md` |
| PostgreSQL / 多实例诊断 | `loadtest-report-20260609-pgpool-multi-instance.md`、`loadtest-report-20260609-multi-instance-budget.md`、`loadtest-report-20260609-postgres-loadtest-profile.md` |
| Backpressure / client retry | `loadtest-report-20260609-backpressure*.md`、`loadtest-report-20260609-client-retry.md` |
| Outbox relay / PublishBatch | `loadtest-report-20260609-outbox-*.md`、`loadtest-report-20260609-publishbatch-*.md`、`loadtest-report-20260609-relay-metrics-smoke.md` |
| Adaptive admission | `loadtest-report-20260609-adaptive-*.md`、`loadtest-report-20260609-recent-metrics-smoke.md`、`loadtest-report-20260609-logical-latency-smoke.md` |

## 当前结论

`message-service` 已具备可讲清楚的真实链路压测证据：从 gRPC 入口、PostgreSQL 本地事务、outbox relay 到 Kafka publish path 均已跑过真实进程压测。后续只在关键机制变化后做 smoke 或小规模验证，不继续消耗本机资源做大规模硬件矩阵。
