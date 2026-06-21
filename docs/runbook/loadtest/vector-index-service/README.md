# vector-index-service Smoke

本目录记录 `vector-index-service` 本地 smoke 入口。原始输出写到
`H:\NexusIM\loadtest-results`，仓库只保存 runbook 和必要报告。

## Outbox Relay Smoke

用途：

- 启动 `vector-index-service` gRPC 进程。
- 启动 `vector-index-service` outbox relay 进程。
- 启动 `vector-index-service` rebuild worker 进程。
- 通过公开 gRPC 执行 `UpsertVectorItem -> SearchVectors -> RequestVectorRebuild
  -> GetVectorIndexJob -> TombstoneVectorItem -> SearchVectors`。
- 验证 `vector_outbox` 生成 `vector.item.indexed.v1` 和 `vector.item.tombstoned.v1`。
- 验证 rebuild request 生成 rebuild job / checkpoint，并由 first-stage
  rebuild worker 推进到 completed。
- 验证 rebuild worker 生成 `vector.rebuild.started.v1` 和
  `vector.rebuild.completed.v1` 低敏 outbox event。
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

- 这是 relay / metadata / first-stage checkpoint worker smoke，不是 embedding worker、
  Milvus / pgvector / OpenSearch backend 或 provider backend rebuild smoke。
- runner 只写低敏 hash / ref，不写 raw document、message body、source URI、
  object key 或 embedding vector array。

## Embedding Worker Smoke

用途：

- 启动 `knowledge-ingestion-service` gRPC 进程。
- 启动 `model-gateway` gRPC 进程。
- 启动 `vector-index-service` gRPC 进程。
- 准备一个 knowledge source / ingestion job / chunk manifest。
- 启动 `vector-index-service` embedding worker，使用
  `knowledge-ingestion-service.ListKnowledgeChunks` 公开 API 拉取 redacted preview。
- 验证 `embedding-producer` 通过 knowledge public API 拉取 redacted preview 并写入
  PostgreSQL `vector_embedding_tasks` queue。
- 验证 `embedding-worker` 从 PostgreSQL queue claim task，调用
  `model-gateway.InvokeEmbedding` 后，通过 `vector-index-service` 公开
  `SearchVectors` 能检索到 knowledge chunk vector metadata。

运行：

```powershell
.\loadtest\vectorembedding\run-local-smoke.ps1
```

常用参数：

```powershell
.\loadtest\vectorembedding\run-local-smoke.ps1 `
  -PgDsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -ResultRoot "H:\NexusIM\loadtest-results"
```

当前边界：

- 这是 first-stage producer / PostgreSQL queue / worker / public API /
  model-gateway handoff smoke，不是 Kafka chunk consumer、provider backend rebuild 或
  Milvus / pgvector / OpenSearch backend smoke。
- runner 不手工调用 `UpsertVectorItem`，也不读其它服务私有表；验证只走
  `SearchVectors`。
- PostgreSQL / outbox / metrics / summary 不保存 raw document、source URI、object key
  或 embedding vector array。

## Knowledge Chunk Consumer Smoke

用途：

- 启动 `knowledge-ingestion-service` gRPC 进程。
- 准备一个 knowledge source / ingestion job / chunk manifest。
- 启动 `knowledge-ingestion-service` outbox relay，把低敏 `knowledge_outbox`
  发布到独立 Kafka `im.knowledge.events.<run>` topic。
- 启动 `vector-index-service` chunk-consumer，消费 `KnowledgeEvent` protobuf。
- 验证 consumer 跳过 `source.created` / `document.parsed`，只处理
  `knowledge.chunk.ready.v1`，通过 `ListKnowledgeChunks` 公开 API resolve redacted
  preview，并写入 PostgreSQL `vector_embedding_tasks` queue。

运行：

```powershell
.\loadtest\vectorembedding\run-local-chunk-consumer-smoke.ps1
```

常用参数：

```powershell
.\loadtest\vectorembedding\run-local-chunk-consumer-smoke.ps1 `
  -PgDsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -KafkaBrokers "localhost:9092" `
  -ResultRoot "H:\NexusIM\loadtest-results"
```

当前边界：

- 这是 `knowledge_outbox -> im.knowledge.events -> chunk-consumer ->
  vector_embedding_tasks` smoke，不启动 embedding worker，也不验证 vector search。
- runner 只验证 public API handoff 和 queue 状态；不读 knowledge 私表，不写 raw text、
  source URI、object key 或 embedding vector array。

## Optional pgvector Profile

用途：

- 为后续 `embedding-worker -> pgvector backend sink` focused smoke 提供本地
  pgvector PostgreSQL。
- 不替换默认 `nexusim-postgres`，不影响普通 metadata / outbox / queue smoke。

启动：

```powershell
docker compose `
  -f deploy\local\docker-compose.yml `
  -f deploy\local\docker-compose.pgvector.yml `
  --profile pgvector `
  up -d pgvector
```

连接串：

```text
postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable
```

Focused smoke：

```powershell
.\loadtest\vectorembedding\run-local-pgvector-smoke.ps1 `
  -StartPgVector `
  -PgDsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -PgVectorDsn "postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable" `
  -ResultRoot "H:\NexusIM\loadtest-results"
```

默认行为：

- 使用 `-StartPgVector` 时，脚本会先检查本机是否已有 `pgvector/pgvector:pg16` 镜像。
- `-StartPgVector` 且没有镜像时默认失败退出，不自动拉取，避免误耗外网流量。
- 如确实允许拉取，可显式加 `-AllowPull`。

embedding worker 相关环境变量：

```text
NEXUSIM_VECTOR_PROVIDER_BACKEND=pgvector
NEXUSIM_VECTOR_PGVECTOR_DSN=postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable
NEXUSIM_VECTOR_PGVECTOR_TABLE=vector_embedding_items
NEXUSIM_VECTOR_PGVECTOR_DIMENSION=8
NEXUSIM_VECTOR_PGVECTOR_ENSURE_SCHEMA=true
```

边界：

- `deploy/local/docker-compose.pgvector.yml` 是可选 overlay，默认不启动，也不在普通
  `check-local` 中拉取镜像。
- 该 profile 会在 dedicated pgvector backend table 中保存 embedding vector array；
  公开 API、metadata PostgreSQL、outbox、metrics 和 smoke summary 仍不得输出 raw
  vector array。
- 当前 README 只记录 profile 和 wiring；真实 pgvector focused smoke 报告后续单独归档。

## Optional Rebuild Provider Backfill

第一版 `rebuild-worker` 默认只推进 rebuild checkpoint / outbox。需要验证 provider
backend backfill 时必须显式开启：

```powershell
$env:NEXUSIM_VECTOR_INDEX_SERVICE_MODE = "rebuild-worker"
$env:NEXUSIM_VECTOR_REBUILD_BACKFILL_SOURCE = "embedding-tasks"
$env:NEXUSIM_VECTOR_REBUILD_BACKFILL_BATCH_SIZE = "100"
$env:NEXUSIM_MODEL_GATEWAY_GRPC_ADDR = "127.0.0.1:10770"
$env:NEXUSIM_VECTOR_PROVIDER_BACKEND = "postgres-test" # local metadata-backed verification
```

本地 focused smoke 可直接使用：

```powershell
.\loadtest\vectorembedding\run-local-smoke.ps1 `
  -IncludeRebuildBackfill `
  -PgDsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -ResultRoot "H:\NexusIM\loadtest-results"
```

该入口会：

- 先跑 `knowledge-ingestion-service -> embedding-producer -> vector_embedding_tasks
  -> embedding-worker -> model-gateway -> vector metadata`。
- 再启动 `rebuild-worker`，设置 `NEXUSIM_VECTOR_REBUILD_TENANT_ID` 限定当前 run tenant，
  避免本地历史 RUNNING/PENDING rebuild job 干扰 focused smoke。
- 使用 `postgres-test` provider backend 做本地 metadata-backed sink 验证，不保存 raw
  vector array。
- 设置 `NEXUSIM_VECTOR_REBUILD_BACKFILL_BATCH_SIZE=1`，验证 checkpoint cursor 分页续跑。

真实 pgvector backend smoke 时改用：

```powershell
$env:NEXUSIM_VECTOR_PROVIDER_BACKEND = "pgvector"
$env:NEXUSIM_VECTOR_PGVECTOR_DSN = "postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable"
```

边界：

- 只读取 `vector-index-service` 自有 `vector_embedding_tasks` 中 `COMPLETED` 的
  redacted-preview task。
- 重新通过 `model-gateway.InvokeEmbedding` 生成 embedding，再写当前配置的 provider backend。
- 不读取 knowledge / memory / search 私有表，不从 vector metadata 伪造缺失的 vector array。
- 未配置 provider backend 时 fail-fast；`postgres-test` 只确认 metadata backend state 和
  hash / dimension，不保存 raw vector array，不代表真实 vector store。
- 每批最多处理 `NEXUSIM_VECTOR_REBUILD_BACKFILL_BATCH_SIZE` 条 matching completed task；
  如果还有下一页，会推进 `vector_rebuild_checkpoints.cursor_value` 并等待下一轮继续 claim
  RUNNING rebuild；只有没有下一页时才标记 rebuild complete。
- embedding task queue 新写入使用 PostgreSQL `now()` 作为 `available_at / created_at /
  updated_at`，避免本地 / 双机开发时应用进程时间和数据库时间轻微漂移导致刚入队任务
  暂时不可 claim。
