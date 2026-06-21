# admin-service policy ruleset smoke - 2026-06-21

## Scope

This was a local multi-process smoke for the admin policy ruleset control path:

```text
CreateAdminOperation(POLICY_RULE_CHANGE)
-> ApproveAdminOperation
-> operation-worker
-> control-plane PublishConfigVersion(POLICY_RULESET_REF)
-> control-plane GetConfigSnapshot
```

The business path used public gRPC APIs. PostgreSQL access in the runner was
limited to applying migrations and cleaning the smoke tenant.

Raw summary:

```text
H:\NexusIM\loadtest-results\admin-policy-ruleset-smoke-20260621-103039\admin-policy-ruleset-smoke-summary.json
```

## Runtime

```text
admin-service grpc: dynamic localhost port
admin-service operation-worker: local process, POLICY_RULE_CHANGE routed to control-plane
control-plane-service grpc: dynamic localhost port
PostgreSQL: postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable
```

The summary was generated while the policy ruleset runner changes were still
uncommitted, so `git_dirty=true` is expected for this run.

## Result

```text
success=true
created_status=SUBMITTED
approved_status=APPROVED
final_status=SUCCEEDED
published_version=policy-admin-policy-ruleset-smoke-20260621-103039
snapshot_version=policy-admin-policy-ruleset-smoke-20260621-103039
snapshot_checksum=sha256:833a1bd5be5f440f2f7ecd5f4c998072b7cef366a6cfa2e0c12e24f3b6c82044
```

## Judgment

This proves the first-stage admin control path can approve a policy ruleset
reference change operation and execute it through the admin operation worker
against the control-plane public gRPC API.

This does not publish policy rule bodies, does not write policy-service private
tables, and does not prove provider-grade policy governance, admin UI,
compensation, or long-running production operations.
