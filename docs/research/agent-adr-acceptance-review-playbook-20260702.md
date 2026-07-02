# Agent ADR Acceptance Review Playbook

Date: 2026-07-02

Status: review playbook for main integration and future owner review. This is
not an accepted ADR, production contract, schema, migration, service directory
or runtime implementation.

## Verdict

Conditionally passed for ADR acceptance reviewability.

Rejected for controlled implementation until accepted ADRs, owner signoff,
real-service preservation smoke and production operator UX evidence exist.

Reason: the evidence package now has enough Agent Lab fixture proof to be
reviewed, but the previous package did not define a concrete signoff workflow.
Without this playbook, a reviewer could accept a candidate without naming the
owner, smoke evidence, replay reader policy or rejection reason that made the
acceptance valid.

## Purpose

This playbook turns the full-package entry request into a repeatable review
procedure. It defines:

- who can sign which part of the package;
- which evidence is sufficient inside Agent Lab;
- which evidence must come from another owner later;
- which failures automatically reject ADR acceptance;
- which result allows a later scoped implementation design.

It does not authorize Agent Lab to create production schema, service directories,
startup paths, real provider integrations or frozen contracts.

## Review Levels

| Level | Decision | Allowed Evidence | Not Allowed |
| --- | --- | --- | --- |
| L0 package entry | Decide whether the package can enter ADR acceptance review | SDDs, ADR candidates, fixture evidence, runbook state, main integration review ledger | Production implementation approval |
| L1 ADR acceptance | Accept or reject one ADR candidate | Candidate doc, focused review, required fixture gates, shared appendix refs | Proto, OpenAPI, Kafka schema, migration or service creation |
| L2 scoped implementation design | Decide a small future implementation slice | Accepted ADR, owner design, boundary smoke plan, rollout and rollback plan | Real service work without owner signoff |
| L3 real-service smoke | Prove refs survive real boundaries | Owner-run low-sensitive smoke over real service interfaces | Raw IM data, hidden fallback, Python-owned final state |
| L4 controlled implementation | Start explicitly scoped production work | Accepted ADR, approved design, real smoke evidence, operator UX plan | Broad agent-service rewrite or unreviewed taxonomy freeze |

## Reviewer Roles

| Role | Owns | Cannot Own |
| --- | --- | --- |
| Agent Lab | SDD drafts, ADR candidates, fixture-only evidence, eval harness, Python candidate checks | Production contracts, ACTIVE memory, approval, execution, audit archive, real-service state |
| Main integration | Package entry verdict, ADR acceptance coordination, cross-service conflict review, final merge decision | Silent production authorization from research docs |
| Service owner | Real service boundary, integration design, service smoke, persistence and runtime ownership | Agent Lab fixture claims as production proof |
| Security / policy owner | Permission, tenant scope, taint, untrusted provider handling, approval boundary | Tool or MCP trust-by-description |
| Audit owner | Audit lineage, evidence refs, retention class, redaction and replay observability | Raw prompt or provider body replay as normal evidence |
| Operator owner | Inspect-and-act UX, release controls, rollback, kill switch, failure-class workflow | Passive-only dashboards for high-risk states |

## Candidate Signoff Matrix

| Candidate | Required Signoffs | Agent Lab Evidence | External Evidence Before Implementation |
| --- | --- | --- | --- |
| Eval / Replay Harness | Main integration, eval owner, audit owner | Eval SDD, ADR candidate, version-bump replay, compatibility, report lifecycle fixtures | Real eval report retention and redaction owner approval |
| Runtime / Workflow Boundary | Main integration, runtime owner, workflow owner | Runtime SDD, ownership matrix, wakeup/cancel/resume/budget fixtures | Real workflow wakeup and cancellation smoke plan |
| Context / EvidencePack | Main integration, retrieval owner, audit owner, security owner | Evidence SDD, preservation rehearsal, citation verifier and denied-lane fixtures | Real retrieval source-ref preservation smoke |
| Memory Admission | Main integration, memory owner, policy owner, operator owner | Memory SDD, category threshold, revocation, ACTIVE rejection and operator fixtures | Real memory admission smoke and operator review |
| Tool / MCP Boundary | Main integration, MCP owner, security owner, action-executor owner | Tool SDD, capability lease, attestation, prepare expiry and sandbox fixtures | Real provider onboarding, prepare/execute and timeout smoke |
| AgentOps / Governance | Main integration, operator owner, release owner, audit owner | AgentOps SDD, release gate, kill switch, baseline approval, failure-class fixtures | Production admin UX, release rollback and incident workflow review |

## Auto-Reject Rules

Any reviewer must reject the affected candidate if one of these is true:

- the candidate lets Python own final business state, ACTIVE memory, approval,
  execution, audit archive or production workflow state;
- the candidate requires workflow-service to read raw prompt, EvidencePack body,
  planner state or model output;
- the candidate allows action-executor to execute without prepared, approved and
  auditable action refs;
- the candidate treats MCP server, tool description or provider output as
  trusted instruction;
- replay requires raw prompt, raw provider body or archived sensitive body for
  normal verification;
- a boundary drops scope, version, taint, source ref or audit lineage;
- operator governance is passive-only for memory, evidence, replay, approval,
  release, failure class, kill switch or rollback;
- fixture evidence claims to authorize a production contract or real service
  integration;
- the candidate freezes taxonomy or schema that the package explicitly leaves
  open.

## Deferral Rules

A reviewer should defer, not reject, when Agent Lab evidence is sufficient but
the next proof belongs to another owner:

- real-service preservation smoke for retrieval, memory, MCP, workflow,
  executor or audit lanes;
- production operator UX for inspect-and-act surfaces;
- SLO, capacity, cost, retention or on-call policy;
- provider onboarding or sandbox policy for real MCP providers;
- migration, schema, service registry, compose or deployment decisions.

A deferral must name the blocking owner and the exact evidence expected.

## Acceptance Result Template

Use this shape for each candidate:

```text
Candidate:
Verdict: accept for ADR / reject / defer
Severity: none / P0 / P1 / P2
Required signoffs checked:
Agent Lab evidence checked:
External blocker, if any:
Rejected production shortcuts:
Allowed next step:
Disallowed next step:
```

## Allowed Next Steps After L1 Acceptance

L1 acceptance allows only a later scoped implementation design. That design must
name:

- the owning Go service or future service module;
- object owner and non-owner state;
- version and compatibility window;
- replay reader policy;
- permission and audit boundary;
- preservation smoke plan;
- operator inspect-and-act surface;
- rollback and kill-switch behavior;
- fixture or public-dataset gate to rerun.

L1 acceptance still does not allow production code until L2 and L3 are complete.

## Current Package Decision

Main integration accepted the package for L0 entry into ADR acceptance review.
It did not approve production implementation. Begin L1 review in this order:

1. Eval / Replay Harness.
2. Runtime / Workflow Boundary.
3. Context / EvidencePack.
4. Memory Admission.
5. Tool / MCP Boundary.
6. AgentOps / Governance.

Eval / Replay Harness and Runtime / Workflow Boundary have been accepted for L1
ADR acceptance by main integration. Context / EvidencePack is the next L1
candidate under review.

Current P0 inside Agent Lab scope: none known.

Current P1 inside Agent Lab scope: none known.

External blockers remain:

- accepted ADRs do not exist yet;
- remaining L1 ADR candidate verdicts are pending;
- production owner signoff is missing;
- real-service preservation smoke is missing;
- production operator UX is missing.

## Re-Review Result

After adding this playbook, the package is more reviewable because acceptance is
no longer just a prose recommendation. Each candidate now has named signoffs,
auto-reject rules, deferral rules and a concrete result template.

This closes the Agent Lab-side review-process gap. It does not close any
external implementation blocker.
