# push-gateway Win/Mac arm64 distributed smoke

本报告记录一次小规模双机分布式 smoke。它验证 `push-gateway` WebSocket gateway 可以运行在 Mac Docker，核心服务、PostgreSQL、Kafka、Redis 和 `push-gateway delivery-consumer` 运行在 Windows，并通过有线 `172.31.50.*` 完成在线通知、回源拉取、ACK 和跨实例 resume。

这不是容量压测，也不是生产级 HA 验收。

## 环境

| 项 | 值 |
| --- | --- |
| Windows wired IP | `172.31.50.1` |
| Mac wired IP | `172.31.50.2` |
| Mac Docker service images | `linux/arm64` |
| Windows infra | PostgreSQL / Kafka / Redis / Schema Registry / Kafka UI |
| Mac role | `nexusim/push-gateway:local` WebSocket gateway |
| Windows role | core services, delivery gRPC Docker, push delivery-consumer |
| Traffic path | service-to-service traffic uses `172.31.50.*` |

Mac image architecture was checked before the smoke:

```text
nexusim/conversation-service:local arm64/linux
nexusim/message-service:local      arm64/linux
nexusim/delivery-service:local     arm64/linux
nexusim/push-gateway:local         arm64/linux
nexusim/receipt-service:local      arm64/linux
nexusim/contacts-service:local     arm64/linux
postgres:16-alpine                 arm64/linux
redis:7-alpine                     arm64/linux
confluentinc/cp-kafka:7.7.1        arm64/linux
confluentinc/cp-schema-registry    arm64/linux
provectuslabs/kafka-ui:latest      arm64/linux
```

## Full Route Smoke

Command:

```powershell
.\tools\run-win-mac-push-smoke.ps1 `
  -MacRunMode docker `
  -WindowsDeliveryRunMode docker `
  -MacHost 172.31.50.2 `
  -WindowsWiredHost 172.31.50.1 `
  -RedisAddrForMac 172.31.50.1:6379 `
  -RedisAddrForWindows 127.0.0.1:6379 `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RunName push-gateway-win-mac-arm64-smoke-20260612-011105
```

Result:

```text
H:\NexusIM\loadtest-results\push-gateway-win-mac-arm64-smoke-20260612-011105\pushgateway-summary.json
```

Key facts:

| Check | Result |
| --- | --- |
| success | `true` |
| client WebSocket URL | `ws://172.31.50.2:11598` |
| Redis route from Mac | `172.31.50.1:6379` |
| delivery gRPC callback from Mac | `172.31.50.1:11597` |
| received notify | `delivery.notify`, `conversation_seq=2` |
| durable read | `PullInbox item_count=1`, `max_seq=2` |
| ACK | `delivery.ack.ok last_received_seq=2` |
| delivery outbox | `PUBLISHED=2`, `PENDING=0`, `DLQ=0` |

This proves the WebSocket gateway on Mac received an online wakeup produced by the Windows-side delivery consumer through Redis route, then completed durable `PullInbox` and `AckDelivery`.

## Cross-Instance Resume Smoke

Command:

```powershell
.\tools\run-win-mac-push-smoke.ps1 `
  -Scenario cross-instance-resume `
  -MacRunMode docker `
  -WindowsDeliveryRunMode docker `
  -MacHost 172.31.50.2 `
  -WindowsWiredHost 172.31.50.1 `
  -RedisAddrForMac 172.31.50.1:6379 `
  -RedisAddrForWindows 127.0.0.1:6379 `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RunName push-gateway-win-mac-arm64-resume-smoke-20260612-011213 `
  -SkipMacSync
```

Result:

```text
H:\NexusIM\loadtest-results\push-gateway-win-mac-arm64-resume-smoke-20260612-011213\pushgateway-summary.json
```

Key facts:

| Check | Result |
| --- | --- |
| success | `true` |
| initial WebSocket | Mac gateway, `ws://172.31.50.2:11598` |
| reconnect WebSocket | Windows gateway, `ws://127.0.0.1:11599` |
| original notify | `conversation_seq=2`, same `event_id/message_id` |
| replayed notify | same `conversation_seq/event_id/message_id` |
| consumer append metric | `redis_resume_append_count=1` |
| reconnect replay metric | `redis_resume_replay_count=1` |
| resume miss | `redis_resume_miss_count=0` |
| durable read | `PullInbox item_count=1`, `max_seq=2` |
| ACK | `delivery.ack.ok last_received_seq=2` |
| delivery outbox | `PUBLISHED=2`, `PENDING=0`, `DLQ=0` |

This proves a client can first connect to the Mac gateway, disconnect before ACK, then reconnect to the Windows gateway with the same `resume_token` and receive Redis-backed replay of the same lightweight `delivery.notify`. Reliable state still comes from `PullInbox` and `AckDelivery`.

## Notes

Both summaries show `git_dirty=true` because the working tree contained uncommitted Docker support files and this runbook note at the time of execution:

```text
deploy/docker/conversation-service.runtime.Dockerfile
deploy/docker/delivery-service.runtime.Dockerfile
deploy/docker/push-gateway.runtime.Dockerfile
deploy/docker/receipt-service.runtime.Dockerfile
docs/runbook/mac-arm64-docker-images.md
```

The runtime path itself succeeded. A later clean run can be used if a strict clean-commit artifact is required.

## Interpretation

This smoke supports the following interview statement:

```text
NexusIM can split online gateway and delivery consumer across two physical machines.
The Mac gateway owns WebSocket sessions, while the Windows-side consumer consumes Kafka delivery events.
Redis route and Redis-backed resume connect those gateway instances, and durable delivery is still guaranteed by PullInbox and AckDelivery rather than WebSocket or Redis.
```

Do not overstate it as:

```text
Full production-grade multi-region HA or capacity validation.
```

Remaining production hardening includes Redis quorum / network partition tests, Kafka HA, PostgreSQL failover, service discovery, deployment orchestration, mTLS, structured observability and capacity planning.
