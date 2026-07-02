# Agent Tool / MCP ADR Candidate Review

Date: 2026-07-02

Status: focused review of `adr-candidate-agent-tool-mcp-boundary.md`. This is
not an accepted ADR and does not authorize production MCP providers, tool
schemas or side-effect execution paths.

## Verdict

Initial verdict: rejected for ADR acceptance.

Reason: the candidate correctly treated providers as untrusted and kept
action-executor as side-effect owner, but left four P1 concerns as future
evidence: capability lease matrix, provider attestation governance,
prepare-expiry re-prepare policy and sandbox onboarding for unknown providers.

After this pass: conditionally passed for main integration review as the fifth
ADR candidate.

The condition is that main integration must still accept the mcp-gateway,
policy-service and action-executor ownership split. No production provider
contract is authorized.

## P0/P1/P2 Findings

| Severity | Finding | Impact | Closure |
| --- | --- | --- | --- |
| P0 | None inside isolated Agent Lab scope | No production MCP provider or side-effect execution exists | Keep hard boundary unchanged |
| P1 | Capability lease rules were review-only | Tool use could exceed bounded scope or freshness | Candidate now requires lease scope, expiry and denial behavior |
| P1 | Provider attestation governance was underspecified | Trusted-provider path could be granted without provenance | Candidate now requires attestation state and sandbox default |
| P1 | Prepare expiry was not tied to state diff | Execution could use stale precheck results | Candidate now requires re-prepare when schema, state or approval window changes |
| P1 | Unknown provider onboarding was future evidence | New MCP providers could bypass quarantine | Candidate now requires sandbox-only onboarding until reviewed |
| P2 | Provider capacity and timeout budgets remain conceptual | Does not block candidate review, but blocks production rollout | Keep for eval/ops hardening |

## Re-Review Checklist

| Gate | Result | Evidence |
| --- | --- | --- |
| Providers untrusted | Pass | Candidate rejects MCP descriptions and outputs as authority |
| Side-effect owner | Pass | Candidate keeps action-executor as sole side-effect owner |
| Capability lease | Pass after this pass | Candidate now requires bounded lease scope and expiry |
| Provider attestation | Pass after this pass | Candidate now requires attestation state for trusted provider selection |
| Prepare expiry | Pass after this pass | Candidate now requires re-prepare on schema/state/approval drift |
| Sandbox onboarding | Pass after this pass | Candidate now defaults unknown providers to sandbox-only or blocked |
| Production boundary | Pass | Candidate still does not add production providers or tool schemas |

## Remaining Conditions

- Main integration must accept the mcp-gateway / policy / executor ownership
  split.
- Fixture hardening should later prove lease denial, attestation downgrade and
  prepare-expiry re-prepare behavior.
- Production provider review process remains governance-scope.

## Next Review Target

Review AgentOps / Governance next. It must prove releases cannot proceed
without owner, eval gate, replay, audit, kill switch and rollback controls.
