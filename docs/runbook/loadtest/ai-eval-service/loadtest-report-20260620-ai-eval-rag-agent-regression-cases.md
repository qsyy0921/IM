# ai-eval-service RAG / Agent Regression Case Expansion

Date: 2026-06-20

Scope: low-sensitive local regression expansion for RAG / Agent service-stack
adapters. This is a local gate smoke, not a model-quality benchmark, long-running
eval platform or production SLO.

Command:

```powershell
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -RunName ai-eval-rag-agent-regression-cases-20260620
```

Raw result root:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-regression-cases-20260620
```

Observed result:

```text
status = passed
adapter_count = 4
case_count = 10
passed_count = 10
failed_count = 0
skipped_count = 0
selected_optional_adapters = rag-service, agent-action-executor
```

New coverage:

- `rag-evidencepack-source-coverage-projection`: verifies RAG preserves
  EvidencePack source coverage, search / memory counts and projection versions.
- `agent-prepare-audit-policy-boundary`: verifies Agent proposal creation
  preserves `mcp-gateway` prepare audit, tool policy metadata, EvidencePack
  source coverage and projection versions before approval / execution.

Data boundary:

- The gate summary stores only low-sensitive counters, adapter names, run ids,
  suite / stage ids and summary refs.
- It does not persist raw prompt, raw EvidencePack, model output, user message
  body, secret, raw tool input or provider response body.

Next:

- Continue expanding RAG / Agent regression cases around retrieval failure,
  reasoning failure and action boundary failure.
