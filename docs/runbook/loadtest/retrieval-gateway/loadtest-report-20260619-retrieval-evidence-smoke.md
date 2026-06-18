# retrieval-gateway EvidencePack Smoke - 2026-06-19

## Scope

本轮验证 `retrieval-gateway` 第一版 EvidencePack 边界：

```text
search-service grpc + memory-service grpc
-> retrieval-gateway grpc
-> RetrieveEvidence
-> EvidencePack
```

该 smoke 直接 seed search / memory projection rows 到 PostgreSQL，然后通过三个真实
gRPC 进程调用公开服务 API。它不重复验证 Kafka timeline projection；search-service
和 memory-service 的 projection smoke 已由各自 runbook 覆盖。

## Command

```powershell
. .\tools\go-env.ps1
.\loadtest\retrieval\run-local-smoke.ps1
```

## Raw Summary

```text
H:\NexusIM\loadtest-results\retrieval-gateway-evidence-smoke-20260619-024309\retrieval-evidence-summary.json
```

## Result

通过。

关键结果：

| Field | Value |
| --- | --- |
| run_name | `retrieval-gateway-evidence-smoke-20260619-024309` |
| retrieval_target | `127.0.0.1:10590` |
| query | `phoenix launch decision` |
| item_count | `2` |
| search_item_count | `1` |
| memory_item_count | `1` |
| search_projection_version | `17` |
| memory_projection_version | `23` |
| retrieval_version | `retrieval-gateway.v1` |

已验证：

- EvidencePack 包含 `SEARCH_MESSAGE` 和 `MEMORY_EVENT`。
- search evidence 带 `message_id`、`source_event_id`、`conversation_seq` 和 `visibility_version`。
- memory evidence 带 `memory_event_id`、source refs、validity window、`temporal_status`、`review_state` 和 `extraction_version`。
- source counts 与 search / memory projection version 被保留。

## Boundary Confirmed

本轮证明的是：

- `retrieval-gateway` 可以通过 search-service 和 memory-service 的公开 gRPC API 聚合 EvidencePack。
- EvidencePack 已能保留 RAG / summary / Agent 后续需要的最小 source attribution 和 temporal metadata。
- smoke runner 的原始输出仍在 `H:\NexusIM\loadtest-results`，没有把原始运行数据写进仓库。

本轮不证明：

- Kafka projection 端到端正确性。
- RAG 回答质量。
- policy-service 显式 retrieval check。
- 向量召回、rerank、Memory Graph 扩展。
- 生产级容量或 HA。

## Next

1. 将 retrieval-gateway 状态从 first boundary implementation 切到 first EvidencePack smoke passed。
2. 继续补 EvidencePack 字段 / policy hardening：rerank score、source coverage、dedupe reason、policy-service retrieval check。
3. 在 EvidencePack 边界稳定后进入 `rag-service` / `summary-service` / `agent-service`。
