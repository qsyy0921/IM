# rag-service RAG Adapter Smoke 2026-06-19

## 结论

本轮真实本地 adapter smoke 通过，证明当前 `rag-service` 能通过
`retrieval-gateway` 获取 `EvidencePack`，返回 grounded deterministic answer，
并保留 search / memory 两类 evidence 和 citations。

这不是 LLM provider 质量评测，也不是生产容量测试。

## 环境

- 运行时间：2026-06-19 本地时间
- 原始结果目录：`H:\NexusIM\loadtest-results\rag-eval-adapter-20260618-203946`
- RAG target：`127.0.0.1:10610`
- 运行服务：
  - search-service `grpc`
  - memory-service `grpc`
  - retrieval-gateway `grpc`
  - rag-service `grpc`

## 命令

```powershell
.\tools\run-ai-eval-rag-adapter.ps1 `
  -PGDSN "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -RAGTarget "127.0.0.1:10610"
```

## 关键结果

- `run_name`: `rag-eval-adapter-20260618-203946`
- `answer_status`: `GROUNDED`
- `generated_by_llm`: `false`
- `citation_count`: `2`
- `evidence_item_count`: `2`
- `search_item_count`: `1`
- `memory_item_count`: `1`
- `source_counts.search_message`: `1`
- `source_counts.memory_event`: `1`
- AI eval adapter active case count：`1`
- Active case：`rag-grounded-answer-citations`
- 断言全部通过：
  - `answer_status`
  - `must_include_citation`
  - `must_return_source_type`
  - `must_not_claim_llm_generation`

## 验证点

- answer 保留 source citation，并能追溯到 message source ref。
- 返回的 EvidencePack 同时包含 search evidence 和 memory evidence。
- memory evidence 保持 `ACTIVE` temporal status、source ref 和 projection version。
- `source_coverage` / projection version 沿链路保留。
- 第一阶段 answer 是 deterministic extractive answer，`generated_by_llm=false`。

## 修复记录

首次运行暴露 `loadtest/rag` seed 使用了不符合 memory schema 的
`review_state='REVIEWED'`。已修正为 `APPROVED`，与
`memory_structured_events` 约束和 memory proto 状态保持一致。

## 限制

- 未接入真实 LLM provider。
- 未验证 citation verifier。
- 未验证 summary / Agent 消费 EvidencePack。
- 未验证生产性能、长压或多实例容量。
