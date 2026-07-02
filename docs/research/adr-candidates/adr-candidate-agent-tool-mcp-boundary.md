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

## Rejection Rules

Reject the ADR if:

- MCP provider is treated as permission authority;
- tool description is inserted as trusted instruction;
- high-risk action can execute without approval and executor;
- provider attestation and capability lease are optional for trusted providers;
- tool output can bypass taint labels.

## Next Evidence Needed

- Capability lease matrix review.
- Provider attestation governance review.
- Prepare-expiry re-prepare policy tied to state diff.
- Sandbox onboarding rules for unknown MCP providers.
