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
| P2 | Provider capacity and timeout budgets were conceptual | Does not block candidate review, but blocks production rollout without real provider owner review | Fixture-only operational readiness rehearsal now covers tool-timeout evidence; production capacity remains blocked |

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
- Fixture code now proves lease denial, attestation downgrade, sandbox
  onboarding, prepare re-prepare, tool-output reuse and stale prepare executor
  rejection in `ai/python/nexusim_ai_eval/tool_mcp_governance.py`,
  `ai/python/fixtures/agent_eval/tool_mcp_governance_rehearsal.json` and
  `ai/python/tests/test_agent_eval_tool_mcp_governance.py`.
- Production provider review process and real provider capacity planning remain
  governance-scope.

## Fixture Evidence Update

Fixture evidence is recorded in
`docs/research/agent-tool-mcp-fixture-evidence-20260702.md`.

This update closes the fixture-only proof request for lease denial,
attestation downgrade and prepare-expiry re-prepare. It does not close
production ownership acceptance, provider governance, action-executor production
contract review or real provider capacity / timeout budgets. Fixture-only
timeout budget evidence is separately recorded in
`docs/research/agent-operational-readiness-fixture-evidence-20260702.md`.

## Next Review Target

Review AgentOps / Governance next. It must prove releases cannot proceed
without owner, eval gate, replay, audit, kill switch and rollback controls.
