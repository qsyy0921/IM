# admin-service config rollback smoke - 2026-06-21

## Scope

This was a local multi-process smoke for the admin rollback control path:

```text
CreateAdminOperation(CONFIG_PUBLISH v1)
-> ApproveAdminOperation
-> operation-worker
-> control-plane PublishConfigVersion
-> CreateAdminOperation(CONFIG_PUBLISH v2)
-> ApproveAdminOperation
-> operation-worker
-> control-plane PublishConfigVersion
-> CreateAdminOperation(CONFIG_ROLLBACK target=v1)
-> ApproveAdminOperation
-> operation-worker
-> control-plane RollbackConfigVersion
-> control-plane GetConfigSnapshot
```

The business path used public gRPC APIs. PostgreSQL access in the runner was
limited to applying migrations and cleaning the smoke tenant.

Raw summary:

```text
H:\NexusIM\loadtest-results\admin-config-rollback-smoke-20260621-051457\admin-config-rollback-smoke-summary.json
```

## Runtime

```text
admin-service grpc: dynamic localhost port
admin-service operation-worker: local process, CONFIG_PUBLISH / CONFIG_ROLLBACK routed to control-plane
control-plane-service grpc: dynamic localhost port
PostgreSQL: postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable
```

The summary was generated while the rollback runner changes were still
uncommitted, so `git_dirty=true` is expected for this run.

## Result

```text
success=true
created_status=SUBMITTED
approved_status=APPROVED
final_status=SUCCEEDED
second_final_status=SUCCEEDED
rollback_final_status=SUCCEEDED
published_version=quota-admin-config-rollback-smoke-20260621-051457-v1
candidate_version=quota-admin-config-rollback-smoke-20260621-051457-v2
rollback_target=quota-admin-config-rollback-smoke-20260621-051457-v1
snapshot_version=quota-admin-config-rollback-smoke-20260621-051457-v1
snapshot_checksum=sha256:fc2848373abd981762936801f878cea64f5117616c5ecc56a8d52e0c5dd518a4
```

## Judgment

This proves the first-stage admin control path can publish two config versions,
then approve and execute a low-risk rollback operation through the admin
operation worker and control-plane public gRPC.

It does not prove provider-grade admin UI, broad compensation workflow,
production HA, capacity, drift monitor, or long-running rollout governance.
