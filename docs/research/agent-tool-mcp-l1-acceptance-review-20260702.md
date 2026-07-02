# Agent Tool / MCP L1 Acceptance Review

Date: 2026-07-02

Status: Agent Lab L1 self-review for
`adr-candidate-agent-tool-mcp-boundary.md`. This is not an accepted ADR, MCP
provider contract, tool schema, production service directory, migration,
runtime implementation or side-effect execution path.

## Verdict

Recommendation: accept the Tool / MCP Boundary candidate for L1 ADR acceptance
review after the accepted Eval / Replay, Runtime / Workflow, Context /
EvidencePack and Memory Admission L1 gates.

Do not approve production implementation from this review.

Reason: the candidate has no known P0/P1 gap inside Agent Lab scope after the
focused review and fixture-evidence hardening. It treats MCP providers, tool
descriptions and tool output as untrusted inputs, keeps Agent Runtime
ToolIntent-only, requires bounded capability leases, current provider
attestation, prepare expiry and re-prepare rules, sandbox onboarding for
unknown providers, output taint preservation and action-executor ownership for
side effects.

## Playbook Result

```text
Candidate: Tool / MCP Boundary
Verdict: recommend accept for L1 ADR review; defer implementation
Severity: none inside Agent Lab scope
Required signoffs checked: Agent Lab complete; Eval / Replay, Runtime / Workflow, Context / EvidencePack and Memory Admission L1 accepted; main integration pending; MCP, security, policy and action-executor owners required before implementation
Agent Lab evidence checked: Tool/MCP SDD, ADR candidate, focused review, tool/MCP governance fixture evidence, cross-service preservation, object completeness, operator governance, operational readiness and controlled implementation readiness
External blocker, if any: real provider onboarding review; production prepare/execute smoke; production sandbox policy; provider capacity/timeout policy; final tool/MCP field design
Rejected production shortcuts: MCP provider as permission authority, trusted tool descriptions, untainted tool output, high-risk auto-execution, stale PreparedToolRef execution, unknown provider production eligibility and fixture-authorized provider/schema promotion
Allowed next step: main integration may decide whether to accept this ADR candidate
Disallowed next step: production MCP provider onboarding, tool schema, gateway API changes, action-executor path changes, migrations, service registry, real backend/provider integration or side-effect execution implementation
```

## Evidence Reviewed

| Evidence | Result |
| --- | --- |
| `docs/sdd/agent-tool-mcp-boundary.md` | Pass; tools can be prepared but Runtime cannot execute high-risk side effects |
| `docs/research/adr-candidates/adr-candidate-agent-tool-mcp-boundary.md` | Pass; providers and outputs are untrusted, mcp-gateway owns prepare/provenance and action-executor owns side effects |
| `docs/research/agent-tool-mcp-adr-review-20260702.md` | Pass; earlier P1 findings were closed to fixture/review level or moved to explicit external conditions |
| `docs/research/agent-tool-mcp-fixture-evidence-20260702.md` | Pass; capability lease denial, attestation downgrade, sandbox onboarding, re-prepare and stale prepare rejection are fixture-proven |
| `docs/research/agent-cross-service-preservation-fixture-evidence-20260702.md` | Pass; MCP -> Runtime and executor -> eval/replay boundaries preserve role, scope, version, taint, audit-lineage and replay-reader refs |
| `docs/research/agent-object-completeness-fixture-evidence-20260702.md` | Pass; ToolIntent, PreparedToolRef and execution objects have owner/lifecycle/version/permission/audit/replay/operator/rejection refs in the conceptual object catalog |
| `docs/research/agent-operator-governance-fixture-evidence-20260702.md` | Pass; approval and release governance require inspect-and-act paths, not passive-only views |
| `docs/research/agent-operational-readiness-fixture-evidence-20260702.md` | Pass; tool provider timeout budgets require owner, measurement, operator, audit and release-gate refs before promotion |
| `docs/research/agent-controlled-implementation-readiness-fixture-evidence-20260702.md` | Pass; implementation remains blocked without accepted ADR, owner review and preservation evidence |

## Requirement Matrix

| L1 Requirement | Review Result | Notes |
| --- | --- | --- |
| Providers remain untrusted | Pass | MCP server descriptions, prompts, resources, metadata and outputs cannot define authority |
| Runtime is ToolIntent-only | Pass | Runtime can propose tool intent and candidate args, but cannot own prepare truth or execute side effects |
| mcp-gateway owns prepare/provenance | Pass | Provider provenance, schema hash, attestation, output envelope and prepare refs stay gateway owned |
| action-executor owns side effects | Pass | High-risk execution requires fresh prepare lineage, approval when required and action-executor validation |
| Capability leases are bounded | Pass | Actor, tenant, skill, tool, operation, risk, expiry, policy and approval refs are required |
| Provider attestation gates trust | Pass | Missing or stale attestation downgrades providers to sandbox behavior or rejects trusted selection |
| Unknown providers are sandbox-only or blocked | Pass | Production eligibility requires review and owner-approved attestation |
| Prepare expires and can drift | Pass | Schema, lease, attestation, state-diff, approval-window or scope drift forces re-prepare |
| Tool output stays tainted | Pass | Output cannot become instruction, permission authority, ACTIVE memory or untainted source truth by reuse |
| Replay avoids re-execution | Pass | Replay uses low-sensitive refs and hashes, not provider body archives or side-effect reruns |
| Production field shape remains unfrozen | Pass | Candidate freezes ownership and rejection rules only, not tool, MCP or provider schemas |

## Auto-Reject Audit

No auto-reject condition was found inside the current Agent Lab scope.

| Auto-Reject Rule | Result |
| --- | --- |
| MCP provider is treated as permission authority | Not triggered; policy refs and owner review remain required |
| Tool description is inserted as trusted instruction | Not triggered; descriptions stay tainted/untrusted |
| High-risk action can execute without prepare, approval and executor | Not triggered; action-executor is the sole side-effect owner |
| Provider attestation or capability lease is optional for trusted providers | Not triggered; both are required for trusted selection and prepare |
| Tool output bypasses taint labels | Not triggered; output envelopes preserve taint and provenance refs |
| Unknown providers bypass sandbox onboarding | Not triggered; unknown providers are sandbox-only or blocked |
| Stale PreparedToolRef can execute without re-prepare | Not triggered in fixture evidence; production executor smoke remains external |
| Fixture evidence authorizes production provider or schema | Not triggered; every related doc rejects provider/schema promotion |

## Findings

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside Agent Lab scope | No production provider or side-effect execution path exists |
| P1 | None inside Agent Lab scope | Previous P1s for lease, attestation, re-prepare and sandbox onboarding are closed to fixture/review level |
| P2 | Provider capacity and timeout budgets remain fixture-only | Production readiness backlog, not an L1 ownership blocker |
| P2 | Production provider onboarding policy is not owner-approved | External MCP/security/governance owner evidence before implementation |
| P2 | Real prepare/execute and stale-ref smoke is missing | External L2/L3 evidence before implementation |

## External Deferrals

These items must be deferred to L2/L3 review before implementation:

- mcp-gateway owner approval for provider registry, provenance, schema hash,
  provider attestation, capability lease, prepare result and output envelope
  behavior;
- security/policy owner approval for scope, tenant, risk-tier, trusted-provider
  selection, sandbox onboarding and denial semantics;
- action-executor owner approval for fresh prepare verification, stale
  PreparedToolRef rejection, idempotency, state-diff, controlled redrive and
  side-effect audit behavior;
- workflow-service owner approval for approval waits only, without workflow or
  Runtime executing side effects;
- operator/governance owner approval for provider review, sandbox release,
  timeout/failure-class inspection, rollback and kill-switch behavior;
- real-service preservation smoke proving prepared refs, provider refs, scope
  refs, version refs, taint labels, approval refs, execution refs, audit lineage
  and replay-reader refs survive mcp-gateway, Runtime, workflow,
  action-executor, eval and audit boundaries.

## Allowed After L1 Acceptance

If main integration accepts this ADR candidate, the only allowed next step is a
scoped implementation design for the Tool / MCP boundary.

That design must name:

- mcp-gateway ownership of provider provenance, attestation, schema hash,
  capability lease, prepare refs and output taint;
- policy/security ownership of actor, tenant, scope, risk and sandbox decisions;
- workflow-service ownership only for durable approval waits;
- action-executor ownership of execution, idempotency, state-diff, redrive and
  execution audit refs;
- compatibility window and replay-reader policy;
- provider onboarding and sandbox governance;
- operator inspect-and-act surfaces for provider, timeout, approval, execution,
  failure class and rollback states;
- fixture/public-dataset gates that must continue to pass.

## Still Disallowed

L1 acceptance must not create or freeze:

- production MCP provider contracts, tool schemas or gateway APIs;
- action-executor prepare/execute API changes;
- proto, OpenAPI, Kafka schema, migration or database tables;
- real PostgreSQL, Kafka, Redis, OpenSearch, model provider, MCP provider,
  workflow-service, memory-service or action-executor integration;
- raw provider body replay or side-effect re-execution as normal verification;
- MCP server, tool description or provider output as trusted instruction,
  permission authority, ACTIVE memory or execution approval;
- Python ownership of final proposal, ACTIVE memory, approval, execution,
  production source truth or audit archive.

## Re-Review Result

After applying the ADR acceptance playbook, the Tool / MCP Boundary candidate is
reviewable and has no known Agent Lab P0/P1 blocker.

Agent Lab recommends that main integration review this candidate fifth. If main
integration accepts it, Agent Lab should then prepare the AgentOps / Governance
L1 review package. If main integration rejects or defers, Agent Lab should
handle that P0/P1 or owner-evidence request before moving on.
