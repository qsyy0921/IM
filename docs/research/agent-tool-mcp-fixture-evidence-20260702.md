# Agent Tool / MCP Fixture Evidence

Date: 2026-07-02

Status: fixture-only evidence update for the Tool / MCP ADR candidate. This is
not an accepted ADR, MCP provider contract, tool schema, service directory or
production execution path.

## Verdict

Conditionally passed for the Tool / MCP governance rehearsal slice.

The fixture harness now proves, with low-sensitive refs only, that capability
leases bound prepare scope, stale or over-broad leases cannot prepare,
attestation downgrade prevents trusted-provider selection, expired or drifted
PreparedToolRef requires re-prepare and action-executor rejects stale prepare or
missing approval before any side effect.

This does not authorize production MCP providers or tool schemas.

## Added Evidence

Code:

- `ai/python/nexusim_ai_eval/tool_mcp_governance.py`
- `ai/python/tests/test_agent_eval_tool_mcp_governance.py`

Fixture:

- `ai/python/fixtures/agent_eval/tool_mcp_governance_rehearsal.json`

The helper verifies:

- Agent Runtime owns ToolIntent only and cannot execute side effects;
- mcp-gateway owns PreparedToolRef, provider attestation, output envelope and
  prepare audit refs;
- action-executor remains the only side-effect execution owner;
- missing, expired or over-broad CapabilityLease refs block prepare;
- stale or missing ProviderAttestation refs downgrade providers to sandbox or
  reject trusted-provider selection;
- unknown MCP providers are sandbox-only until reviewed;
- schema, lease, attestation, state-diff or approval-window drift forces
  re-prepare before execution;
- rejected prepare refs cannot be executed;
- tool output remains tainted and cannot become instruction, permission
  authority or ACTIVE memory;
- action-executor rejects stale PreparedToolRef and missing approval without
  executing side effects.

## Review Closure

This closes the fixture evidence portion of the Tool / MCP ADR review
condition:

- "Fixture hardening should later prove lease denial, attestation downgrade and
  prepare-expiry re-prepare behavior."

It also adds fixture evidence for sandbox onboarding, tool-output reuse and
action-executor stale prepare rejection.

It does not close:

- main integration review for mcp-gateway / policy / action-executor ownership;
- provider governance review for production attestation and onboarding;
- action-executor owner review for production stale PreparedToolRef rejection;
- production provider capacity and timeout budgets.

## Next Evidence Target

AgentOps fixture evidence is now recorded in
`docs/research/agentops-governance-fixture-evidence-20260702.md`.

Next work should focus on main integration review or memory calibration /
dataset reproducibility hardening.
