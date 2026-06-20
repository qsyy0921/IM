# admin-service tenant quota smoke - 2026-06-21

## Scope

This was a local multi-process smoke for the admin tenant quota control path:

```text
CreateAdminOperation(TENANT_QUOTA_CHANGE)
-> ApproveAdminOperation
-> operation-worker
-> control-plane PublishConfigVersion(API_GATEWAY_TENANT_QUOTA)
-> control-plane GetConfigSnapshot
```

The business path used public gRPC APIs. PostgreSQL access in the runner was
limited to applying migrations and cleaning the smoke tenant.

Raw summary:

```text
H:\NexusIM\loadtest-results\admin-tenant-quota-smoke-20260621-053138\admin-tenant-quota-smoke-summary.json
```

## Runtime

```text
admin-service grpc: dynamic localhost port
admin-service operation-worker: local process, TENANT_QUOTA_CHANGE routed to control-plane
control-plane-service grpc: dynamic localhost port
PostgreSQL: postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable
```

The summary was generated while the tenant quota runner changes were still
uncommitted, so `git_dirty=true` is expected for this run.

## Result

```text
success=true
created_status=SUBMITTED
approved_status=APPROVED
final_status=SUCCEEDED
published_version=quota-admin-tenant-quota-smoke-20260621-053138
snapshot_version=quota-admin-tenant-quota-smoke-20260621-053138
snapshot_checksum=sha256:fc2848373abd981762936801f878cea64f5117616c5ecc56a8d52e0c5dd518a4
```

## Judgment

This proves the first-stage admin control path can approve a tenant quota
change operation and execute it through the admin operation worker against the
control-plane public gRPC API.

It does not prove provider-grade admin UI, full quota rollout governance,
capacity, drift monitor, or long-running production operations.
