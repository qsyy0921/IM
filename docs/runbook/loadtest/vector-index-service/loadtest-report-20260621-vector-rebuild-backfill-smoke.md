# vector-index-service rebuild provider backfill smoke

日期：2026-06-21

范围：本地 focused smoke，验证 `embedding-tasks` rebuild backfill 可以从
`vector_embedding_tasks` 中读取已完成的 redacted preview task，重新经过
`model-gateway.InvokeEmbedding`，再写入显式配置的 provider backend。

## 运行入口

```powershell
.\loadtest\vectorembedding\run-local-smoke.ps1 `
  -IncludeRebuildBackfill `
  -PgDsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -ResultRoot "H:\NexusIM\loadtest-results"
```

原始结果目录：

```text
H:\NexusIM\loadtest-results\vector-embedding-producer-smoke-20260621-101900
```

## 结果

通过。

关键摘要：

```text
phase=request-rebuild
tenant_id=tenant-vector-embedding-producer-smoke-20260621-101900
embedding_task_count=2
embedding_task_completed=2
rebuild_job_status=INDEXED
rebuild_checkpoint_status=COMPLETED
rebuild_cursor_value=embedding-task:knowledge-chunk:kchk_7b1b22decf339287f3f1315c_000:deterministic-embedding-v1
```

## 覆盖点

- `knowledge-ingestion-service` 通过公开 gRPC 准备 knowledge source / job / chunk
  manifest。
- `embedding-producer` 通过公开 `ListKnowledgeChunks` 读取 redacted preview，并写入
  `vector_embedding_tasks`。
- `embedding-worker` 从 PostgreSQL queue claim task，调用
  `model-gateway.InvokeEmbedding` 后写入 vector metadata。
- `rebuild-worker` 使用 `NEXUSIM_VECTOR_REBUILD_BACKFILL_SOURCE=embedding-tasks`
  读取 completed queue task，重新 embedding 并写 provider backend。
- `NEXUSIM_VECTOR_REBUILD_TENANT_ID` 限定当前 run tenant，避免本地历史 rebuild job
  干扰 focused smoke。
- `NEXUSIM_VECTOR_REBUILD_BACKFILL_BATCH_SIZE=1` 验证 checkpoint cursor 分页续跑。

## 边界

- 本轮使用 `NEXUSIM_VECTOR_PROVIDER_BACKEND=postgres-test`，只验证 owned backend
  metadata state / hash / dimension，不保存 raw vector array。
- 本轮不代表 pgvector / Milvus / OpenSearch 真后端 smoke。
- 公开 API、metadata PostgreSQL、outbox、metrics 和 summary 不输出 raw document、
  source URI、object key 或 embedding vector array。
