# ADR Candidate: Tool / MCP Boundary

Status: candidate. Not accepted. Does not authorize production MCP providers or
tool schemas.

## Context

The skeleton proves malicious tool description blocking, unsafe MCP output
quarantine, provider provenance mismatch, tool-selection attack blocking, schema
mismatch detection, prepare expiry, capability lease refs and provider
attestation refs.

The production risk is treating MCP providers, tool descriptions or tool output
as trusted authority.

## Candidate Decision

Tool and MCP providers are untrusted inputs. Agent Runtime may produce
ToolIntent and proposal refs. High-risk actions require prepare, policy,
approval and action-executor execution.

mcp-gateway owns provider provenance, schema hash, capability lease,
attestation refs, output tainting and prepare result refs. action-executor
remains the sole side-effect owner.

## Owned Objects

| Object | Owner | Purpose |
| --- | --- | --- |
| ToolProvider | mcp-gateway / governance | Registered provider identity |
| ProviderAttestation | mcp-gateway / governance | Provenance/review/sandbox proof ref |
| CapabilityLease | policy + mcp-gateway | Bounded prepare/use grant |
| PreparedToolRef | mcp-gateway | Prepare/precheck result |
| ToolSchemaHash | mcp-gateway | Schema used during prepare |
| ToolOutputEnvelope | mcp-gateway | Provenance, validation and taint wrapper |
| ToolRiskTier | governance / policy | Read, dry-run, proposal, approval, blocked |
| ToolExecutionPolicy | policy / action-executor | Required prepare/approval/execution behavior |

## Boundary Rules

- MCP server descriptions are untrusted.
- Tool output is not instruction.
- High-risk actions cannot auto-execute.
- Prepared refs expire.
- Execution requires fresh enough prepare lineage and approval when required.
- Schema changes after prepare require re-prepare.
- Tool output cannot become ACTIVE memory without memory admission.

## Capability Lease Gate

CapabilityLease must define:

- actor and tenant scope;
- skill and tool scope;
- allowed operation class;
- risk tier;
- expiry;
- policy decision ref;
- approval requirement;
- replay reader policy ref.

Missing, expired or over-broad leases reject prepare or force re-prepare. A
provider cannot self-issue authority by advertising a capability.

## Provider Attestation And Sandbox Onboarding

ProviderAttestation must record:

- provider identity;
- owner;
- review status;
- trust tier;
- schema hash;
- sandbox or production eligibility;
- last review ref.

Unknown or unreviewed providers are sandbox-only or blocked. Trusted-provider
selection requires current attestation; stale or missing attestation downgrades
the provider to sandbox behavior.

## Prepare Expiry And State-Diff Re-Prepare

PreparedToolRef is invalid when:

- lease expired;
- schema hash changed;
- provider attestation changed;
- relevant state diff invalidates the dry-run/precheck;
- approval window expired;
- actor, tenant, skill or tool scope changed.

Execution must verify fresh prepare lineage before action-executor accepts a
high-risk action. Stale prepare refs reject execution or force re-prepare.

## Tool Output Reuse

ToolOutputEnvelope must keep provenance, validation status and taint labels
whenever output enters ContextPackage, memory candidate extraction or replay.

Tool output cannot become:

- system instruction;
- permission authority;
- ACTIVE memory;
- execution approval;
- untainted factual source

without the owning verifier or admission path.

## Rejection Rules

Reject the ADR if:

- MCP provider is treated as permission authority;
- tool description is inserted as trusted instruction;
- high-risk action can execute without approval and executor;
- provider attestation and capability lease are optional for trusted providers;
- tool output can bypass taint labels;
- unknown providers can bypass sandbox onboarding;
- stale PreparedToolRef can execute without re-prepare.

## Next Evidence Needed

- Main integration review for capability lease matrix.
- Provider governance review for attestation and sandbox onboarding.
- Fixture proof for lease expiry, attestation downgrade, sandbox onboarding,
  state-diff re-prepare and stale PreparedToolRef rejection is recorded in
  `docs/research/agent-tool-mcp-fixture-evidence-20260702.md`.
- action-executor production review for stale PreparedToolRef rejection.
