# RAG-Agent group-memory answer / proposal gate - 2026-06-24

Scope: local low-sensitive service-stack gate for the RAG-Agent demo path. This is
not a production benchmark, capacity result, HA proof, or model-quality claim.

## Command

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -RunName ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1 `
  -RequestTimeout 30s
```

## Raw Results

- Gate result root: `H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1`
- Gate summary: `H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1\ai-eval-regression-gate-summary.json`
- RAG-Agent child result root: `H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1-rag-agent-demo`
- RAG-Agent summary: `H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1-rag-agent-demo\rag-agent-demo-summary.json`

## Gate Summary

- Status: `passed`
- Adapters: `4`
- Cases: `27`
- Passed: `27`
- Failed: `0`
- Skipped: `0`
- Optional adapter: `rag-agent-demo`

Adapter breakdown:

| Adapter | Cases | Passed | Failed | Skipped |
| --- | ---: | ---: | ---: | ---: |
| `profile-agent-safety` | 20 | 20 | 0 | 0 |
| `action-external-http-provider` | 2 | 2 | 0 | 0 |
| `action-external-mcp-failure` | 4 | 4 | 0 | 0 |
| `rag-agent-demo` | 1 | 1 | 0 | 0 |

## RAG-Agent Evidence

The `rag-agent-demo` adapter now verifies a multi-event group-memory scenario in
addition to the previously archived public candidate review, temporal update,
profile repair approval, and profile repair negative gates.

Confirmed low-sensitive facts from the summary:

- RAG answer status: `GROUNDED`
- Agent proposal status: `APPROVED`
- Agent approval recorded: `true`
- Action execution status: `RECORDED`
- Action executed: `false`
- Cross-group source refs preserved: `true`
- Speaker attribution preserved: `true`
- Memory graph edges preserved: `true`
- Profile aggregate preserved: `true`
- Group-memory answer verified: `true`
- Group-memory proposal verified: `true`
- Group-memory event count: `3`
- Group-memory RAG evidence count: `3`
- Group-memory Agent evidence count: `3`
- Group-memory event types: `DECISION`, `BLOCKER`, `FILE`
- Group-memory source ref count: `6`
- Group-memory cross-group source ref count: `3`

The runner constructs the group-memory scenario only through public service
contracts:

```text
memory-service SubmitMemoryCandidate
-> ReviewMemoryCandidate(APPROVE)
-> rag-service AnswerQuestion
-> agent-service CreateAgentProposal
-> action-executor audit path
```

No private table reads are used by the RAG-Agent runner. The report stores hashes,
counts, ids, statuses, and summary refs only; it does not store raw answer text,
raw proposal text, raw EvidencePack payload, prompts, model output, tool input, or
secrets.

## Boundary Confirmed

- `memory-service` owns structured group-memory facts and review state.
- `retrieval-gateway` remains the EvidencePack boundary.
- `rag-service` only answers from EvidencePack.
- `agent-service` only proposes from EvidencePack and approval state.
- Real tool execution remains gated by proposal / approval / action-executor audit.
- Missing or mismatched approval still fails closed through the existing profile
  repair negative gate.

## Next Step

Continue the AI / Agent demo path by deepening EvidencePack source-chain coverage
and real-business proposal scenarios. Do not expand the client UI unless it blocks
the backend / AI demonstration.
