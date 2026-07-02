# NexusIM Agent Interview Demo

This demo is a backend-isolated Agent-layer walkthrough. It proves that an
Agent run can be explained, evaluated and replayed from low-sensitive refs
without reading real IM data or calling production services.

Run:

```powershell
python ai/python/scripts/run_agent_interview_demo.py ai/python/fixtures/agent_eval/interview_send_message_agent_demo.json
```

Optional artifacts:

```powershell
python ai/python/scripts/run_agent_interview_demo.py `
  ai/python/fixtures/agent_eval/interview_send_message_agent_demo.json `
  --summary-out .tmp-agent-interview-demo/summary.json `
  --report-out .tmp-agent-interview-demo/eval-report.json `
  --replay-out .tmp-agent-interview-demo/replay-bundle.json `
  --force
```

## What It Proves

Flow:

```text
synthetic IM-like request
-> MessageCommitted / SendMessageTrace refs
-> EvidencePack refs
-> ContextPackage refs
-> MemoryCandidate refs
-> ToolIntent / proposal refs
-> approval/action fixture refs
-> EvalReport
-> ReplayBundle
```

The Agent is outside the SendMessage hot path. SendMessage is represented only
by `send_message_trace_ref` and `message_committed_ref`; the demo starts after
message commit and reports `hot_path_impact = none`.

EvidencePack is the AI read boundary. The fixture contains visible message refs,
forbidden message refs and coverage-gap refs. The ContextPackage refs are
derived from allowed refs only, so denied evidence is reported without leaking
private message refs into model input.

MemoryCandidate is not ACTIVE memory. Python can produce a candidate or a
needs-review result, but this demo never admits memory. In production,
memory-service would own ACTIVE admission, review, revocation and retrieval
eligibility.

ToolIntent is not execution. The demo can prepare or block a tool proposal, but
it never performs side effects. Approval and action fields are fixture refs
only. In production, workflow-service would own durable waits and
action-executor would own execution.

EvalReport and ReplayBundle make the run reviewable. EvalReport summarizes
quality and safety checks. ReplayBundle keeps low-sensitive refs and hashes so
operators can inspect behavior without re-running external side effects or
returning raw payloads.

## Covered Cases

- normal release-delay summary with a follow-up task proposal;
- denied evidence that must not enter ContextPackage;
- group memory candidate requiring review;
- unsafe tool/provider output blocked before execution;
- approval-required proposal that is not executed directly.

## Future Production Work

This is not production integration. Production still needs accepted ADRs,
owner-approved contracts, real-service smoke tests, auth/policy/audit wiring,
operator UX, SLOs, rollback plans and security review.
