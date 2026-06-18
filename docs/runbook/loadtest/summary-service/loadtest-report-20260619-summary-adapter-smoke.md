# summary-service Adapter Smoke 2026-06-19

## 结论

本轮真实本地 adapter smoke 通过，证明当前 `summary-service` 能通过
`retrieval-gateway` 获取 `EvidencePack`，生成 grounded deterministic
summary，并保留 search / memory 两类 evidence 和 citations。

这不是 LLM provider 质量评测，也不是生产容量测试。

## 环境

- 运行时间：2026-06-19 本地时间
- 原始结果目录：`H:\NexusIM\loadtest-results\summary-adapter-smoke-20260618-212729`
- Summary target：`127.0.0.1:10620`
- 运行服务：
  - search-service `grpc`
  - memory-service `grpc`
  - retrieval-gateway `grpc`
  - summary-service `grpc`

## 命令

```powershell
.\tools\run-summary-adapter-smoke.ps1 `
  -PGDSN "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -SummaryTarget "127.0.0.1:10620" `
  -RunName "summary-adapter-smoke-20260618-212729"
```

## 关键结果

- `run_name`: `summary-adapter-smoke-20260618-212729`
- `summary_status`: `GROUNDED`
- `generated_by_llm`: `false`
- `citation_count`: `2`
- `evidence_item_count`: `2`
- `search_item_count`: `1`
- `memory_item_count`: `1`
- `source_counts.search_message`: `1`
- `source_counts.memory_event`: `1`
- `summary_version`: `summary-service.v1`
- `retrieval_version`: `retrieval-gateway.v1`

## 验证点

- summary 保留 source citation，并能追溯到 message source ref。
- 返回的 EvidencePack 同时包含 search evidence 和 memory evidence。
- memory evidence 保持 `ACTIVE` temporal status、source ref 和 projection
  version。
- `source_coverage` / projection version 沿链路保留。
- 第一阶段 summary 是 deterministic extractive summary，
  `generated_by_llm=false`。

## 限制

- 未接入真实 LLM provider。
- 未验证外部 prompt boundary、token budget、PII / secret filter。
- 未验证 Agent 消费 summary。
- 未验证生产性能、长压或多实例容量。
