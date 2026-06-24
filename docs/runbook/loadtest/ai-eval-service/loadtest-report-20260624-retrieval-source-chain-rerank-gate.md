# ai-eval-service retrieval source-chain rerank gate

Date: 2026-06-24

Scope: prove the retrieval-gateway source-chain-aware rerank first pass against
the real local service stack. This report is low-sensitive and records only
adapter counts, assertion names, summary paths and score values. Raw smoke JSON
stays under `H:\NexusIM\loadtest-results`.

## Commands

```powershell
.\tools\build-service-docker-images.ps1 `
  -Services retrieval-gateway `
  -Platform linux/amd64

docker compose -f deploy\local\docker-compose.services.yml `
  up -d --no-deps --force-recreate retrieval-gateway-grpc

.\tools\run-ai-eval-retrieval-adapter.ps1 `
  -RunName retrieval-source-chain-rerank-adapter-check-20260624 `
  -RetrievalTarget 127.0.0.1:10590

.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter retrieval-gateway `
  -RunName ai-eval-service-stack-live-20260624-retrieval-source-chain-rerank-v2
```

## Result

```text
status=passed
adapter_count=4
case_count=27
passed_count=27
failed_count=0
skipped_count=0
selected_optional_adapters=retrieval-gateway
```

Retrieval adapter assertion result:

```text
case=retrieval-gateway-current-memory-live-preserves-chain
assertions=9
passed=9
source_chain_rerank_preserved=true
search_rerank_score=1.00
memory_rerank_score=1.29
item_count=3
search_item_count=1
memory_item_count=1
profile_item_count=1
```

Raw summaries:

- `H:\NexusIM\loadtest-results\retrieval-source-chain-rerank-adapter-check-20260624\retrieval-eval-adapter-summary.json`
- `H:\NexusIM\loadtest-results\ai-eval-service-stack-live-20260624-retrieval-source-chain-rerank-v2\retrieval-eval-adapter-summary.json`
- `H:\NexusIM\loadtest-results\ai-eval-service-stack-live-20260624-retrieval-source-chain-rerank-v2\retrieval-gateway-record-summary.json`
- `H:\NexusIM\loadtest-results\ai-eval-service-stack-live-20260624-retrieval-source-chain-rerank-v2\ai-eval-regression-gate-summary.json`

## Cases Closed

The `retrieval-gateway` positive adapter now verifies that:

- EvidencePack source coverage includes search, memory and profile evidence.
- Current memory filtering excludes expired, superseded and future memory rows.
- Cross-group source refs and speaker attribution are preserved.
- Multi-hop actor chain and complete source-chain evidence are present before
  RAG / Agent consumption.
- Source-chain-aware rerank lifts multi-source memory evidence above a single
  search hit.
- Search and memory projection versions are preserved.

## Boundaries

- This is a first-stage local service-stack eval gate, not a production model
  quality benchmark or capacity test.
- The gate uses low-sensitive synthetic data and calls the real
  `retrieval-gateway` gRPC API after rebuilding and recreating the local
  retrieval-gateway container.
- It does not persist raw EvidencePack, prompts, model output, user content,
  secrets or tool input.
- The gate does not introduce a new fallback path; retrieval still fails closed
  when source services or source-chain expansion are unavailable.
