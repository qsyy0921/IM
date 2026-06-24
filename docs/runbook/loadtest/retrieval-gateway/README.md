# retrieval-gateway Loadtest / Smoke Runbook

本目录记录 `retrieval-gateway` 的本地 smoke 结果。

当前 smoke 目标：

```text
search-service grpc + memory-service grpc
+ optional vector-index-service grpc
-> retrieval-gateway grpc
-> RetrieveEvidence
-> EvidencePack contains SEARCH_MESSAGE + MEMORY_EVENT + PROFILE_AGGREGATE
-> optional VECTOR_ITEM through vector-index-service SearchVectors
-> cross-group source refs preserved, stale / future memory excluded by query seq
```

第一版 smoke 不重复验证 Kafka timeline projection。`loadtest/retrieval` 会直接在
PostgreSQL 中 seed search / memory projection rows，然后通过三个真实 gRPC 进程
验证 retrieval-gateway 只经公开 search / memory API 聚合 EvidencePack。显式传入
`-IncludeVectorBackend` 时，脚本会额外启动 `vector-index-service`，由 runner 通过
公开 `UpsertVectorItem` seed 低敏 vector metadata，并要求 retrieval-gateway 通过
公开 `SearchVectors` 返回 refs-only `VECTOR_ITEM`。

运行入口：

```powershell
. .\tools\go-env.ps1
.\loadtest\retrieval\run-local-smoke.ps1

# opt-in vector backend path
.\loadtest\retrieval\run-local-smoke.ps1 -IncludeVectorBackend
```

默认原始 summary 输出到 `H:\NexusIM\loadtest-results`，仓库只保留报告。

报告：

- [2026-06-19 retrieval EvidencePack smoke](loadtest-report-20260619-retrieval-evidence-smoke.md)
- [2026-06-20 cross-group / temporal retrieval smoke](loadtest-report-20260620-cross-group-temporal-retrieval-smoke.md)
- [2026-06-24 retrieval vector backend smoke](loadtest-report-20260624-retrieval-vector-backend-smoke.md)
