# conversation-service GetSendContext Smoke

## 目标

验证 `message-service` 可以通过真实 gRPC 调用 `conversation-service` 获取会话发送上下文，并完成 `SendMessage` 写入。

本报告不是容量压测报告，只是跨服务 read path 的最小真实进程验收。

## 拓扑

```text
loadtest/sendmessage
-> message-service gRPC :11495
-> conversation-service gRPC :11496
-> PostgreSQL conversations / conversation_members
-> message-service PostgreSQL transaction
```

未启动：

```text
outbox relay
Kafka consumer
delivery-service
push-gateway
```

## 环境

| 项 | 值 |
| --- | --- |
| PostgreSQL | `nexusim-postgres` Docker container，`localhost:5432` |
| conversation-service | 本机进程，`127.0.0.1:11496` |
| message-service | 本机进程，`127.0.0.1:11495` |
| VU | `2` |
| duration | `3s` |
| conversation_count | `2` |
| tenant | `tenant-conv-smoke` |
| policy mock permission version | `7` |

## 准备动作

1. 构建本地二进制：

```powershell
. .\tools\go-env.ps1
go build -o bin\conversation-service.exe ./services/conversation-service/cmd/conversation-service
go build -o bin\message-service.exe ./services/message-service/cmd/message-service
go build -o bin\sendmessage-loadtest.exe ./loadtest/sendmessage
```

2. 通过 Docker 内部 `psql` 应用 message / conversation migration。
3. 插入两个会话和两个用户成员：

```text
conv-conv-smoke-0
conv-conv-smoke-1
user-0
user-1
```

4. 启动 `conversation-service`：

```powershell
$env:NEXUSIM_CONVERSATION_SERVICE_MODE='grpc'
$env:NEXUSIM_CONVERSATION_GRPC_ADDR='127.0.0.1:11496'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
bin\conversation-service.exe
```

5. 启动 `message-service`，指向真实 conversation-service：

```powershell
$env:NEXUSIM_MESSAGE_SERVICE_MODE='grpc'
$env:NEXUSIM_GRPC_ADDR='127.0.0.1:11495'
$env:NEXUSIM_CONVERSATION_SERVICE_ADDR='127.0.0.1:11496'
$env:NEXUSIM_CONVERSATION_RPC_TIMEOUT='200ms'
$env:NEXUSIM_MOCK_PERMISSION_VERSION='7'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
bin\message-service.exe
```

## 压测命令

```powershell
bin\sendmessage-loadtest.exe `
  --target 127.0.0.1:11495 `
  --vus 2 `
  --duration 3s `
  --request-timeout 2s `
  --stats-wait 0s `
  --tenant-id tenant-conv-smoke `
  --conversation-prefix conv-conv-smoke `
  --conversation-count 2 `
  --pg-dsn 'postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable' `
  --result-dir loadtest\results\conversation-smoke-20260609-104549
```

## 结果

原始 summary：

```text
loadtest/results/conversation-smoke-20260609-104549/sendmessage-summary.json
```

| 指标 | 值 |
| --- | ---: |
| request_count | 725 |
| success_count | 725 |
| error_count | 0 |
| success_rate | 1.0 |
| accepted_rps | 241.67 |
| p95 | 10.36ms |
| p99 | 13.26ms |
| logical_p99 | 13.26ms |
| outbox_pending_count | 725 |

`outbox_pending_count=725` 是预期现象，因为本次只验证 `conversation-service` read path，未启动 outbox relay。测试结束后已删除 `tenant-conv-smoke` 的 message/outbox/conversation 测试数据。

## 结论

- `conversation-service` 的 PostgreSQL read path 可用。
- `message-service` 的 `ConversationQueryPort` 可以从 strict mock 切换为真实 gRPC client。
- `member_version`、`permission_version`、`conversation_mode`、`fanout_mode` 等字段能通过跨服务调用参与 `SendMessage`。
- 这一步证明系统已经开始从单微服务切片走向多微服务协作。

## 限制

- 本次没有启动 outbox relay，因此不验证 Kafka 发布。
- 本次没有压 conversation-service 的容量，只验证可运行链路。
- 成员变更 Saga、成员边界 timeline event、ACL 投影仍未实现。
