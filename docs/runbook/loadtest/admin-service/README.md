# admin-service operator tools

本目录记录 `admin-service` 本地 operator 工具和后续 smoke。原始 summary / log
如后续生成，应写入 `H:\NexusIM\loadtest-results`；仓库只放说明和报告。

## Operator approval CLI

工具：

创建 operation：

```powershell
go run ./loadtest/admin -mode create `
  -target 127.0.0.1:10770 `
  -tenant-id tenant-admin `
  -operation-type CONFIG_PUBLISH `
  -target-ref-hash sha256:quota-target `
  -risk-level MEDIUM `
  -payload-schema-version admin.config_publish.v1 `
  -operation-payload-file H:\NexusIM\loadtest-results\admin\config-publish-payload.json `
  -operator-ref operator:alice `
  -reason-ref reason:ticket-123 `
  -evidence-refs evidence:ticket-123
```

审批 operation：

```powershell
go run ./loadtest/admin -mode approve `
  -target 127.0.0.1:10770 `
  -tenant-id tenant-admin `
  -operation-id admop_123 `
  -approver-ref operator:bob `
  -approver-role ADMIN `
  -reason-ref reason:ticket-123 `
  -evidence-refs evidence:ticket-123
```

支持模式：

```text
create
approve
reject
get
list
config-publish-smoke
```

边界：

- 只调用 `admin-service` 公开 gRPC API。
- 不读取 PostgreSQL 私表，也不直接执行业务 mutation。
- 输出低敏 JSON：operation / approval id、状态、hash/ref、时间戳；不输出
  `operation_payload_json`、reason 原文、EvidencePack 正文或下游 response body。
- `create` 可通过 `-operation-payload-file` 读取 payload；原始 payload 文件应放在
  `H:\NexusIM\loadtest-results`，不要放进仓库。
- 支持 `-admin-tls-*` 参数连接 TLS / mTLS gRPC 端点；不配置时用于本地 insecure
  smoke / operator 演示。

后续：

- 本地进程 smoke 可直接运行：

```powershell
.\loadtest\admin\run-local-smoke.ps1
```

- 该 smoke 会启动 `control-plane-service grpc`、`admin-service grpc` 和
  `admin-service operation-worker`，再通过公开 gRPC 执行
  `CreateAdminOperation -> operator approve -> operation-worker ->
  control-plane PublishConfigVersion -> GetConfigSnapshot`。
- 第一条真实下游 adapter 是非 critical `CONFIG_PUBLISH`，operation-worker 设置
  `NEXUSIM_CONTROL_PLANE_GRPC_ADDR` 后会调用
  `control-plane-service.PublishConfigVersion`；该 smoke 仍应通过公开 gRPC 创建和审批
  operation，不直接写数据库。
- 仍不替代 admin UI、审批系统或 provider-grade 运维平台。
