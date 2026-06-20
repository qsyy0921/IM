# admin-service config publish smoke - 2026-06-21

## Scope

This was a local multi-process smoke for the first real downstream admin adapter:

```text
CreateAdminOperation(CONFIG_PUBLISH)
-> ApproveAdminOperation
-> admin-service operation-worker
-> control-plane-service PublishConfigVersion
-> control-plane-service GetConfigSnapshot
```

The smoke used public gRPC APIs for the business path. PostgreSQL access in the runner was limited to applying migrations and cleaning the smoke tenant.

Raw summary:

```text
H:\NexusIM\loadtest-results\admin-config-publish-smoke-20260621-045122\admin-config-publish-smoke-summary.json
```

## Runtime

```text
admin-service grpc: dynamic localhost port
admin-service operation-worker: local process, CONFIG_PUBLISH routed to control-plane
control-plane-service grpc: dynamic localhost port
PostgreSQL: postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable
```

The summary was generated while the smoke runner changes were still uncommitted, so `git_dirty=true` is expected for this run.

## Result

```text
success=true
created_status=SUBMITTED
approved_status=APPROVED
final_status=SUCCEEDED
snapshot_version=quota-admin-config-publish-smoke-20260621-045122
snapshot_checksum=sha256:fc2848373abd981762936801f878cea64f5117616c5ecc56a8d52e0c5dd518a4
```

## Judgment

This proves the first-stage admin control path can create and approve a low-risk config publish operation, route it through the admin operation worker, and publish the version through control-plane public gRPC.

It does not prove provider-grade admin UI, rollback, workflow compensation, production HA, capacity, or long-running operator governance.
