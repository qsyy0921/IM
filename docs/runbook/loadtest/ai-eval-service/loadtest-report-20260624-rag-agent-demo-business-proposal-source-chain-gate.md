# RAG-Agent business proposal source-chain gate - 2026-06-24

## Scope

This run validates the next RAG-Agent demo capability:

```text
reviewed group memory -> EvidencePack -> Agent business proposal
-> approval -> action-executor audit
```

It does not execute a real business mutation. The selected business tool is
`conversation.note.create`; because no explicit mutation adapter is configured,
the expected result is an action-executor audit record with
`business_action_executed=false`.

## Command

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -RunName ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1 `
  -RequestTimeout 30s
```

## Result

```text
run_name: ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1
status: passed
adapter_count: 4
case_count: 27
passed_count: 27
failed_count: 0
skipped_count: 0
```

Raw result root:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1
```

Gate summary:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1\ai-eval-regression-gate-summary.json
```

RAG-Agent child summary:

```text
H:\NexusIM\loadtest-results\ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1-rag-agent-demo\rag-agent-demo-summary.json
```

## Verified Facts

The `rag-agent-demo` summary includes:

```text
business_proposal_verified: true
business_proposal_approval_recorded: true
business_action_audit_recorded: true
business_action_executed: false
business_execution_status: RECORDED
business_proposal_memory_event_count: 3
business_proposal_evidence_memory_count: 3
business_proposal_source_ref_count: 6
business_proposal_cross_group_source_ref_count: 3
business_proposal_event_types: DECISION, TASK, STATUS
business_tool_name: conversation.note.create
business_skill_id: conversation.note.create
```

The new adapter assertion is:

```text
business_proposal_must_preserve_source_chain_and_audit_boundary
```

## Boundary Confirmed

- Agent still creates a proposal only; it does not write business facts directly.
- Approval is recorded through `ApproveAgentProposal`.
- action-executor records the approved action audit.
- Unconfigured business mutation remains unexecuted.
- Summary output stores hashes, counts and ids only; it does not store raw
  proposal text, raw EvidencePack text, raw model output or raw tool output.

## Next

Continue the AI / Agent demo path by deepening EvidencePack source-chain and
rerank coverage. Real mutation scenarios require an explicit business adapter,
policy, approval, executor and audit boundary.
