# Agent Demo Codex Implementation Playbook

Date: 2026-07-02

Status: implementation playbook for the interview-ready backend-isolated Agent
demo. This is not an ADR, proto, OpenAPI, Kafka schema, migration, production
service directory, production runtime, real backend integration or release
pipeline.

## Verdict

The current SDD package is not enough to let Codex automatically write the
whole NexusIM production project.

It is enough to let Codex implement one scoped module: an interview-ready,
backend-isolated Agent demo runner with tests and a concise README.

Reason: Codex works best when a task has a clear goal, context, constraints,
validation loop and stopping condition. The SDDs describe architecture and
boundaries, but they do not freeze production contracts or authorize real
service integration. This playbook translates the SDD package into one concrete
implementation target.

## Goal Prompt

Use this as the stable Codex goal for the next implementation phase:

```text
Complete an interview-ready backend-isolated NexusIM Agent demo without opening
production integration. Implement one command that consumes a synthetic
IM-like fixture and emits low-sensitive refs for MessageCommitted, EvidencePack,
ContextPackage, MemoryCandidate, ToolIntent / proposal, approval/action fixture
refs, EvalReport and ReplayBundle. Stop only when focused tests, full
ai/python tests, ruff, mypy, Python AI worker boundary guard, runbook
consistency and git diff checks pass, or when a hard boundary would be violated.
```

## Source Documents To Read First

Every implementation turn should read:

1. `prompt.md`
2. `agent.md`
3. `docs/runbook/current-goal.md`
4. `docs/research/agent-architecture-scope-closure-20260702.md`
5. `docs/research/agent-sdd-interview-readiness-review-20260702.md`
6. this playbook

Read SDDs only when touching their concern:

| Concern | Read |
| --- | --- |
| Runtime trace | `docs/sdd/agent-runtime.md` |
| Context / evidence | `docs/sdd/agent-context-evidencepack.md` |
| Memory candidate | `docs/sdd/agent-memory-admission.md` |
| Tool intent / MCP safety | `docs/sdd/agent-tool-mcp-boundary.md` |
| Eval / replay output | `docs/sdd/agent-eval-replay-harness.md` |
| Release-gate refs | `docs/sdd/agent-governance-agentops.md` |

## Allowed Implementation Scope

Allowed paths:

```text
ai/python/nexusim_ai_eval/
ai/python/nexusim_ai_common/
ai/python/nexusim_ai_memory/
ai/python/fixtures/agent_eval/
ai/python/scripts/
ai/python/tests/
docs/research/
docs/runbook/
```

Do not touch:

```text
proto/
api/
deploy/
docker/
services/
loadtest/
database migrations
production service registry
production startup paths
```

Do not connect to:

```text
PostgreSQL
Kafka
Redis
OpenSearch
real workflow-service
real memory-service
real action-executor
real audit-service
real MCP provider
real model provider
real NexusIM IM data
```

## Target Command

Add one command:

```powershell
python ai/python/scripts/run_agent_interview_demo.py ai/python/fixtures/agent_eval/interview_send_message_agent_demo.json
```

Optional output paths:

```powershell
python ai/python/scripts/run_agent_interview_demo.py `
  ai/python/fixtures/agent_eval/interview_send_message_agent_demo.json `
  --report-out .tmp-agent-interview-demo/eval-report.json `
  --replay-out .tmp-agent-interview-demo/replay-bundle.json `
  --summary-out .tmp-agent-interview-demo/summary.json `
  --force
```

The command must not require network, model credentials, backend services or
production data.

## Expected Demo Flow

```text
synthetic IM-like request
-> MessageCommittedRef / SendMessageTraceRef fixture
-> EvidencePack refs
-> ContextPackage refs
-> MemoryCandidate refs
-> ToolIntent / proposal refs
-> approval/action fixture refs
-> EvalReport
-> ReplayBundle
```

This flow demonstrates that SendMessage remains a backend hot path concern and
Agent work happens asynchronously from refs.

## Fixture Shape

Create:

```text
ai/python/fixtures/agent_eval/interview_send_message_agent_demo.json
```

Required top-level fields:

```json
{
  "fixture_kind": "agent_interview_demo",
  "schema_version": 1,
  "demo_id": "interview-send-message-agent-demo-v1",
  "send_message_trace_ref": "trace:send-message:demo-001",
  "message_committed_ref": "message-committed:demo-001",
  "agent_request_ref": "agent-request:release-followup-summary",
  "cases": []
}
```

Each case should include low-sensitive refs only:

| Field | Purpose |
| --- | --- |
| `case_id` | stable case id |
| `user_request_ref` | request ref, not raw prompt archive |
| `conversation_ref` | synthetic conversation/project ref |
| `visible_message_refs` | refs allowed for evidence |
| `forbidden_message_refs` | refs that must not appear in context |
| `policy_scope_ref` | synthetic policy scope |
| `evidence_pack_ref` | AI read boundary |
| `context_package_ref` | derived runtime input |
| `expected_memory_candidate_ref` | candidate-only memory |
| `expected_tool_intent_ref` | candidate tool/action intent |
| `approval_fixture_ref` | approval decision fixture ref |
| `action_fixture_ref` | action result fixture ref |
| `expected_failure_class` | expected failure or `none` |

Minimum cases:

1. happy path: summarize a release delay and propose a follow-up task;
2. denied evidence: forbidden message refs must not leak;
3. memory candidate needs review: group decision lacks enough confirmation;
4. unsafe tool output or provider: tool prepare is blocked or tainted;
5. approval required: proposal is not executed directly.

## New Module Shape

Preferred module:

```text
ai/python/nexusim_ai_eval/interview_demo.py
```

Suggested functions:

```python
load_interview_demo_fixture(path: Path) -> InterviewDemoFixture
run_interview_demo(fixture: InterviewDemoFixture) -> InterviewDemoResult
interview_demo_result_to_payload(result: InterviewDemoResult) -> dict[str, Any]
write_interview_demo_outputs(result: InterviewDemoResult, paths: OutputPaths) -> None
```

Use dataclasses or existing contract patterns from `nexusim_ai_eval.contracts`.
Keep names conceptual and local; do not present them as production schemas.

## Output Contract

The summary JSON should include:

```json
{
  "demo_status": "PASS",
  "hot_path_impact": "none",
  "agent_trigger_mode": "async_ref_only",
  "production_backend_connected": false,
  "raw_message_body_used": false,
  "message_committed_ref": "...",
  "evidence_pack_refs": [],
  "context_package_refs": [],
  "memory_candidate_refs": [],
  "tool_intent_refs": [],
  "approval_action_refs": [],
  "eval_report_ref": "...",
  "replay_bundle_ref": "...",
  "blocked_cases": []
}
```

EvalReport should reuse existing low-sensitive EvalReport payload conventions
where possible. ReplayBundle should reuse existing ReplayBundle refs and hashes
where possible.

## README Requirement

Add:

```text
ai/python/fixtures/agent_eval/interview_demo_README.md
```

It should explain:

- what the demo proves;
- why Agent is outside SendMessage hot path;
- why EvidencePack is the read boundary;
- why MemoryCandidate is not ACTIVE memory;
- why ToolIntent is not execution;
- how EvalReport and ReplayBundle prove safety and debuggability;
- what remains future production work.

Keep it concise enough to read before an interview.

## Test Plan

Add focused tests:

```text
ai/python/tests/test_agent_eval_interview_demo.py
```

Required tests:

- accepts the valid interview demo fixture;
- rejects production backend connection fields;
- rejects raw message body, raw prompt, provider request body or secret fields;
- verifies Agent hot-path impact is `none`;
- verifies denied evidence refs do not enter ContextPackage refs;
- verifies MemoryCandidate remains candidate-only;
- verifies ToolIntent/proposal does not execute side effects;
- verifies approval/action refs are fixture refs only;
- verifies EvalReport and ReplayBundle refs are emitted;
- verifies blocked cases include expected failure classes.

## Validation Commands

Focused:

```powershell
python -m pytest ai/python/tests/test_agent_eval_interview_demo.py -q
python ai/python/scripts/run_agent_interview_demo.py ai/python/fixtures/agent_eval/interview_send_message_agent_demo.json
```

Standard:

```powershell
python -m pytest ai/python/tests -q
python -m ruff check ai/python
python -m mypy ai/python/nexusim_ai_common ai/python/nexusim_ai_memory ai/python/nexusim_ai_eval ai/python/scripts
.\tools\check-python-ai-worker-boundary.ps1
.\tools\check-runbook-consistency.ps1
git diff --check
git diff --cached --check
```

`check-runbook-entrypoints.ps1` may still fail on pre-existing non-Agent service
brief length issues. Agent entrypoint files must remain within limits.

## Auto-Reject Rules

Reject the implementation if it:

- touches production service directories, proto, OpenAPI, Kafka schema,
  migration or startup path;
- imports real backend service packages;
- opens network connections;
- requires model credentials;
- reads real IM data;
- stores raw prompts, raw message bodies, raw provider outputs, secrets or
  private payloads;
- claims fixture data is production truth;
- lets Python own final proposal, approval, execution, ACTIVE memory or audit
  archive;
- lets ToolIntent execute side effects;
- hides denied evidence instead of reporting a low-sensitive coverage gap;
- makes Agent part of SendMessage synchronous latency.

## Done When

The module is done when:

- one command runs the demo from a clean checkout;
- summary, EvalReport and ReplayBundle outputs are low-sensitive;
- focused tests and standard gates pass;
- README explains the demo in interview language;
- runbook records that the next phase is demo hardening or interview narrative,
  not broad architecture expansion.

## What Codex Should Not Try To Do

Do not ask Codex to "write the whole IM project" from the SDDs.

Break the project into goals:

1. interview demo runner;
2. demo README;
3. interview narrative;
4. optional focused hardening cases;
5. only later, if explicitly requested, production ADR / contract work.

This keeps the work reviewable and prevents the SDD package from becoming an
unbounded implementation request.

## References

- Codex follow-goal guidance:
  <https://developers.openai.com/codex/use-cases/follow-goals>
- Codex best practices:
  <https://developers.openai.com/codex/learn/best-practices>
- Codex customization / AGENTS guidance:
  <https://developers.openai.com/codex/concepts/customization>
