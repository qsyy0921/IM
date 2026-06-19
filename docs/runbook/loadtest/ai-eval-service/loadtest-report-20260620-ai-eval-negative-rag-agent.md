# AI Eval Service Stack Negative RAG / Agent Regression Smoke

Date: 2026-06-20

Command:

```powershell
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -RunName ai-eval-service-stack-negative-rag-agent-20260620
```

Scope:

- First-stage local AI eval service-stack regression gate.
- Selected optional adapters: `rag-service`, `agent-action-executor`.
- Not a production benchmark, model-quality benchmark, capacity run or HA proof.
- Raw summaries are stored under `H:\NexusIM\loadtest-results`; this report keeps only low-sensitive refs and counters.

Result:

```text
status=passed
adapter_count=4
case_count=19
passed_count=19
failed_count=0
skipped_count=0
```

Adapter breakdown:

| Adapter | Cases | Passed | Summary |
| --- | ---: | ---: | --- |
| profile-agent-safety | 6 | 6 | `H:\NexusIM\loadtest-results\ai-eval-service-stack-negative-rag-agent-20260620\profile-agent-safety-eval-summary.json` |
| action-external-http-provider | 2 | 2 | `H:\NexusIM\loadtest-results\ai-eval-service-stack-negative-rag-agent-20260620\action-external-http-eval-summary.json` |
| rag-service | 4 | 4 | `H:\NexusIM\loadtest-results\ai-eval-service-stack-negative-rag-agent-20260620\rag-eval-adapter-summary.json` |
| agent-action-executor | 7 | 7 | `H:\NexusIM\loadtest-results\ai-eval-service-stack-negative-rag-agent-20260620\agent-eval-adapter-summary.json` |

New regression coverage:

- `rag-insufficient-evidence-abstains`: `loadtest/rag -scenario no-evidence` seeds only visibility membership, calls real `rag-service`, and verifies `INSUFFICIENT_EVIDENCE`, empty EvidencePack, zero citations, `generated_by_llm=false`, and preserved `rag_version` / `retrieval_version`.
- `agent-policy-denied-blocks-proposal`: `loadtest/agent -scenario policy-denied` seeds a deny tool policy, calls real `agent-service`, and verifies `BLOCKED` proposal status, no approval, no execution, `TOOL_POLICY_DENIED`, and blocked MCP prepare audit with input hash only.

Key raw summaries:

- Gate: `H:\NexusIM\loadtest-results\ai-eval-service-stack-negative-rag-agent-20260620\ai-eval-regression-gate-summary.json`
- RAG no-evidence run: `H:\NexusIM\loadtest-results\ai-eval-service-stack-negative-rag-agent-20260620-rag-service-no-evidence\rag-answer-summary.json`
- Agent policy-denied run: `H:\NexusIM\loadtest-results\ai-eval-service-stack-negative-rag-agent-20260620-agent-action-executor-policy-denied\agent-proposal-summary.json`

Conclusion:

The service-stack gate now covers both positive version/hash-only paths and first negative RAG / Agent control-boundary paths. RAG abstains instead of fabricating citations when no visible evidence exists. Agent stops at MCP/policy prepare when a tool is denied and does not create approval or execution state.
