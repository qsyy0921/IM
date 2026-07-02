# Agent Eval / Replay L1 Acceptance Review

Date: 2026-07-02

Status: Agent Lab L1 self-review for
`adr-candidate-agent-eval-replay-harness.md`. This is not an accepted ADR,
production contract, schema, migration, service directory, release pipeline or
runtime implementation.

## Verdict

Recommendation: accept the Eval / Replay candidate for L1 ADR acceptance review.

Do not approve production implementation from this review.

Reason: the candidate has no known P0/P1 gap inside Agent Lab scope after the
focused review and fixture-evidence hardening. It defines release-gate
semantics, replay minimums, failure-class lifecycle, baseline approval,
retention/redaction, version-bump replay, dataset reproducibility and promotion
blocking in a way that can be reviewed by main integration.

## Playbook Result

```text
Candidate: Agent Eval / Replay Harness
Verdict: recommend accept for L1 ADR review; defer implementation
Severity: none inside Agent Lab scope
Required signoffs checked: Agent Lab complete; main integration pending; eval owner and audit owner required before implementation
Agent Lab evidence checked: SDD, ADR candidate, focused review, replay version-bump fixture, contract-version compatibility, dataset reproducibility, operator governance, operational readiness and controlled implementation readiness
External blocker, if any: production eval report retention/redaction owner approval; real ai-eval-service storage/operator design; release pipeline integration
Rejected production shortcuts: production schema, service directory, baseline auto-refresh, raw payload replay, Python-owned production truth and unaccepted contract freeze
Allowed next step: main integration may decide whether to accept this ADR candidate
Disallowed next step: production code, schema, migration, service registry, real provider/backend integration or release automation
```

## Evidence Reviewed

| Evidence | Result |
| --- | --- |
| `docs/sdd/agent-eval-replay-harness.md` | Pass; ownership, non-goals, replay refs, eval metrics, failure semantics and isolated slice are explicit |
| `docs/research/adr-candidates/adr-candidate-agent-eval-replay-harness.md` | Pass; release-gate semantics, replay minimums, baseline approval, retention/redaction and rejection rules are named |
| `docs/research/agent-eval-replay-adr-review-20260702.md` | Pass; earlier P1 findings were closed or moved to explicit external conditions |
| `docs/research/agent-fixture-evidence-hardening-20260702.md` | Pass; ReplayBundle version-bump rehearsal exists and raw legacy payloads fail closed |
| `docs/research/agent-contract-version-compatibility-fixture-evidence-20260702.md` | Pass; EvalReport and ReplayBundle have compatibility-window, replay-reader, redaction, deprecation, migration, preservation, audit, operator and rejection refs |
| `docs/research/agent-dataset-reproducibility-fixture-evidence-20260702.md` | Pass; dataset manifest, license, snapshot, split, import, adapter and deterministic report refs are required |
| `docs/research/agent-operator-governance-fixture-evidence-20260702.md` | Pass; replay, release, failure-class and rollback surfaces require inspect-and-act paths |
| `docs/research/agent-operational-readiness-fixture-evidence-20260702.md` | Pass for fixture scope; eval retention and escalation budget evidence exist, while production SLOs remain deferred |
| `docs/research/agent-controlled-implementation-readiness-fixture-evidence-20260702.md` | Pass; implementation remains blocked without accepted ADR, owner review and preservation evidence |

## Requirement Matrix

| L1 Requirement | Review Result | Notes |
| --- | --- | --- |
| Eval / Replay is the first promotion gate | Pass | Candidate makes eval/replay the first ADR and release gate for future Agent slices |
| Release blocks missing required suite | Pass | Missing suite, P0/P1 failure, incomplete replay, unapproved baseline and unreproducible dataset block promotion |
| Replay does not depend on raw payloads | Pass | Raw prompt, raw provider body, raw IM text, secrets and full MCP payload archives are rejected for normal replay |
| Replay minimum refs are complete | Pass | Context, EvidencePack, memory, tool, workflow, execution, state diff, audit and failure taxonomy refs are required |
| Failure-class lifecycle is governed | Pass | Severity, owner, first-seen report, disposition, regression fixture and retirement rule are required |
| Baseline refresh is governed | Pass | BaselineApproval requires old/new refs, reviewer, reason, waivers/rejections and rollback ref |
| Retention/redaction is low-sensitive by default | Pass | Report and replay artifacts carry retention class and redaction policy refs; forbidden raw fields are named |
| Replay reader compatibility is testable | Pass | Version-bump rehearsal proves older ReplayBundle artifacts stay explainable or fail closed |
| Dataset evidence is reproducible | Pass | Public/synthetic manifests require license, snapshot, split, import and deterministic report refs |
| Operator can inspect and act | Pass | Replay, release, failure-class, kill switch and rollback surfaces are covered by operator governance evidence |
| Controlled implementation remains blocked | Pass | Readiness gate rejects unaccepted ADRs, production contract shortcuts, real service shortcuts and Python final ownership |

## Auto-Reject Audit

No auto-reject condition was found inside the current Agent Lab scope.

| Auto-Reject Rule | Result |
| --- | --- |
| Python owns final business state, approval, execution, audit archive or production workflow state | Not triggered; Python remains fixture/eval/candidate-only |
| Replay requires raw prompt, raw provider body or archived sensitive body | Not triggered; normal replay uses low-sensitive refs and hashes |
| Boundary drops scope, version, taint, source ref or audit lineage | Not triggered in fixture evidence; real boundary smoke remains external |
| Operator governance is passive-only | Not triggered; inspect-and-act refs exist for replay/release/failure/rollback surfaces |
| Fixture evidence authorizes production contract | Not triggered; every evidence doc rejects production authorization |
| Candidate freezes schema or taxonomy | Not triggered; candidate freezes gate semantics and replay minimums only |

## Findings

| Severity | Finding | Disposition |
| --- | --- | --- |
| P0 | None inside Agent Lab scope | No production safety contradiction found because real data and production implementation remain blocked |
| P1 | None inside Agent Lab scope | Previous P1s for failure lifecycle, baseline approval, retention/redaction and version-bump replay are closed to fixture/review level |
| P2 | Production retention durations, storage layout, release UX and on-call policy are not implemented | External owner / later scoped implementation design; does not block L1 ADR review |
| P2 | Large public benchmark thresholds and cost/latency targets remain research baselines | Keep as production readiness backlog, not an ADR acceptance blocker |

## External Deferrals

These items must be deferred to L2/L3 review before implementation:

- eval owner approval for ai-eval-service report catalog, storage, retention and
  baseline management;
- audit / security owner approval for final redaction duration, deletion behavior
  and replay reader behavior after source expiry;
- release / operator owner approval for baseline approval UX, failure-class
  owner workflow, rollback and kill-switch surfaces;
- real-service preservation smoke for EvalReport / ReplayBundle refs if the
  future implementation crosses ai-eval-service, agent runtime, audit-service or
  governance/control-plane boundaries;
- production telemetry, SLO and on-call policy for report generation latency,
  eval retention and incident escalation.

## Allowed After L1 Acceptance

If main integration accepts this ADR candidate, the only allowed next step is a
scoped implementation design for Eval / Replay as the first promotion gate.

That design must name:

- ai-eval-service and Agent Runtime ownership boundaries;
- low-sensitive EvalReport and ReplayBundle refs;
- version and compatibility-window policy;
- replay reader policy;
- permission, audit and redaction boundaries;
- baseline approval and rollback workflow;
- fixture/public-dataset gates that must continue to pass.

## Still Disallowed

L1 acceptance must not create or freeze:

- proto, OpenAPI, Kafka schema, migration or database tables;
- production service directory or startup path;
- production EvalReport or ReplayBundle wire shape;
- release automation or baseline auto-refresh;
- real PostgreSQL, Kafka, Redis, OpenSearch, model provider, MCP provider,
  workflow-service, memory-service or action-executor integration;
- Python ownership of production truth, approval, execution, audit archive or
  ACTIVE memory.

## Re-Review Result

After applying the ADR acceptance playbook, the Eval / Replay candidate is
reviewable and has no known Agent Lab P0/P1 blocker.

Agent Lab recommends that main integration review this candidate first. If main
integration accepts it, Agent Lab should then prepare the Runtime / Workflow L1
review package. If main integration rejects or defers, Agent Lab should handle
that P0/P1 or owner-evidence request before moving on.
