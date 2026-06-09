# conversation-service 本地运行说明

本文用于本地启动 `conversation-service`，并让 `message-service` 通过真实 gRPC 依赖读取会话发送上下文。

当前范围只覆盖：

```text
conversation-service GetSendContext
-> PostgreSQL conversations / conversation_members
-> message-service ConversationQueryPort
```

不覆盖成员变更 Saga、delivery、push、Kafka relay 容量压测。

## 1. 前置条件

进入仓库根目录：

```powershell
cd E:\development\IM
. .\tools\go-env.ps1
```

确认本地 PostgreSQL 容器运行：

```powershell
docker ps --format "{{.Names}} {{.Status}}" | Select-String nexusim-postgres
```

默认 DSN：

```powershell
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
```

## 2. 应用 migration

Windows PowerShell 直接把 SQL 管道传给容器内 `psql` 时可能出现编码问题。推荐先 `docker cp`，再在容器内执行。

```powershell
docker cp migrations\postgres\message\000001_message_core.sql nexusim-postgres:/tmp/nexusim_message_core.sql
docker exec nexusim-postgres psql -U nexusim -d nexusim -v ON_ERROR_STOP=1 -f /tmp/nexusim_message_core.sql

docker cp migrations\postgres\conversation\000001_conversation_core.sql nexusim-postgres:/tmp/nexusim_conversation_core.sql
docker exec nexusim-postgres psql -U nexusim -d nexusim -v ON_ERROR_STOP=1 -f /tmp/nexusim_conversation_core.sql

docker cp migrations\postgres\conversation\000002_member_change_saga_v2.sql nexusim-postgres:/tmp/nexusim_conversation_member_change_saga_v2.sql
docker exec nexusim-postgres psql -U nexusim -d nexusim -v ON_ERROR_STOP=1 -f /tmp/nexusim_conversation_member_change_saga_v2.sql
```

确认表存在：

```powershell
docker exec nexusim-postgres psql -U nexusim -d nexusim -tAc "select to_regclass('public.conversations'), to_regclass('public.conversation_members'), to_regclass('public.member_change_saga')"
```

## 3. 准备 smoke 数据

下面的 seed 用于两路 VU、两个 conversation 的最小 smoke。

```powershell
$sql = @"
DELETE FROM conversation_members WHERE tenant_id = 'tenant-conv-smoke';
DELETE FROM conversations WHERE tenant_id = 'tenant-conv-smoke';
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES
  ('tenant-conv-smoke', 'conv-conv-smoke-0', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local'),
  ('tenant-conv-smoke', 'conv-conv-smoke-1', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 3, 5, 7, 'local');
INSERT INTO conversation_members (tenant_id, conversation_id, user_id, role, status, member_version, permission_version)
VALUES
  ('tenant-conv-smoke', 'conv-conv-smoke-0', 'user-0', 'MEMBER', 'ACTIVE', 5, 7),
  ('tenant-conv-smoke', 'conv-conv-smoke-1', 'user-0', 'MEMBER', 'ACTIVE', 5, 7),
  ('tenant-conv-smoke', 'conv-conv-smoke-0', 'user-1', 'MEMBER', 'ACTIVE', 5, 7),
  ('tenant-conv-smoke', 'conv-conv-smoke-1', 'user-1', 'MEMBER', 'ACTIVE', 5, 7);
"@
docker exec nexusim-postgres psql -U nexusim -d nexusim -v ON_ERROR_STOP=1 -c $sql
```

## 4. 构建服务

```powershell
go build -o bin\conversation-service.exe ./services/conversation-service/cmd/conversation-service
go build -o bin\message-service.exe ./services/message-service/cmd/message-service
go build -o bin\sendmessage-loadtest.exe ./loadtest/sendmessage
```

## 5. 启动 conversation-service

新开一个 PowerShell：

```powershell
cd E:\development\IM
. .\tools\go-env.ps1
$env:NEXUSIM_CONVERSATION_SERVICE_MODE='grpc'
$env:NEXUSIM_CONVERSATION_GRPC_ADDR='127.0.0.1:11496'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
bin\conversation-service.exe
```

成功日志：

```text
conversation-service gRPC server started on 127.0.0.1:11496
```

## 6. 启动 message-service 并接入真实 conversation-service

再开一个 PowerShell：

```powershell
cd E:\development\IM
. .\tools\go-env.ps1
$env:NEXUSIM_MESSAGE_SERVICE_MODE='grpc'
$env:NEXUSIM_GRPC_ADDR='127.0.0.1:11495'
$env:NEXUSIM_PG_DSN='postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable'
$env:NEXUSIM_CONVERSATION_SERVICE_ADDR='127.0.0.1:11496'
$env:NEXUSIM_CONVERSATION_RPC_TIMEOUT='200ms'
$env:NEXUSIM_MOCK_PERMISSION_VERSION='7'
bin\message-service.exe
```

成功日志：

```text
message-service using conversation-service at 127.0.0.1:11496
message-service gRPC server started on 127.0.0.1:11495
```

如果不设置 `NEXUSIM_CONVERSATION_SERVICE_ADDR`，`message-service` 会回退到 strict mock，便于复现历史压测。

当前 `message-service` 使用 gRPC lazy dial。也就是说，启动时能创建 client 不代表 `conversation-service` 一定可达；如果地址写错，通常会在第一批请求里表现为 `dependency unavailable`。本地联调时先确认 `conversation-service` 已输出启动日志，再启动 `message-service`。

在 `policy-service` 尚未实现前，发送权限仍来自 static policy mock。真实 conversation-service 联调时必须让：

```text
NEXUSIM_MOCK_PERMISSION_VERSION == conversations.permission_version
```

否则 `message-service` 会按设计返回 dependency version mismatch，这不是 conversation-service 查询失败。

## 7. 小规模 smoke

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
  --result-dir loadtest\results\conversation-smoke-manual
```

通过标准：

```text
success_rate = 1
error_count = 0
```

本 smoke 不启动 outbox relay，因此 `outbox_pending_count` 会等于成功写入数，这是预期现象。

## 8. 清理 smoke 数据

```powershell
$cleanup = @"
DELETE FROM message_outbox WHERE tenant_id = 'tenant-conv-smoke';
DELETE FROM conversation_timeline_events WHERE tenant_id = 'tenant-conv-smoke';
DELETE FROM message_log WHERE tenant_id = 'tenant-conv-smoke';
DELETE FROM conversation_seq WHERE tenant_id = 'tenant-conv-smoke';
DELETE FROM conversation_members WHERE tenant_id = 'tenant-conv-smoke';
DELETE FROM conversations WHERE tenant_id = 'tenant-conv-smoke';
"@
docker exec nexusim-postgres psql -U nexusim -d nexusim -v ON_ERROR_STOP=1 -c $cleanup
```

## 9. 常见问题

| 现象 | 处理 |
| --- | --- |
| `conversation read failed` | 检查 conversation 表是否已迁移，`conversation-service` 是否能连 PostgreSQL |
| `conversation not found` | 检查 tenant / conversation id 是否和 seed 数据一致 |
| `PermissionDenied: conversation member is not active` | 检查 `conversation_members` 是否包含当前 `user-<vu>`，且 status 为 `ACTIVE` |
| `permission version changed during send dependency read` | 检查 `NEXUSIM_MOCK_PERMISSION_VERSION` 是否等于 conversation 的 `permission_version` |
| PowerShell 管道执行 SQL 报 `syntax error at or near BEGIN` | 使用本文的 `docker cp` + `psql -f` 方式执行 migration |
