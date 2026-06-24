# retrieval-gateway vector backend smoke - 2026-06-24

本报告记录 `retrieval-gateway -> vector-index-service SearchVectors` 的最小真实进程
smoke。它验证 refs-only `VECTOR_ITEM` 能进入 EvidencePack，但不宣称 pgvector /
Milvus / OpenSearch vector、真实 embedding provider 或容量能力已完成。

## 运行

```powershell
. .\tools\go-env.ps1
.\loadtest\retrieval\run-local-smoke.ps1 `
  -IncludeVectorBackend `
  -RunName retrieval-vector-backend-smoke-20260624-223543
```

原始低敏 summary：

```text
H:\NexusIM\loadtest-results\retrieval-vector-backend-smoke-20260624-223543\retrieval-evidence-summary.json
```

## 进程链路

```text
search-service grpc
+ memory-service grpc
+ vector-index-service grpc
-> retrieval-gateway grpc
-> RetrieveEvidence(include_vector=true)
-> EvidencePack SEARCH_MESSAGE + MEMORY_EVENT + PROFILE_AGGREGATE + VECTOR_ITEM
```

## 关键结果

| 字段 | 值 |
| --- | --- |
| run_name | `retrieval-vector-backend-smoke-20260624-223543` |
| retrieval_target | `127.0.0.1:10590` |
| vector_target | `127.0.0.1:10760` |
| retrieval_version | `retrieval-gateway.v1.hybrid-source-vector-rrf-graph-depth1` |
| item_count | `4` |
| search_item_count | `1` |
| memory_item_count | `1` |
| profile_item_count | `1` |
| vector_item_count | `1` |
| vector_source_service | `memory-service` |
| vector_collection_type | `MEMORY_EVENT` |
| vector_no_raw_text | `true` |

已验证：

- search / memory / profile / vector source counts 均为 1。
- vector evidence 由 vector-index-service 公共 `UpsertVectorItem` seed，并通过
  retrieval-gateway 调用 vector-index-service 公共 `SearchVectors` 进入 EvidencePack。
- `VECTOR_ITEM` 只携带 vector item ref、source ref hash、source service、collection
  type、visibility version 和 tombstone status。
- `VECTOR_ITEM` 不携带 raw text 或 embedding vector。
- 既有 cross-group source refs、speaker attribution、memory graph edge、profile
  aggregate、temporal filtering 和 stale memory exclusion 未回归。

## 边界

- 本轮使用 vector-index-service 默认 PostgreSQL metadata / `postgres-test` backend state，
  不需要拉取 pgvector / Milvus / OpenSearch 镜像。
- 本轮没有验证真实 embedding provider、embedding-worker backfill、pgvector similarity
  search 或向量容量。
- RAG / summary / Agent 仍只能通过 retrieval-gateway 消费 EvidencePack；vector-index-service
  不直接服务 RAG 请求。
