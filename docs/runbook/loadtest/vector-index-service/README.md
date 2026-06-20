# vector-index-service Smoke

本目录记录 `vector-index-service` 本地 smoke 入口。原始输出写到
`H:\NexusIM\loadtest-results`，仓库只保存 runbook 和必要报告。

## Outbox Relay Smoke

用途：

- 启动 `vector-index-service` gRPC 进程。
- 启动 `vector-index-service` outbox relay 进程。
- 通过公开 gRPC 执行 `UpsertVectorItem -> SearchVectors -> TombstoneVectorItem -> SearchVectors`。
- 验证 `vector_outbox` 生成 `vector.item.indexed.v1` 和 `vector.item.tombstoned.v1`。
- 从 Kafka `im.vector.events.<run>` 读取 `VectorEvent`，确认 relay 发布成功。
- 检查 outbox / Kafka payload 不包含 raw text、source URI、object key、向量数组或 secret 类字段。

运行：

```powershell
.\loadtest\vectorindex\run-local-smoke.ps1
```

常用参数：

```powershell
.\loadtest\vectorindex\run-local-smoke.ps1 `
  -PgDsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -KafkaBrokers "localhost:9092" `
  -ResultRoot "H:\NexusIM\loadtest-results"
```

前置条件：

- `nexusim-postgres` 容器可用。
- `nexusim-kafka` 容器可用。
- 本机 Go 工具链通过 `tools\go-env.ps1` 配置。

当前边界：

- 这是 relay / metadata smoke，不是 embedding worker、Milvus / pgvector /
  OpenSearch backend smoke。
- runner 只写低敏 hash / ref，不写 raw document、message body、source URI、
  object key 或 embedding vector array。
