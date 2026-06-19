# NexusIM AI Eval Service-Stack Version / Hash-Only Expansion

Date: 2026-06-20

## Scope

This is a low-sensitive local service-stack eval expansion. It verifies
additional RAG / Agent regression cases against running local services and
records only summary references, counters and low-sensitive metadata in
`ai-eval-service`.

It is not a model-quality benchmark, production SLO, long-running eval platform
or capacity test.

## Command

```powershell
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -RunName ai-eval-service-stack-versioned-hash-only-20260620
```

Raw result root:

```text
H:\NexusIM\loadtest-results\ai-eval-service-stack-versioned-hash-only-20260620
```

Suite summary:

```text
H:\NexusIM\loadtest-results\ai-eval-service-stack-versioned-hash-only-20260620\ai-eval-regression-gate-summary.json
```

## Result

```text
status = passed
adapter_count = 4
case_count = 17
passed_count = 17
failed_count = 0
skipped_count = 0
selected_optional_adapters = rag-service, agent-action-executor
```

Adapter case counts:

```text
profile-agent-safety = 6
action-external-http-provider = 2
rag-service = 3
agent-action-executor = 6
```

## Newly Covered Cases

- `rag-versioned-grounding-contract`
- `agent-service-versioned-proposal-contract`
- `agent-action-hash-only-audit-contract`

## Verified Boundary

- RAG summary preserves `rag_version` and `retrieval_version`.
- Agent proposal summary preserves `agent_version` and `retrieval_version`.
- Agent proposal remains proposal-only before the default execution path.
- Safe local tool execution records input / output hashes only.
- `mcp_gateway_tool_call_audits`, `action_executor_execution_audits` and
  `action_executor_tool_results` expose no raw input / output payload columns in
  the smoke summary checks.

## Limits

This is still a positive service-stack regression expansion. RAG insufficient
evidence, Agent policy denial, external MCP unavailable, and richer model-output
negative cases remain future eval work.
