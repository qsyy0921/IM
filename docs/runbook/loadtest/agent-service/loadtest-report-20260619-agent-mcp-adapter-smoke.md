# agent-service -> mcp-gateway Adapter Smoke Report

Date: 2026-06-19

Raw result directory:
`H:\NexusIM\loadtest-results\agent-mcp-adapter-smoke-20260619-044341`

## Scope

This is a local process smoke for the current AI application foundation path:

```text
retrieval-gateway -> agent-service -> mcp-gateway
                 mcp-gateway -> skill-registry + policy-service
                 mcp-gateway -> mcp_gateway_tool_call_audits
```

It verifies proposal-only Agent behavior after the mcp-gateway prepare boundary.
It does not execute an external MCP tool and does not claim production capacity.

## Runtime Shape

The smoke used:

- Existing local `retrieval-gateway` on `127.0.0.1:10590`.
- Temporary `policy-service` on `127.0.0.1:10801` with PG-backed tool rules enabled.
- Temporary `skill-registry` on `127.0.0.1:10640`.
- Temporary `mcp-gateway` on `127.0.0.1:10650`.
- Temporary current-build `agent-service` on `127.0.0.1:10631`.

The temporary processes were stopped after the smoke.

## Seeded Data

The runner applied the required expand-only migrations and seeded low-sensitive
rows for one smoke tenant:

- `skill_registry_definitions`: active `conversation.note.create` skill with
  `TOOL_ACTION_CALL` and approval required.
- `policy_tool_action_rules`: allow `CALL` on `conversation_note`, approval
  required, `permission_version=42`.
- search / memory projection rows that produce one search evidence item and one
  memory evidence item.

## Result

Summary path:
`H:\NexusIM\loadtest-results\agent-mcp-adapter-smoke-20260619-044341\agent-proposal-summary.json`

Key result fields:

```text
proposal_status=PROPOSED
requires_approval=true
generated_by_llm=false
skill_id=conversation.note.create
prepared_audit_id=mcp_audit_561a0d2bb5046e98647932afc34e6cd0
policy_allowed=true
policy_requires_approval=true
policy_decision_source=TOOL_RULE
policy_classification=TOOL_APPROVAL_REQUIRED
policy_permission_version=42
mcp_audit.status=ALLOWED
mcp_audit.input_sha256_present=true
mcp_audit.idempotency_key_matches=true
evidence_item_count=2
search_item_count=1
memory_item_count=1
citation_count=2
```

## Verified Invariants

- `agent-service` called `mcp-gateway.PrepareToolCall` before generating the
  proposal and returned `skill_id` / `prepared_audit_id`.
- `mcp-gateway` accepted the registered skill and PG-backed policy rule.
- `mcp_gateway_tool_call_audits` recorded an `ALLOWED` prepare audit with
  `input_sha256`, not raw tool input.
- `agent-service` still retrieved EvidencePack through `retrieval-gateway`.
- The response remained proposal-only: no business mutation and
  `generated_by_llm=false`.
- Citations traced back to seeded message / memory source references.

## Follow-up

Next AI-mainline work is proposal store / approval workflow / action-executor
handoff. Real MCP / tool adapter execution remains out of scope until approval
and executor checks are connected.
