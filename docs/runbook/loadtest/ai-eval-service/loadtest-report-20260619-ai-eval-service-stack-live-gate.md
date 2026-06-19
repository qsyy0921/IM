# ai-eval-service RAG / Agent Service-Stack Live Gate

Date: 2026-06-19

Scope: first local live gate for `ai-eval-service` optional RAG / Agent service
stack adapters. This is a local regression gate smoke, not a model-quality
benchmark, long-running eval platform or production SLO.

Command:

```powershell
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -RunName ai-eval-service-stack-gate-smoke-20260619-live-wrapper
```

Raw result root:

```text
H:\NexusIM\loadtest-results\ai-eval-service-stack-gate-smoke-20260619-live-wrapper
```

Runtime notes:

- Built current local Docker images for `search-service`, `memory-service`,
  `retrieval-gateway`, `rag-service`, `agent-service`, `skill-registry`,
  `mcp-gateway`, `action-executor`, `summary-service` and `policy-service`.
- `agent-service` and `summary-service` runtime Dockerfiles were changed from
  remote distroless base image to `scratch`, matching the other static Go
  service images and avoiding external base-image pulls.
- `policy-service-grpc` local compose now enables
  `NEXUSIM_POLICY_RULES_ENABLED=true`, so tool-policy rules seeded by the
  Agent smoke are evaluated instead of falling back to static deny.

Observed result:

```text
status = passed
adapter_count = 4
case_count = 8
passed_count = 8
failed_count = 0
skipped_count = 0
selected_optional_adapters = rag-service, agent-action-executor
```

Adapters:

- `profile-agent-safety`: 2 / 2 passed.
- `action-external-http-provider`: 2 / 2 passed.
- `rag-service`: 1 / 1 passed.
- `agent-action-executor`: 3 / 3 passed.

Verified boundary:

- `rag-service` returned a grounded answer with citations and source coverage
  from `retrieval-gateway` / EvidencePack.
- `agent-service` produced an approval-required proposal with citations.
- `action-executor` verified the approved proposal path, wrote low-sensitive
  execution audit and covered both non-executed external action and safe local
  tool output-hash projection.
- `ai-eval-service` recorded every adapter summary and read it back through
  `GetEvalRun` / `ListEvalRuns`.

Data boundary:

- The gate summary stores only low-sensitive counters, adapter names, run ids,
  suite/stage ids and summary refs.
- It does not persist raw prompt, raw EvidencePack, model output, user message
  body, secret, raw tool input or provider response body.

Next:

- Add a CI / local-gate skeleton around this policy-driven gate.
- Continue expanding low-sensitive RAG / Agent regression cases without turning
  local smoke results into production SLO claims.
