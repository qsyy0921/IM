# ADR-033: api-gateway tenant quota source

## Status

Accepted.

## Context

`api-gateway` already supports first-stage tenant-scoped rate limiting, static tenant plan overrides and file-based hot reload. The next hardening step is often described as "config center / DB-backed quota", but a direct database dependency from `api-gateway` would blur service ownership and make the user-facing entrypoint depend on internal business tables.

The gateway boundary remains:

- verify gateway token;
- rewrite public identity into trusted metadata;
- proxy user-facing RPCs;
- enforce entrypoint transport and quota policy;
- expose low-sensitive operational metrics.

It must not become the owner of tenant billing, plan lifecycle, risk policy or business facts.

## Decision

`api-gateway` must not directly read or write business-service internal tables for tenant quota. The current supported tenant plan sources are:

```text
none
inline/json
file
url
```

`url` is a narrow HTTP(S) snapshot adapter: it consumes the same versioned quota snapshot contract used by file reload and applies it through the same atomic in-memory plan swap. It does not query tenant, billing or business storage.

`db`, `database`, `config`, `config-center` and unknown source values must fail closed until a separate control-plane/config service source is implemented and accepted.

The target quota source is a versioned control-plane/config-service contract:

```text
tenant quota authoring / approval
-> versioned quota snapshot
-> api-gateway watcher / reload port
-> atomic in-memory plan swap
-> low-sensitive applied-version and reload metrics
```

The config owner may store quota data in its own database, but `api-gateway` consumes a service-owned contract or exported snapshot, not arbitrary business tables.

## Required Evidence Before Enabling Config-Center Quota

Any future implementation must include:

- a service-owned schema or API contract for tenant quota snapshots;
- version, checksum and generated-at metadata;
- fail-closed behavior for malformed or unsupported config versions;
- rollback to the last valid applied snapshot;
- low-sensitive metrics for source, applied version, reload success and reload failure;
- no tenant id, user id, token or plan detail in public metrics labels;
- unit tests for invalid versions, stale snapshots and rollback;
- a small smoke proving reload without gateway restart;
- an ADR update if the source is not control-plane/config-service.

## Consequences

This keeps the gateway simple and prevents hidden coupling to tenant or billing storage. It also means "DB-backed quota" is not a shortcut where each gateway instance queries PostgreSQL on the hot path. The next implementation should introduce a narrow port and a versioned config-source adapter, then prove it with failure and rollback tests.
