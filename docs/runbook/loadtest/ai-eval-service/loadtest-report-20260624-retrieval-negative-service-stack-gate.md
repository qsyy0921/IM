# ai-eval-service retrieval negative service-stack gate

Date: 2026-06-24

Scope: close the `retrieval-gateway` negative / miss gap from the previous
collaborative-memory live gate. This report is low-sensitive and records only
adapter counts, case ids and summary paths. Raw smoke JSON stays under
`H:\NexusIM\loadtest-results`.

## Commands

```powershell
. .\tools\go-env.ps1
go test ./loadtest/retrievalnegative ./loadtest/retrieval ./services/retrieval-gateway/... -count=1

.\tools\run-ai-eval-retrieval-negative-adapter.ps1 `
  -RunName ai-eval-retrieval-negative-live-20260624 `
  -OutputPath H:\NexusIM\loadtest-results\ai-eval-retrieval-negative-live-20260624\retrieval-negative-eval-adapter-summary.json

.\tools\run-ai-eval-retrieval-adapter.ps1 `
  -RunName ai-eval-retrieval-positive-live-20260624 `
  -OutputPath H:\NexusIM\loadtest-results\ai-eval-retrieval-positive-live-20260624\retrieval-eval-adapter-summary.json

.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter retrieval-gateway,retrieval-gateway-negative `
  -RunName ai-eval-retrieval-gate-live-20260624

.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter memory-service,retrieval-gateway,retrieval-gateway-negative,rag-service,summary-service,agent-action-executor `
  -RunName ai-eval-service-stack-live-20260624-retrieval-negative
```

## Result

Focused retrieval gate:

```text
status=passed
adapter_count=5
case_count=31
passed_count=31
failed_count=0
skipped_count=0
selected_optional_adapters=retrieval-gateway,retrieval-gateway-negative
```

Full AI service-stack gate:

```text
status=passed
adapter_count=9
case_count=51
passed_count=51
failed_count=0
skipped_count=0
selected_optional_adapters=memory-service,retrieval-gateway,retrieval-gateway-negative,rag-service,summary-service,agent-action-executor
```

Raw summaries:

- `H:\NexusIM\loadtest-results\ai-eval-retrieval-negative-live-20260624\retrieval-negative-eval-adapter-summary.json`
- `H:\NexusIM\loadtest-results\ai-eval-retrieval-positive-live-20260624\retrieval-eval-adapter-summary.json`
- `H:\NexusIM\loadtest-results\ai-eval-retrieval-gate-live-20260624\ai-eval-regression-gate-summary.json`
- `H:\NexusIM\loadtest-results\ai-eval-service-stack-live-20260624-retrieval-negative\ai-eval-regression-gate-summary.json`

## Cases closed

The new `retrieval-gateway-negative` adapter covers four active retrieval cases
that were previously skipped by the positive EvidencePack adapter:

- `retrieval-source-coverage-empty-memory`: memory source coverage is explicitly
  `EMPTY` while search still returns evidence.
- `retrieval-temporal-superseded-filter`: active memory is returned and
  superseded memory is absent.
- `retrieval-attribution-source-ref-required`: returned memory evidence carries
  `MESSAGE` source refs and a valid dedupe reason.
- `retrieval-cross-tenant-permission-deny`: cross-tenant auth returns no
  `SEARCH_MESSAGE` evidence leak.

## Boundaries

- This is a first-stage service-stack eval gate, not a production model-quality
  benchmark.
- The negative adapter seeds low-sensitive search / memory projection fixtures
  and calls the real `retrieval-gateway` gRPC API.
- It does not persist raw EvidencePack, prompts, model output, user content,
  secrets or tool input.
- Cross-tenant isolation is accepted when the request either returns
  `PermissionDenied` or produces an empty EvidencePack with no evidence leak;
  it does not claim a provider-grade ReBAC policy engine is complete.
