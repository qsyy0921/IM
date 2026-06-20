# knowledge-ingestion -> vector-index Handoff Smoke

本目录记录知识导入到向量索引的本地 handoff smoke。原始输出写到
`H:\NexusIM\loadtest-results`，仓库只保存 runbook 和必要报告。

## Scope

验证内容：

- 启动 `knowledge-ingestion-service` gRPC 进程。
- 启动 `vector-index-service` gRPC 进程。
- 通过公开 gRPC 创建 knowledge source、提交本地 chunk manifest、列出 chunks。
- 使用 chunk 的低敏 metadata 调用 `vector-index-service.UpsertVectorItem`。
- 通过 `vector-index-service.SearchVectors` 验证两个 chunk 均可按同一
  visibility / policy 搜到。

不验证内容：

- 不启动真实 parser / embedding worker。
- 不调用 model provider。
- 不验证 Milvus / pgvector / OpenSearch 后端。
- 不消费 knowledge outbox，也不发布新的 production handoff event。

## Run

```powershell
.\loadtest\knowledgevector\run-local-smoke.ps1
```

常用参数：

```powershell
.\loadtest\knowledgevector\run-local-smoke.ps1 `
  -PgDsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -ResultRoot "H:\NexusIM\loadtest-results"
```

前置条件：

- PostgreSQL 可用。
- 本机 Go 工具链通过 `tools\go-env.ps1` 配置。

边界：

- runner 只使用公开 gRPC 串联两个服务。
- production code 不跨服务读私有表。
- 输出只记录 source / chunk / vector 的低敏 id 和 hash。
