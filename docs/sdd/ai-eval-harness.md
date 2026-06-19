# AI Eval Harness SDD v0.1

## Positioning

The first-stage AI eval harness is a local validation layer for NexusIM's AI
foundation. It is not a production `ai-eval-service` yet. It protects the
transition from retrieval-gateway to RAG / summary / Agent by keeping failure
classes explicit and versioned.

## Scope

First-stage cases cover:

- retrieval miss: a missing source is visible through `source_coverage`;
- temporal version: active evidence must not be confused with superseded facts;
- attribution: answers and actions need source refs;
- permission leak: policy / visibility failures must not return evidence;
- profile overgeneralization: group facts must not become unsupported personal
  memory;
- tool policy violation: high-risk tool calls require approval;
- action execution safety: Agent actions must go through proposal / approval /
  executor / audit.

## Artifacts

- Case file: `docs/runbook/ai-eval/retrieval-eval-cases.json`
- Validator: `tools/validate-ai-eval-cases.ps1`
- Execution adapters:
  - `tools/run-ai-eval-rag-adapter.ps1`
  - `tools/run-ai-eval-agent-adapter.ps1`
- Runbook: `docs/runbook/ai-eval/README.md`

The case file is low-sensitive and synthetic. It stores expected assertions,
not raw private messages, model prompts, provider responses or production
traffic.

## Non-Goals

- No LLM provider integration in v0.1.
- No production benchmark claims.
- No long-running eval dashboard.
- No online hot-path dependency.

## Next Step

RAG / summary / Agent implementations must consume EvidencePack and can add
execution adapters that evaluate these cases against real outputs. Any future
`ai-eval-service` should reuse this case taxonomy rather than inventing a
separate safety vocabulary.

2026-06-19 update: first-stage Agent execution adapter is available. It runs
real `loadtest/agent` against `agent-service` and `action-executor`, then
validates active Agent / action-executor cases for approval-required behavior,
approved proposal verification, low-sensitive execution audit recording and
`executed=false`. It is still local safety evidence, not a production benchmark
or real external tool execution.
