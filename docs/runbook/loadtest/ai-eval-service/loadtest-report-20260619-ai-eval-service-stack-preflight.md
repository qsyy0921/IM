# ai-eval-service RAG / Agent Service-Stack Gate Preflight

Date: 2026-06-19

Scope: endpoint readiness preflight for the optional RAG / Agent service-stack
gate. This is not a live gate smoke and does not prove RAG / Agent adapter
execution.

Command:

```powershell
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -PreflightOnly `
  -AllowMissing `
  -RunName ai-eval-service-stack-preflight-20260619-after-reboot
```

Raw result root:

```text
H:\NexusIM\loadtest-results\ai-eval-service-stack-preflight-20260619-after-reboot
```

Observed result:

```text
status = missing
selected_optional_adapters = rag-service, agent-action-executor
ready = policy-service, postgres
missing = rag-service, retrieval-gateway, search-service, memory-service,
          agent-service, action-executor, mcp-gateway, skill-registry
```

Boundary:

- The preflight records endpoint readiness only.
- It stores no prompt, EvidencePack, model output, user content, secret, tool
  input or provider response body.
- The RAG / Agent live gate still requires the service stack to be started and
  then rerun without `-PreflightOnly` / `-AllowMissing`.
